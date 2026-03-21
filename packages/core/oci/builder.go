// Package oci constructs hardened OCI runtime specs for gVisor sandboxes.
package oci

import (
	"fmt"
	"os"
	"path/filepath"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"boxer/config"
)

// SpecBuilder constructs a hardened OCI runtime spec.
type SpecBuilder struct {
	rootfsPath  string
	execID      string
	cmd         []string
	env         []string
	cwd         string
	network     string
	netnsPath   string // pre-configured netns path for sandbox/host modes
	limits      *config.ResourceLimits
	extraMounts []specs.Mount
	getUID      func() int
	getGID      func() int
}

// NewSpecBuilder creates a SpecBuilder for the given rootfs path and execution ID.
func NewSpecBuilder(rootfsPath, execID string) *SpecBuilder {
	return &SpecBuilder{
		rootfsPath: rootfsPath,
		execID:     execID,
		cwd:        "/",
		getUID:     os.Getuid,
		getGID:     os.Getgid,
	}
}

// WithUIDProvider overrides the functions used to obtain the current process's
// UID and GID. This is primarily useful in tests to simulate rootless mode
// without actually running as a non-root user.
func (b *SpecBuilder) WithUIDProvider(getUID, getGID func() int) *SpecBuilder {
	b.getUID = getUID
	b.getGID = getGID
	return b
}

// WithCmd sets the command and arguments to run inside the container.
func (b *SpecBuilder) WithCmd(cmd []string) *SpecBuilder {
	b.cmd = cmd
	return b
}

// WithEnv sets additional environment variables for the container process.
func (b *SpecBuilder) WithEnv(env []string) *SpecBuilder {
	b.env = env
	return b
}

// WithCwd sets the working directory for the container process.
func (b *SpecBuilder) WithCwd(cwd string) *SpecBuilder {
	if cwd != "" {
		b.cwd = cwd
	}
	return b
}

// WithLimits applies resource limits (CPU, memory, pids, wall clock, nofile) to the spec.
func (b *SpecBuilder) WithLimits(limits config.ResourceLimits) *SpecBuilder {
	b.limits = &limits
	return b
}

// WithNetwork sets the network mode (none, sandbox, host).
func (b *SpecBuilder) WithNetwork(network string) *SpecBuilder {
	b.network = network
	return b
}

// WithNetworkNamespacePath sets the path of a pre-configured network namespace
// to be joined by the container. Required when network is "sandbox" or "host":
// gVisor will enter this namespace (which must have been set up by CNI) instead
// of creating a new, empty one.
func (b *SpecBuilder) WithNetworkNamespacePath(path string) *SpecBuilder {
	b.netnsPath = path
	return b
}

// WithMounts appends extra bind mounts (e.g. input files, output dir) to the spec.
func (b *SpecBuilder) WithMounts(mounts []specs.Mount) *SpecBuilder {
	b.extraMounts = append(b.extraMounts, mounts...)
	return b
}

// Build produces a complete, hardened OCI spec.
//
//nolint:funlen // Build constructs a complete OCI spec with many required fields; splitting would obscure the structure
func (b *SpecBuilder) Build() (*specs.Spec, error) {
	if b.rootfsPath == "" {
		return nil, fmt.Errorf("rootfs path not set")
	}
	if len(b.cmd) == 0 {
		return nil, fmt.Errorf("cmd not set")
	}
	if err := b.validateExtraMounts(); err != nil {
		return nil, err
	}

	env := b.env
	// Always ensure PATH is available.
	hasPath := false
	for _, e := range env {
		if len(e) >= 5 && e[:5] == "PATH=" {
			hasPath = true
			break
		}
	}
	if !hasPath {
		env = append([]string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}, env...)
	}

	// Rootless mode: when boxer itself is not running as root we must create a
	// user namespace so that gVisor's gofer process can set up mount namespaces
	// without CAP_SYS_ADMIN on the host.
	//
	// Without an explicit UIDMapping in the OCI spec, gVisor defaults to a
	// full identity mapping (0→0, size=4294967295). newuidmap(1) rejects that
	// for unprivileged users because they do not own UID 0 on the host. We
	// supply a single-entry mapping — host UID → container UID 0 — which
	// newuidmap accepts as long as the entry falls within the caller's own UID.
	//
	// Inside the user namespace, UID 0 has no real root privileges on the host;
	// it is confined to the namespace and further isolated by gVisor itself.
	//
	// When running as root (production), we skip the user namespace entirely
	// and drop the container process to nobody (65534) instead.
	rootless := b.getUID() != 0

	var uid, gid uint32
	if rootless {
		uid, gid = 0, 0
	} else {
		uid, gid = 65534, 65534
	}

	noNewPriv := true
	readonly := true

	nofile := uint64(256)
	if b.limits != nil && b.limits.NoFile != nil {
		nofile = *b.limits.NoFile
	}

	namespaces := []specs.LinuxNamespace{
		{Type: specs.PIDNamespace},
		{Type: specs.IPCNamespace},
		{Type: specs.UTSNamespace},
		{Type: specs.MountNamespace},
	}
	// For sandbox/host modes, join the pre-configured CNI netns so gVisor's
	// netstack has a real veth interface to route traffic through. A non-empty
	// path is required: gVisor reads it from the OCI spec and calls setns(2).
	// For none (default), include a fresh isolated namespace with no interfaces.
	switch b.network {
	case "sandbox", "host":
		if b.netnsPath == "" {
			return nil, fmt.Errorf("network=%q requires a pre-configured network namespace path", b.network)
		}
		namespaces = append(namespaces, specs.LinuxNamespace{
			Type: specs.NetworkNamespace,
			Path: b.netnsPath,
		})
	default:
		namespaces = append(namespaces, specs.LinuxNamespace{Type: specs.NetworkNamespace})
	}

	var uidMappings, gidMappings []specs.LinuxIDMapping
	if rootless {
		namespaces = append(namespaces, specs.LinuxNamespace{Type: specs.UserNamespace})
		hostUID := uint32(b.getUID()) //nolint:gosec // UIDs on Linux fit in uint32
		hostGID := uint32(b.getGID()) //nolint:gosec // GIDs on Linux fit in uint32
		uidMappings = []specs.LinuxIDMapping{{ContainerID: 0, HostID: hostUID, Size: 1}}
		gidMappings = []specs.LinuxIDMapping{{ContainerID: 0, HostID: hostGID, Size: 1}}
	}

	spec := &specs.Spec{
		Version:  "1.0.0",
		Hostname: b.execID,
		Process: &specs.Process{
			User:            specs.User{UID: uid, GID: gid},
			Args:            b.cmd,
			Env:             env,
			Cwd:             b.cwd,
			Capabilities:    &specs.LinuxCapabilities{},
			NoNewPrivileges: noNewPriv,
			Rlimits: []specs.POSIXRlimit{
				{Type: "RLIMIT_NOFILE", Hard: nofile, Soft: nofile},
			},
		},
		Root: &specs.Root{
			Path:     b.rootfsPath,
			Readonly: readonly,
		},
		Mounts: append(standardMounts(), b.extraMounts...),
		Linux: &specs.Linux{
			Namespaces:  namespaces,
			UIDMappings: uidMappings,
			GIDMappings: gidMappings,
			Resources:   b.buildResources(),
		},
	}

	return spec, nil
}

func (b *SpecBuilder) validateExtraMounts() error {
	reserved := make(map[string]bool, len(standardMounts()))
	for _, m := range standardMounts() {
		reserved[filepath.Clean(m.Destination)] = true
	}

	seen := make(map[string]bool, len(b.extraMounts))
	for i := range b.extraMounts {
		if b.extraMounts[i].Destination == "" {
			return fmt.Errorf("mount has empty destination")
		}
		if !filepath.IsAbs(b.extraMounts[i].Destination) {
			return fmt.Errorf("mount destination must be absolute: %q", b.extraMounts[i].Destination)
		}
		clean := filepath.Clean(b.extraMounts[i].Destination)
		if reserved[clean] {
			return fmt.Errorf("mount destination conflicts with reserved mount: %q", clean)
		}
		if seen[clean] {
			return fmt.Errorf("duplicate mount destination: %q", clean)
		}
		seen[clean] = true
		b.extraMounts[i].Destination = clean
	}
	return nil
}

func (b *SpecBuilder) buildResources() *specs.LinuxResources {
	if b.limits == nil {
		return nil
	}
	res := &specs.LinuxResources{}

	if b.limits.CPUCores != nil {
		period := uint64(100_000)
		quota := int64(*b.limits.CPUCores * float64(period))
		res.CPU = &specs.LinuxCPU{
			Period: &period,
			Quota:  &quota,
		}
	}
	if b.limits.MemoryMB != nil {
		limit := *b.limits.MemoryMB * 1024 * 1024
		res.Memory = &specs.LinuxMemory{Limit: &limit}
	}
	if b.limits.PidsLimit != nil {
		res.Pids = &specs.LinuxPids{Limit: *b.limits.PidsLimit}
	}

	if res.CPU == nil && res.Memory == nil && res.Pids == nil {
		return nil
	}
	return res
}

// standardMounts returns the required mounts every sandbox gets.
func standardMounts() []specs.Mount {
	return []specs.Mount{
		{
			Destination: "/proc",
			Type:        "proc",
			Source:      "proc",
		},
		{
			Destination: "/dev",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "noexec", "mode=755", "size=65536k"},
		},
		{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "ro"},
		},
		{
			Destination: "/tmp",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "nodev", "size=65536k"},
		},
	}
}
