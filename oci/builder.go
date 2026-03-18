package oci

import (
	"fmt"
	"os"

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

func (b *SpecBuilder) WithCmd(cmd []string) *SpecBuilder {
	b.cmd = cmd
	return b
}

func (b *SpecBuilder) WithEnv(env []string) *SpecBuilder {
	b.env = env
	return b
}

func (b *SpecBuilder) WithCwd(cwd string) *SpecBuilder {
	if cwd != "" {
		b.cwd = cwd
	}
	return b
}

func (b *SpecBuilder) WithLimits(limits config.ResourceLimits) *SpecBuilder {
	b.limits = &limits
	return b
}

// WithMounts appends extra bind mounts (e.g. input files, output dir) to the spec.
func (b *SpecBuilder) WithMounts(mounts []specs.Mount) *SpecBuilder {
	b.extraMounts = append(b.extraMounts, mounts...)
	return b
}

// Build produces a complete, hardened OCI spec.
func (b *SpecBuilder) Build() (*specs.Spec, error) {
	if b.rootfsPath == "" {
		return nil, fmt.Errorf("rootfs path not set")
	}
	if len(b.cmd) == 0 {
		return nil, fmt.Errorf("cmd not set")
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
		{Type: specs.NetworkNamespace},
		{Type: specs.IPCNamespace},
		{Type: specs.UTSNamespace},
		{Type: specs.MountNamespace},
	}

	var uidMappings, gidMappings []specs.LinuxIDMapping
	if rootless {
		namespaces = append(namespaces, specs.LinuxNamespace{Type: specs.UserNamespace})
		hostUID := uint32(b.getUID())
		hostGID := uint32(b.getGID())
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
