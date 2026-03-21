package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/unix"
)

// defaultCNIConfig is an embedded bridge network config that provides outbound
// internet access via NAT masquerade. It creates a Linux bridge (boxer0),
// assigns addresses from 10.88.0.0/16 using host-local IPAM, and enables IP
// masquerade so containers can reach the public internet.
const defaultCNIConfig = `{
  "cniVersion": "1.0.0",
  "name": "boxer",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "boxer0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "10.88.0.0/16",
        "routes": [{"dst": "0.0.0.0/0"}]
      }
    }
  ]
}`

const netnsDir = "/var/run/netns"

// NetworkSetup holds the resources allocated for a container's network namespace.
// Call Teardown after the container exits to release them.
type NetworkSetup struct {
	netNS    ns.NetNS
	execID   string
	confList *libcni.NetworkConfigList
	cniConf  *libcni.CNIConfig
}

// NetNSPath returns the filesystem path of the pinned network namespace.
func (n *NetworkSetup) NetNSPath() string {
	return n.netNS.Path()
}

// SetupNetwork creates a new network namespace pinned to a stable filesystem
// path, runs CNI ADD to configure a veth pair, IP address, default route, and
// NAT masquerade, then returns a NetworkSetup the caller must Teardown after
// the container exits.
//
// Requires CAP_NET_ADMIN (running as root in practice).
func SetupNetwork(ctx context.Context, execID string, pluginDirs []string, cacheDir string) (*NetworkSetup, error) {
	netnsPath := filepath.Join(netnsDir, "boxer-"+execID)

	if err := createNetNS(netnsPath); err != nil {
		return nil, fmt.Errorf("create network namespace: %w", err)
	}

	netNS, err := ns.GetNS(netnsPath)
	if err != nil {
		removeNetNS(netnsPath)
		return nil, fmt.Errorf("open network namespace: %w", err)
	}

	confList, err := libcni.ConfListFromBytes([]byte(defaultCNIConfig))
	if err != nil {
		netNS.Close() //nolint:errcheck
		removeNetNS(netnsPath)
		return nil, fmt.Errorf("parse CNI config: %w", err)
	}

	cniConf := libcni.NewCNIConfigWithCacheDir(pluginDirs, cacheDir, nil)

	rtConf := &libcni.RuntimeConf{
		ContainerID: execID,
		NetNS:       netnsPath,
		IfName:      "eth0",
	}

	if _, err := cniConf.AddNetworkList(ctx, confList, rtConf); err != nil {
		netNS.Close() //nolint:errcheck
		removeNetNS(netnsPath)
		return nil, fmt.Errorf("CNI ADD: %w", err)
	}

	log.Debug().Str("exec_id", execID).Str("netns", netnsPath).Msg("CNI network setup complete")

	return &NetworkSetup{
		netNS:    netNS,
		execID:   execID,
		confList: confList,
		cniConf:  cniConf,
	}, nil
}

// Teardown runs CNI DEL and removes the pinned network namespace. Errors are
// logged rather than returned — teardown is always best-effort so it does not
// mask the container's result.
func (n *NetworkSetup) Teardown(ctx context.Context) {
	rtConf := &libcni.RuntimeConf{
		ContainerID: n.execID,
		NetNS:       n.netNS.Path(),
		IfName:      "eth0",
	}
	if err := n.cniConf.DelNetworkList(ctx, n.confList, rtConf); err != nil {
		log.Warn().Err(err).Str("exec_id", n.execID).Msg("CNI DEL failed")
	}
	if err := n.netNS.Close(); err != nil {
		log.Warn().Err(err).Str("exec_id", n.execID).Msg("close network namespace handle failed")
	}
	removeNetNS(n.netNS.Path())
	log.Debug().Str("exec_id", n.execID).Msg("CNI network teardown complete")
}

// createNetNS creates a new network namespace pinned to path via a bind mount.
//
// A goroutine locked to a dedicated OS thread:
//  1. Calls unshare(CLONE_NEWNET) to move the thread into a fresh, empty netns.
//  2. Bind-mounts /proc/self/task/<tid>/ns/net onto path to pin the netns.
//
// The goroutine does NOT call runtime.UnlockOSThread — the Go runtime reclaims
// the thread when the goroutine exits. The netns survives because the bind
// mount holds a kernel reference independent of any thread or process.
func createNetNS(path string) error {
	if err := os.MkdirAll(netnsDir, 0o755); err != nil { //nolint:gosec // 0o755 matches standard /var/run/netns permissions
		return fmt.Errorf("create netns dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDONLY, 0o444)
	if err != nil {
		return fmt.Errorf("create netns file: %w", err)
	}
	f.Close() //nolint:errcheck // file is the bind-mount target; close immediately after creation

	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		// Intentionally no UnlockOSThread: after Unshare the thread is in the
		// wrong netns. Letting the goroutine exit forces the runtime to discard
		// the thread rather than returning it to the pool.

		if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
			errCh <- fmt.Errorf("unshare(CLONE_NEWNET): %w", err)
			return
		}
		// /proc/self/task/<tid>/ns/net refers to THIS thread's netns, not the
		// main thread's (/proc/self/ns/net tracks thread 1 on Linux).
		threadNetNSPath := fmt.Sprintf("/proc/self/task/%d/ns/net", unix.Gettid())
		if err := unix.Mount(threadNetNSPath, path, "bind", unix.MS_BIND, ""); err != nil {
			errCh <- fmt.Errorf("bind-mount netns: %w", err)
			return
		}
		errCh <- nil
	}()

	if err := <-errCh; err != nil {
		os.Remove(path) //nolint:errcheck // best-effort: remove the empty file we created
		return err
	}
	return nil
}

// removeNetNS unmounts and removes the pinned netns file at path.
func removeNetNS(path string) {
	if err := unix.Unmount(path, unix.MNT_DETACH); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("unmount netns failed")
	}
	if err := os.Remove(path); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("remove netns file failed")
	}
}
