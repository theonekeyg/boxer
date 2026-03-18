package oci

import (
	"encoding/json"
	"os"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"boxer/config"
)

func baseBuilder() *SpecBuilder {
	return NewSpecBuilder("/var/lib/boxer/images/sha256-abc/rootfs", "boxer-test-123").
		WithCmd([]string{"python3", "-c", "print(1)"}).
		WithEnv([]string{"MY_VAR=hello"}).
		WithCwd("/app")
}

func TestBuild_BasicSpec(t *testing.T) {
	spec, err := baseBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", spec.Version)
	}
	expectedUID := uint32(65534)
	if os.Getuid() != 0 {
		expectedUID = 0
	}
	if spec.Process.User.UID != expectedUID {
		t.Errorf("expected UID %d, got %d", expectedUID, spec.Process.User.UID)
	}
	if !spec.Process.NoNewPrivileges {
		t.Error("expected NoNewPrivileges=true")
	}
	if !spec.Root.Readonly {
		t.Error("expected readonly rootfs")
	}
}

func TestBuild_RootlessUserNamespace(t *testing.T) {
	const fakeUID, fakeGID uint32 = 1000, 1001
	spec, err := baseBuilder().
		WithUIDProvider(func() int { return int(fakeUID) }, func() int { return int(fakeGID) }).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	hasUserNS := false
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type == specs.UserNamespace {
			hasUserNS = true
			break
		}
	}
	if !hasUserNS {
		t.Error("expected UserNamespace in rootless mode")
	}
	if len(spec.Linux.UIDMappings) != 1 {
		t.Fatalf("expected 1 UIDMapping, got %d", len(spec.Linux.UIDMappings))
	}
	if len(spec.Linux.GIDMappings) != 1 {
		t.Fatalf("expected 1 GIDMapping, got %d", len(spec.Linux.GIDMappings))
	}
	if spec.Linux.UIDMappings[0].HostID != fakeUID {
		t.Errorf("UID mapping host ID: expected %d, got %d", fakeUID, spec.Linux.UIDMappings[0].HostID)
	}
	if spec.Linux.GIDMappings[0].HostID != fakeGID {
		t.Errorf("GID mapping host ID: expected %d, got %d", fakeGID, spec.Linux.GIDMappings[0].HostID)
	}
}

func TestBuild_AllNilLimits_NoResources(t *testing.T) {
	spec, err := baseBuilder().WithLimits(config.ResourceLimits{}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Linux.Resources != nil {
		t.Errorf("expected nil Resources for all-nil limits, got %+v", spec.Linux.Resources)
	}
}

// TestBuild_OnlyNoFile_NoLinuxResources verifies that setting only NoFile does
// not populate spec.Linux.Resources (NoFile is enforced via RLIMIT_NOFILE on
// spec.Process.Rlimits, not via cgroup resources).
func TestBuild_OnlyNoFile_NoLinuxResources(t *testing.T) {
	nofile := uint64(512)
	spec, err := baseBuilder().WithLimits(config.ResourceLimits{NoFile: &nofile}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Linux.Resources != nil {
		t.Errorf("expected nil Linux.Resources when only NoFile is set, got %+v", spec.Linux.Resources)
	}
	// The value must be enforced as RLIMIT_NOFILE on the process.
	found := false
	for _, r := range spec.Process.Rlimits {
		if r.Type == "RLIMIT_NOFILE" {
			found = true
			if r.Soft != nofile || r.Hard != nofile {
				t.Errorf("RLIMIT_NOFILE: expected soft/hard=%d, got soft=%d hard=%d", nofile, r.Soft, r.Hard)
			}
		}
	}
	if !found {
		t.Error("expected RLIMIT_NOFILE in spec.Process.Rlimits")
	}
}

// TestBuild_OnlyWallClockSecs_NoLinuxResources verifies that WallClockSecs does
// not map into spec.Linux.Resources (there is no cgroup field for wall-clock
// time; enforcement is handled outside the OCI spec). Build must succeed and
// Linux.Resources must remain nil when no other cgroup limits are set.
func TestBuild_OnlyWallClockSecs_NoLinuxResources(t *testing.T) {
	wall := int64(30)
	spec, err := baseBuilder().WithLimits(config.ResourceLimits{WallClockSecs: &wall}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Linux.Resources != nil {
		t.Errorf("expected nil Linux.Resources when only WallClockSecs is set, got %+v", spec.Linux.Resources)
	}
}

func TestBuild_NoCaps(t *testing.T) {
	spec, err := baseBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	caps := spec.Process.Capabilities
	if caps == nil {
		t.Fatal("expected Capabilities field present")
	}
	// All cap sets must be empty (drop all caps).
	for _, set := range [][]string{
		caps.Bounding, caps.Effective, caps.Inheritable, caps.Permitted, caps.Ambient,
	} {
		if len(set) != 0 {
			t.Errorf("expected empty cap set, got %v", set)
		}
	}
}

func TestBuild_StandardMounts(t *testing.T) {
	spec, err := baseBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	dests := map[string]bool{}
	for _, m := range spec.Mounts {
		dests[m.Destination] = true
	}
	for _, required := range []string{"/proc", "/dev", "/sys", "/tmp"} {
		if !dests[required] {
			t.Errorf("missing required mount: %s", required)
		}
	}
}

func TestBuild_CPUQuotaMath(t *testing.T) {
	cpu := 0.5
	limits := config.ResourceLimits{CPUCores: &cpu}
	spec, err := baseBuilder().WithLimits(limits).Build()
	if err != nil {
		t.Fatal(err)
	}
	cpuRes := spec.Linux.Resources.CPU
	if cpuRes == nil {
		t.Fatal("expected CPU resources")
	}
	if cpuRes.Period == nil || *cpuRes.Period != 100_000 {
		t.Errorf("expected period=100000, got %v", cpuRes.Period)
	}
	if cpuRes.Quota == nil || *cpuRes.Quota != 50_000 {
		t.Errorf("expected quota=50000, got %v", cpuRes.Quota)
	}
}

func TestBuild_MemoryBytes(t *testing.T) {
	mem := int64(256)
	limits := config.ResourceLimits{MemoryMB: &mem}
	spec, err := baseBuilder().WithLimits(limits).Build()
	if err != nil {
		t.Fatal(err)
	}
	memRes := spec.Linux.Resources.Memory
	if memRes == nil {
		t.Fatal("expected Memory resources")
	}
	if memRes.Limit == nil || *memRes.Limit != 256*1024*1024 {
		t.Errorf("expected 256 MiB, got %v", memRes.Limit)
	}
}

func TestBuild_PidsLimit(t *testing.T) {
	pids := int64(64)
	limits := config.ResourceLimits{PidsLimit: &pids}
	spec, err := baseBuilder().WithLimits(limits).Build()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Linux.Resources.Pids == nil || spec.Linux.Resources.Pids.Limit != 64 {
		t.Error("expected pids limit=64")
	}
}

func TestBuild_Namespaces(t *testing.T) {
	spec, err := baseBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	nsTypes := map[specs.LinuxNamespaceType]bool{}
	for _, ns := range spec.Linux.Namespaces {
		nsTypes[ns.Type] = true
	}
	for _, required := range []specs.LinuxNamespaceType{
		specs.PIDNamespace,
		specs.NetworkNamespace,
		specs.IPCNamespace,
		specs.UTSNamespace,
		specs.MountNamespace,
	} {
		if !nsTypes[required] {
			t.Errorf("missing namespace: %s", required)
		}
	}
}

func TestBuild_PathEnvFallback(t *testing.T) {
	// No PATH in env — builder should prepend a default PATH.
	spec, err := NewSpecBuilder("/rootfs", "exec-1").
		WithCmd([]string{"sh"}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	hasPath := false
	for _, e := range spec.Process.Env {
		if len(e) >= 5 && e[:5] == "PATH=" {
			hasPath = true
		}
	}
	if !hasPath {
		t.Error("expected fallback PATH in env")
	}
}

func TestBuild_JSONRoundtrip(t *testing.T) {
	spec, err := baseBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var back specs.Spec
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Version != "1.0.0" {
		t.Errorf("roundtrip version mismatch: %s", back.Version)
	}
}

func TestBuild_MissingRootfs(t *testing.T) {
	_, err := NewSpecBuilder("", "exec-1").WithCmd([]string{"sh"}).Build()
	if err == nil {
		t.Error("expected error for empty rootfs")
	}
}

func TestBuild_MissingCmd(t *testing.T) {
	_, err := NewSpecBuilder("/rootfs", "exec-1").Build()
	if err == nil {
		t.Error("expected error for empty cmd")
	}
}

func TestBuild_WithMounts_AppearsInSpec(t *testing.T) {
	extra := []specs.Mount{
		{Source: "/host/input.py", Destination: "/input.py", Type: "bind", Options: []string{"rbind", "ro"}},
		{Source: "/host/output", Destination: "/output", Type: "bind", Options: []string{"rbind", "rw"}},
	}
	spec, err := baseBuilder().WithMounts(extra).Build()
	if err != nil {
		t.Fatal(err)
	}
	dests := map[string]specs.Mount{}
	for _, m := range spec.Mounts {
		dests[m.Destination] = m
	}
	for _, want := range extra {
		got, ok := dests[want.Destination]
		if !ok {
			t.Errorf("expected mount at %s not found", want.Destination)
			continue
		}
		if got.Source != want.Source {
			t.Errorf("mount %s source: want %s, got %s", want.Destination, want.Source, got.Source)
		}
	}
	// Standard mounts must still be present.
	for _, required := range []string{"/proc", "/dev", "/sys", "/tmp"} {
		if _, ok := dests[required]; !ok {
			t.Errorf("standard mount %s missing after WithMounts", required)
		}
	}
}
