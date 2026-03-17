package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// BoxerConfig is the top-level service configuration.
// All data directories are derived from Home (~/.boxer by default).
type BoxerConfig struct {
	// Home is the base directory for all boxer data.
	// Defaults to ~/.boxer. All other paths are derived from it.
	Home string `json:"home"`

	RunscPath        string         `json:"runsc_path"`
	Platform         string         `json:"platform"`           // systrap|ptrace|kvm
	OutputLimitBytes int            `json:"output_limit_bytes"` // bytes, per stream
	ListenAddr       string         `json:"listen_addr"`        // :8080
	Defaults         ResourceLimits `json:"defaults"`
}

// StateRoot is where per-execution temp directories are created.
func (c *BoxerConfig) StateRoot() string { return filepath.Join(c.Home, "run") }

// ImageStore is where unpacked image rootfs trees are cached.
func (c *BoxerConfig) ImageStore() string { return filepath.Join(c.Home, "images") }

// ConfigFile returns the path of the config file inside Home.
func (c *BoxerConfig) ConfigFile() string { return filepath.Join(c.Home, "config.json") }

// ResourceLimits holds per-execution resource constraints. All fields are
// pointers so callers can distinguish "not set" from zero.
type ResourceLimits struct {
	CPUCores      *float64 `json:"cpu_cores"`
	MemoryMB      *int64   `json:"memory_mb"`
	PidsLimit     *int64   `json:"pids_limit"`
	WallClockSecs *int64   `json:"wall_clock_secs"`
	NoFile        *uint64  `json:"nofile"`
}

// boxerHome returns the default base directory: $HOME/.boxer.
func boxerHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".boxer"), nil
}

func defaultConfig() (BoxerConfig, error) {
	home, err := boxerHome()
	if err != nil {
		return BoxerConfig{}, err
	}
	cpu := 1.0
	mem := int64(256)
	pids := int64(64)
	wall := int64(30)
	nofile := uint64(256)
	return BoxerConfig{
		Home:             home,
		RunscPath:        "/usr/local/bin/runsc",
		Platform:         "systrap",
		OutputLimitBytes: 10 * 1024 * 1024,
		ListenAddr:       ":8080",
		Defaults: ResourceLimits{
			CPUCores:      &cpu,
			MemoryMB:      &mem,
			PidsLimit:     &pids,
			WallClockSecs: &wall,
			NoFile:        &nofile,
		},
	}, nil
}

// Load reads the config with the following precedence:
//  1. --config flag
//  2. $BOXER_CONFIG env var
//  3. ~/.boxer/config.json
//  4. built-in defaults
func Load() (*BoxerConfig, error) {
	cfg, err := defaultConfig()
	if err != nil {
		return nil, err
	}

	path := ""
	if f := flag.Lookup("config"); f != nil {
		path = f.Value.String()
	}
	if path == "" {
		path = os.Getenv("BOXER_CONFIG")
	}
	if path == "" {
		path = cfg.ConfigFile()
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ResolveLimits merges per-request overrides on top of the configured defaults.
func (c *BoxerConfig) ResolveLimits(overrides *ResourceLimits) ResourceLimits {
	result := c.Defaults
	if overrides == nil {
		return result
	}
	if overrides.CPUCores != nil && *overrides.CPUCores != 0 {
		result.CPUCores = overrides.CPUCores
	}
	if overrides.MemoryMB != nil && *overrides.MemoryMB != 0 {
		result.MemoryMB = overrides.MemoryMB
	}
	if overrides.PidsLimit != nil && *overrides.PidsLimit != 0 {
		result.PidsLimit = overrides.PidsLimit
	}
	if overrides.WallClockSecs != nil && *overrides.WallClockSecs != 0 {
		result.WallClockSecs = overrides.WallClockSecs
	}
	if overrides.NoFile != nil && *overrides.NoFile != 0 {
		result.NoFile = overrides.NoFile
	}
	return result
}
