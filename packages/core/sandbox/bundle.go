// Package sandbox manages OCI bundle directories and runs containers via gVisor runsc.
package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
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
	outputDir := filepath.Join(execRoot, "output")

	for _, dir := range []string{bundlePath, runscState, outputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // 0o755 required for bundle directories
			return nil, fmt.Errorf("create bundle dir: %w", err)
		}
	}

	configJSON, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	configPath := filepath.Join(bundlePath, "config.json")
	if err := os.WriteFile(configPath, configJSON, 0o644); err != nil { //nolint:gosec // config.json is not secret; 0o644 allows runsc to read it
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	log.Debug().Str("exec_id", execID).Str("config_json", configPath).Msg("created OCI bundle")

	return &BundleDir{
		ExecID:     execID,
		execRoot:   execRoot,
		bundlePath: bundlePath,
	}, nil
}

// OutputPath returns the output directory path for the given execution before
// the bundle is created. Use this when the path is needed prior to NewBundleDir
// (e.g. to build OCI spec mounts). Must stay in sync with NewBundleDir's layout.
func OutputPath(stateRoot, execID string) string {
	return filepath.Join(stateRoot, execID, "output")
}

// BundlePath returns the path to the directory containing config.json.
func (b *BundleDir) BundlePath() string { return b.bundlePath }

// RunscRoot returns the per-execution runsc state directory.
func (b *BundleDir) RunscRoot() string { return filepath.Join(b.execRoot, "runsc-state") }

// OutputDir returns the per-execution directory that is bind-mounted to /output inside the container.
func (b *BundleDir) OutputDir() string { return filepath.Join(b.execRoot, "output") }

// Cleanup removes the entire execution root. Errors are logged, not returned.
func (b *BundleDir) Cleanup() {
	if err := os.RemoveAll(b.execRoot); err != nil {
		log.Warn().Err(err).Str("exec_id", b.ExecID).Msg("failed to remove exec root")
	}
}
