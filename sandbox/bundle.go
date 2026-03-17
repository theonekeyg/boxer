package sandbox

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/google/uuid"
)

// NewExecID generates a unique container ID for runsc.
func NewExecID() string {
	return "boxer-" + uuid.NewString()
}

// BundleDir is a temporary directory tree for a single OCI execution.
//
// Layout:
//
//	<state_root>/<exec_id>/
//	  bundle/
//	    config.json
//	  runsc-state/
type BundleDir struct {
	ExecID    string
	execRoot  string
	bundlePath string
}

// NewBundleDir creates the bundle directory tree and writes config.json.
func NewBundleDir(stateRoot, execID string, spec *specs.Spec) (*BundleDir, error) {
	execRoot := filepath.Join(stateRoot, execID)
	bundlePath := filepath.Join(execRoot, "bundle")
	runscState := filepath.Join(execRoot, "runsc-state")

	for _, dir := range []string{bundlePath, runscState} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create bundle dir: %w", err)
		}
	}

	configJSON, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	configPath := filepath.Join(bundlePath, "config.json")
	if err := os.WriteFile(configPath, configJSON, 0o644); err != nil {
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	slog.Debug("created OCI bundle", "exec_id", execID, "config_json", configPath)

	return &BundleDir{
		ExecID:     execID,
		execRoot:   execRoot,
		bundlePath: bundlePath,
	}, nil
}

// BundlePath returns the path to the directory containing config.json.
func (b *BundleDir) BundlePath() string { return b.bundlePath }

// RunscRoot returns the per-execution runsc state directory.
func (b *BundleDir) RunscRoot() string { return filepath.Join(b.execRoot, "runsc-state") }

// Cleanup removes the entire execution root. Errors are logged, not returned.
func (b *BundleDir) Cleanup() {
	if err := os.RemoveAll(b.execRoot); err != nil {
		slog.Warn("failed to remove exec root", "exec_id", b.ExecID, "error", err)
	}
}
