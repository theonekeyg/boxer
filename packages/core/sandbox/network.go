package sandbox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

const (
	bridgeName   = "boxer0"
	bridgeAddr   = "10.88.0.1/16"
	bridgeSubnet = "10.88.0.0/16"
	bridgeGW     = "10.88.0.1"
	netnsDir     = "/var/run/netns"
)

var (
	bridgeOnce sync.Once
	bridgeErr  error
	ipCounter  atomic.Uint32
)

func init() {
	ipCounter.Store(1) // first Add(1) → 2, the first usable host address
}

// NetworkSetup holds the resources allocated for a container's network.
// Call Teardown after the container exits to release them.
type NetworkSetup struct {
	netnsPath     string
	resolvConf    string // path to written resolv.conf; empty if not created
	vethName      string // host-side veth; deleting it also removes the container peer
}

// NetNSPath returns the filesystem path of the pinned network namespace.
func (n *NetworkSetup) NetNSPath() string { return n.netnsPath }

// ResolvConfPath returns the host path of the resolv.conf written for this
// container, so callers can bind-mount it to /etc/resolv.conf.
func (n *NetworkSetup) ResolvConfPath() string { return n.resolvConf }

// SetupNetwork creates a pinned network namespace and wires it to the host via
// a veth pair connected to the boxer0 bridge. The container gets an IP in
// 10.88.0.0/16 and a default route through the bridge, which NAT-masquerades
// outbound traffic to the internet.
//
// Requires CAP_NET_ADMIN (running as root in practice).
func SetupNetwork(execID string) (*NetworkSetup, error) {
	// Ensure the bridge, IP forwarding, and iptables rule exist (once per process).
	bridgeOnce.Do(func() { bridgeErr = ensureBridge() })
	if bridgeErr != nil {
		return nil, fmt.Errorf("setup bridge: %w", bridgeErr)
	}

	// Allocate a unique IP and a veth name derived from a monotonic counter.
	// IPs cycle through 10.88.0.2 – 10.88.255.254 (65533 addresses).
	n := ipCounter.Add(1)
	ipN := (n-2)%65533 + 2
	containerIP := fmt.Sprintf("10.88.%d.%d/16", ipN>>8, ipN&0xff)
	vethHost := fmt.Sprintf("veth%04x", n%0x10000)

	netnsPath := fmt.Sprintf("%s/%s", netnsDir, execID)
	if err := createNetNS(netnsPath); err != nil {
		return nil, fmt.Errorf("create netns: %w", err)
	}

	nsFd, err := unix.Open(netnsPath, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		removeNetNS(netnsPath)
		return nil, fmt.Errorf("open netns: %w", err)
	}
	defer unix.Close(nsFd) //nolint:errcheck // fd used only during setup

	if err := setupVeth(vethHost, containerIP, nsFd); err != nil {
		removeNetNS(netnsPath)
		return nil, err
	}

	// Write a resolv.conf with public DNS so the container can resolve names.
	resolvConf := netnsPath + ".resolv.conf"
	const resolvContents = "nameserver 8.8.8.8\nnameserver 8.8.4.4\n"
	if err := os.WriteFile(resolvConf, []byte(resolvContents), 0o444); err != nil {
		removeNetNS(netnsPath)
		return nil, fmt.Errorf("write resolv.conf: %w", err)
	}

	log.Debug().Str("exec_id", execID).Str("ip", containerIP).
		Str("veth", vethHost).Str("netns", netnsPath).Msg("network setup complete")

	return &NetworkSetup{netnsPath: netnsPath, resolvConf: resolvConf, vethName: vethHost}, nil
}

// Teardown deletes the host-side veth (which also removes its container peer),
// removes the resolv.conf file, and unmounts the pinned network namespace.
// Errors are logged, not returned.
func (n *NetworkSetup) Teardown() {
	if link, err := netlink.LinkByName(n.vethName); err == nil {
		if err := netlink.LinkDel(link); err != nil {
			log.Warn().Err(err).Str("veth", n.vethName).Msg("delete veth failed")
		}
	}
	if n.resolvConf != "" {
		if err := os.Remove(n.resolvConf); err != nil && !os.IsNotExist(err) {
			log.Warn().Err(err).Str("path", n.resolvConf).Msg("remove resolv.conf failed")
		}
	}
	removeNetNS(n.netnsPath)
	log.Debug().Str("netns", n.netnsPath).Msg("network teardown complete")
}

// ensureBridge creates the boxer0 bridge with its gateway IP, enables IPv4
// forwarding, and installs the iptables masquerade rule. All steps are
// idempotent so it is safe to call on a host where a previous boxer process
// already configured the bridge.
func ensureBridge() error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
		return fmt.Errorf("enable ip_forward: %w", err)
	}

	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		newBr := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
		if err := netlink.LinkAdd(newBr); err != nil {
			return fmt.Errorf("create bridge %s: %w", bridgeName, err)
		}
		br, err = netlink.LinkByName(bridgeName)
		if err != nil {
			return fmt.Errorf("get bridge after create: %w", err)
		}
	}

	addr, err := netlink.ParseAddr(bridgeAddr)
	if err != nil {
		return fmt.Errorf("parse bridge addr: %w", err)
	}
	if err := netlink.AddrAdd(br, addr); err != nil && !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("assign bridge IP: %w", err)
	}

	if err := netlink.LinkSetUp(br); err != nil {
		return fmt.Errorf("bring bridge up: %w", err)
	}

	return ensureMasquerade()
}

// setupVeth creates a veth pair, attaches the host side to boxer0, moves the
// container side into nsFd, then configures the container interface with the
// given IP, brings up lo and eth0, and adds a default route via the bridge.
func setupVeth(hostName, containerIP string, nsFd int) error {
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostName},
		PeerName:  "eth0",
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("create veth pair: %w", err)
	}

	hostLink, err := netlink.LinkByName(hostName)
	if err != nil {
		return fmt.Errorf("get host veth: %w", err)
	}

	contLink, err := netlink.LinkByName("eth0")
	if err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("get container veth peer: %w", err)
	}

	bridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("get bridge: %w", err)
	}
	if err := netlink.LinkSetMaster(hostLink, bridge); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("attach veth to bridge: %w", err)
	}
	if err := netlink.LinkSetUp(hostLink); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("bring host veth up: %w", err)
	}

	if err := netlink.LinkSetNsFd(contLink, nsFd); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("move veth into netns: %w", err)
	}

	// Open a netlink handle scoped to the container netns to configure eth0
	// without entering (setns-ing) the namespace in the calling goroutine.
	nsHandle := netns.NsHandle(nsFd)
	nlHandle, err := netlink.NewHandleAt(nsHandle)
	if err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("open netlink handle for netns: %w", err)
	}
	defer nlHandle.Close()

	eth0, err := nlHandle.LinkByName("eth0")
	if err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("get eth0 in netns: %w", err)
	}

	addr, err := netlink.ParseAddr(containerIP)
	if err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("parse container IP: %w", err)
	}
	if err := nlHandle.AddrAdd(eth0, addr); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("assign container IP: %w", err)
	}
	if err := nlHandle.LinkSetUp(eth0); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("bring eth0 up: %w", err)
	}

	lo, err := nlHandle.LinkByName("lo")
	if err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("get lo in netns: %w", err)
	}
	if err := nlHandle.LinkSetUp(lo); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("bring lo up: %w", err)
	}

	gw := net.ParseIP(bridgeGW)
	if err := nlHandle.RouteAdd(&netlink.Route{
		LinkIndex: eth0.Attrs().Index,
		Gw:        gw,
	}); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("add default route: %w", err)
	}

	return nil
}

// ensureMasquerade installs a single iptables POSTROUTING masquerade rule for
// the boxer subnet. Uses -C to check first so the rule is never duplicated.
func ensureMasquerade() error {
	args := []string{
		"-t", "nat", "-C", "POSTROUTING",
		"-s", bridgeSubnet, "!", "-o", bridgeName, "-j", "MASQUERADE",
	}
	if exec.Command("iptables", args...).Run() == nil { //nolint:gosec // args are all constants
		return nil // rule already present
	}
	args[2] = "-A"
	out, err := exec.Command("iptables", args...).CombinedOutput() //nolint:gosec
	if err != nil {
		return fmt.Errorf("iptables masquerade: %w: %s", err, out)
	}
	return nil
}

// createNetNS creates a new network namespace pinned to path via a bind mount.
//
// A goroutine locked to a dedicated OS thread:
//  1. Calls unshare(CLONE_NEWNET) to move the thread into a fresh, empty netns.
//  2. Bind-mounts /proc/self/task/<tid>/ns/net onto path to pin the netns.
//
// The goroutine does NOT call runtime.UnlockOSThread — the Go runtime reclaims
// the thread on goroutine exit. The netns survives because the bind mount holds
// a kernel reference independent of any thread or process.
func createNetNS(path string) error {
	if err := os.MkdirAll(netnsDir, 0o755); err != nil { //nolint:gosec // 0o755 matches standard /var/run/netns permissions
		return fmt.Errorf("create netns dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDONLY, 0o444)
	if err != nil {
		return fmt.Errorf("create netns file: %w", err)
	}
	f.Close() //nolint:errcheck // bind-mount target; close immediately after creation

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
		// /proc/self/task/<tid>/ns/net is THIS thread's netns.
		// /proc/self/ns/net tracks thread 1 on Linux and would be wrong here.
		threadNetNSPath := fmt.Sprintf("/proc/self/task/%d/ns/net", unix.Gettid())
		if err := unix.Mount(threadNetNSPath, path, "bind", unix.MS_BIND, ""); err != nil {
			errCh <- fmt.Errorf("bind-mount netns: %w", err)
			return
		}
		errCh <- nil
	}()

	if err := <-errCh; err != nil {
		os.Remove(path) //nolint:errcheck
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
