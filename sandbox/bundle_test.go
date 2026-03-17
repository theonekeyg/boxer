package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func minimalSpec() *specs.Spec {
	noNewPriv := true
	readonly := true
	uid := uint32(65534)
	gid := uint32(65534)
	return &specs.Spec{
		Version: "1.0.0",
		Process: &specs.Process{
			Args:            []string{"sh"},
			Env:             []string{"PATH=/bin"},
			Cwd:             "/",
			NoNewPrivileges: noNewPriv,
			User:            specs.User{UID: uid, GID: gid},
			Capabilities:    &specs.LinuxCapabilities{},
		},
		Root: &specs.Root{Path: "/rootfs", Readonly: readonly},
	}
}

func TestNewBundleDir_CreatesDirectories(t *testing.T) {
	stateRoot := t.TempDir()
	execID := NewExecID()
	spec := minimalSpec()

	bundle, err := NewBundleDir(stateRoot, execID, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Cleanup()

	// bundle dir must exist
	if info, err := os.Stat(bundle.BundlePath()); err != nil || !info.IsDir() {
		t.Error("bundle path does not exist or is not a directory")
	}
	// runsc-state must exist
	if info, err := os.Stat(bundle.RunscRoot()); err != nil || !info.IsDir() {
		t.Error("runsc-state does not exist or is not a directory")
	}
}

func TestNewBundleDir_ConfigJSONValid(t *testing.T) {
	stateRoot := t.TempDir()
	bundle, err := NewBundleDir(stateRoot, NewExecID(), minimalSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Cleanup()

	data, err := os.ReadFile(filepath.Join(bundle.BundlePath(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}
	if v["ociVersion"] != "1.0.0" {
		t.Errorf("unexpected ociVersion: %v", v["ociVersion"])
	}
}

func TestBundleDir_CleanupRemovesRoot(t *testing.T) {
	stateRoot := t.TempDir()
	bundle, err := NewBundleDir(stateRoot, NewExecID(), minimalSpec())
	if err != nil {
		t.Fatal(err)
	}
	execRoot := filepath.Dir(bundle.BundlePath())
	if _, err := os.Stat(execRoot); err != nil {
		t.Fatal("exec root should exist before Cleanup")
	}
	bundle.Cleanup()
	if _, err := os.Stat(execRoot); !os.IsNotExist(err) {
		t.Error("exec root should be removed after Cleanup")
	}
}

func TestNewExecID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for range 50 {
		id := NewExecID()
		if ids[id] {
			t.Fatalf("duplicate exec ID: %s", id)
		}
		ids[id] = true
		if !strings.HasPrefix(id, "boxer-") {
			t.Errorf("exec ID missing 'boxer-' prefix: %s", id)
		}
	}
}

func TestBundleDir_RunscRootInsideExecRoot(t *testing.T) {
	stateRoot := t.TempDir()
	bundle, err := NewBundleDir(stateRoot, NewExecID(), minimalSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Cleanup()

	execRoot := filepath.Dir(bundle.BundlePath())
	if !strings.HasPrefix(bundle.RunscRoot(), execRoot) {
		t.Errorf("runsc root %s is not inside exec root %s", bundle.RunscRoot(), execRoot)
	}
}
