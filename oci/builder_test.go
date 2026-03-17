package oci

import (
	"encoding/json"
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
	if spec.Process.User.UID != 65534 {
		t.Errorf("expected UID 65534, got %d", spec.Process.User.UID)
	}
	if !spec.Process.NoNewPrivileges {
		t.Error("expected NoNewPrivileges=true")
	}
	if !spec.Root.Readonly {
		t.Error("expected readonly rootfs")
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
