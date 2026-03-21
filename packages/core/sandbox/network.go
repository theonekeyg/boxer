//go:build linux

package sandbox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
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
	bridgeMu    sync.Mutex
	bridgeReady bool
	ipCounter   atomic.Uint32

	// execIDRe allows letters, digits, hyphens, underscores, and dots in
	// execIDs used to build filesystem paths (netns bind-mount targets).
	// The explicit ".." check below further guards against path traversal.
	execIDRe = regexp.MustCompile(`^[a-zA-Z0-9_.+-]+$`)
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
	// Validate execID before using it in a filesystem path. The regex rejects
	// slashes; the explicit check rejects "..". Together they prevent path traversal.
	if !execIDRe.MatchString(execID) || execID == ".." {
		return nil, fmt.Errorf("invalid execID %q: must match [a-zA-Z0-9_.+-]+ and not be \"..\"", execID)
	}

	// Ensure the bridge, IP forwarding, and nftables rules exist. Unlike
	// sync.Once, this mutex-based guard retries on failure so a transient
	// error (e.g., nftables not yet ready at boot) does not permanently break
	// all subsequent containers in this process.
	bridgeMu.Lock()
	if !bridgeReady {
		if err := ensureBridge(); err != nil {
			bridgeMu.Unlock()
			return nil, fmt.Errorf("setup bridge: %w", err)
		}
		bridgeReady = true
	}
	bridgeMu.Unlock()

	// Allocate a unique IP and a veth name derived from a monotonic counter.
	// IPs cycle through 10.88.0.2 – 10.88.255.254 (65533 addresses). The
	// counter itself is a uint32 that never wraps in practice (would require
	// ~4 billion increments), but the IP range wraps every 65,533 allocations.
	// If more than 65,533 containers are running simultaneously, a new container
	// may receive an IP already assigned to a live container. In practice boxer
	// is not designed for that scale; operators requiring collision-free IPs at
	// very high concurrency should assign a larger subnet or implement an
	// in-use IP bitmap.
	n := ipCounter.Add(1)
	ipN := (n-2)%65533 + 2
	containerIP := fmt.Sprintf("10.88.%d.%d/16", ipN>>8, ipN&0xff)
	vethHost := fmt.Sprintf("veth%04x", n%0x10000)

	netnsPath := filepath.Join(netnsDir, execID)
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

	// Write a resolv.conf for the container. We use the host's /etc/resolv.conf
	// so container DNS respects host policies. An override path can be supplied
	// via the SANDBOX_RESOLV_CONF environment variable. If both are unavailable
	// we fall back to public DNS so the container remains functional.
	resolvConf := netnsPath + ".resolv.conf"
	if err := os.WriteFile(resolvConf, sandboxResolvContents(), 0o444); err != nil {
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
// forwarding, and installs nftables rules. All steps are idempotent so it is
// safe to call on a host where a previous boxer process already configured the
// bridge.
//
// NOTE: enabling ip_forward is a global, persistent host-level change. For
// production deployments operators should pre-configure this via sysctl.conf
// (net.ipv4.ip_forward = 1) rather than relying on boxer to set it at runtime.
func ensureBridge() error {
	// Only write ip_forward if it is not already enabled; the write is a
	// global, persistent host change so we avoid it when unnecessary.
	if cur, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); err == nil {
		if len(cur) > 0 && cur[0] == '1' {
			log.Debug().Msg("ip_forward already enabled")
		} else {
			log.Info().Msg("enabling ip_forward (global host setting; consider setting net.ipv4.ip_forward=1 in sysctl.conf)")
			if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
				return fmt.Errorf("enable ip_forward: %w", err)
			}
		}
	} else {
		if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
			return fmt.Errorf("enable ip_forward: %w", err)
		}
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

	return ensureNftablesRules()
}

// ensureNftablesRules installs nftables FORWARD ACCEPT and POSTROUTING
// MASQUERADE rules for the boxer0 bridge. Rules are placed at chain priority
// -10, which runs before firewalld's priority-0 chains, making this approach
// work on all hosts regardless of whether firewalld, ufw, or plain iptables
// is in use. The function is idempotent: if our table already exists it
// returns immediately.
func ensureNftablesRules() error {
	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("open nftables: %w", err)
	}

	// Idempotency: skip setup only when both our tables are present. Checking
	// just one table would silently accept a half-configured state (e.g., a
	// crash after boxer_filter was created but before boxer_nat was).
	//
	// TODO: also inspect chains inside each table so a partial config is
	// detected and repaired. Example broken sequence: (1) boxer creates both
	// tables successfully and sets bridgeReady=true; (2) an operator runs
	// "nft flush table inet boxer_filter", which removes all chains and rules
	// but leaves the table itself; (3) boxer restarts — this check sees both
	// tables, returns nil, and all subsequent containers silently lose their
	// FORWARD ACCEPT rules for the lifetime of the process.
	//
	// We intentionally leave this as a TODO rather than adding per-container
	// chain inspection: ensureNftablesRules is called once per process (guarded
	// by bridgeMu/bridgeReady), the interference scenario requires deliberate
	// external action, and a process restart trivially restores correct state.
	// The cost of a ListChains round-trip on every SetupNetwork call is not
	// justified by the threat model.
	tables, err := c.ListTables()
	if err != nil {
		return fmt.Errorf("list nftables tables: %w", err)
	}
	var hasFilter, hasNat bool
	for _, t := range tables {
		if t.Name == "boxer_filter" {
			hasFilter = true
		}
		if t.Name == "boxer_nat" {
			hasNat = true
		}
	}
	if hasFilter && hasNat {
		return nil
	}

	// inet boxer_filter — priority -10 runs before firewalld (priority 0).
	filterTable := &nftables.Table{Name: "boxer_filter", Family: nftables.TableFamilyINet}
	c.AddTable(filterTable)
	filterChain := c.AddChain(&nftables.Chain{
		Name:     "forward",
		Table:    filterTable,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityRef(-10),
	})

	// iifname "boxer0" accept
	c.AddRule(&nftables.Rule{
		Table: filterTable, Chain: filterChain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifname(bridgeName)},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
	// oifname "boxer0" accept
	c.AddRule(&nftables.Rule{
		Table: filterTable, Chain: filterChain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifname(bridgeName)},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})

	// ip boxer_nat — MASQUERADE for the boxer subnet.
	natTable := &nftables.Table{Name: "boxer_nat", Family: nftables.TableFamilyIPv4}
	c.AddTable(natTable)
	natChain := c.AddChain(&nftables.Chain{
		Name:     "postrouting",
		Table:    natTable,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPostrouting,
		Priority: nftables.ChainPriorityNATSource,
	})

	// ip saddr 10.88.0.0/16 oifname != "boxer0" masquerade
	_, subnet, _ := net.ParseCIDR(bridgeSubnet) //nolint:errcheck // constant is valid
	c.AddRule(&nftables.Rule{
		Table: natTable, Chain: natChain,
		Exprs: []expr.Any{
			// match source IP against 10.88.0.0/16
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 12, Len: 4},
			&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: 4, Mask: subnet.Mask, Xor: []byte{0, 0, 0, 0}},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: subnet.IP.To4()},
			// oifname != "boxer0"
			&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ifname(bridgeName)},
			// masquerade
			&expr.Masq{},
		},
	})

	if err := c.Flush(); err != nil {
		return fmt.Errorf("apply nftables rules: %w", err)
	}
	return nil
}

// sandboxResolvContents returns DNS configuration for the container resolv.conf.
// Priority: SANDBOX_RESOLV_CONF env → host /etc/resolv.conf → public DNS fallback.
func sandboxResolvContents() []byte {
	if path := os.Getenv("SANDBOX_RESOLV_CONF"); path != "" {
		if data, err := os.ReadFile(path); err == nil { //nolint:gosec // path from trusted env var
			return data
		}
	}
	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		return data
	}
	return []byte("nameserver 8.8.8.8\nnameserver 8.8.4.4\n")
}

// ifname returns a 16-byte interface name suitable for nftables comparisons.
func ifname(n string) []byte {
	b := make([]byte, 16)
	copy(b, n+"\x00")
	return b
}

// setupVeth creates a veth pair, attaches the host side to boxer0, moves the
// container side into nsFd, then configures the container interface with the
// given IP, brings up lo and eth0, and adds a default route via the bridge.
func setupVeth(hostName, containerIP string, nsFd int) error {
	// Use a unique temporary name for the peer so it doesn't collide with
	// any existing "eth0" in the host namespace. We rename it to "eth0"
	// after moving it into the container netns.
	peerTmpName := hostName + "p"
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostName},
		PeerName:  peerTmpName,
	}
	// Delete any stale veth with the same host-side name left over from a
	// previous boxer process that exited without calling Teardown.
	if stale, err := netlink.LinkByName(hostName); err == nil {
		netlink.LinkDel(stale) //nolint:errcheck // best-effort stale cleanup
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("create veth pair: %w", err)
	}

	hostLink, err := netlink.LinkByName(hostName)
	if err != nil {
		return fmt.Errorf("get host veth: %w", err)
	}

	contLink, err := netlink.LinkByName(peerTmpName)
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

	// Rename the peer from its temporary name to "eth0" inside the container netns.
	peerLink, err := nlHandle.LinkByName(peerTmpName)
	if err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("get peer in netns: %w", err)
	}
	if err := nlHandle.LinkSetName(peerLink, "eth0"); err != nil {
		netlink.LinkDel(hostLink) //nolint:errcheck
		return fmt.Errorf("rename peer to eth0: %w", err)
	}

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
