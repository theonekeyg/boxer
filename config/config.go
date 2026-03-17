package config

import (
	"encoding/json"
	"flag"
	"os"
)

// BoxerConfig is the top-level service configuration.
type BoxerConfig struct {
	RunscPath        string         `json:"runsc_path"`
	Platform         string         `json:"platform"`           // systrap|ptrace|kvm
	StateRoot        string         `json:"state_root"`         // /tmp/boxer
	ImageStore       string         `json:"image_store"`        // /var/lib/boxer/images
	OutputLimitBytes int            `json:"output_limit_bytes"` // bytes, per stream
	ListenAddr       string         `json:"listen_addr"`        // :8080
	Defaults         ResourceLimits `json:"defaults"`
}

// ResourceLimits holds per-execution resource constraints. All fields are
// pointers so callers can distinguish "not set" from zero.
type ResourceLimits struct {
	CPUCores      *float64 `json:"cpu_cores"`
	MemoryMB      *int64   `json:"memory_mb"`
	PidsLimit     *int64   `json:"pids_limit"`
	WallClockSecs *int64   `json:"wall_clock_secs"`
	NoFile        *uint64  `json:"nofile"`
}

func defaultConfig() BoxerConfig {
	cpu := 1.0
	mem := int64(256)
	pids := int64(64)
	wall := int64(30)
	nofile := uint64(256)
	return BoxerConfig{
		RunscPath:        "/usr/local/bin/runsc",
		Platform:         "systrap",
		StateRoot:        "/tmp/boxer",
		ImageStore:       "/var/lib/boxer/images",
		OutputLimitBytes: 10 * 1024 * 1024,
		ListenAddr:       ":8080",
		Defaults: ResourceLimits{
			CPUCores:      &cpu,
			MemoryMB:      &mem,
			PidsLimit:     &pids,
			WallClockSecs: &wall,
			NoFile:        &nofile,
		},
	}
}

// Load reads the config from (in order of precedence):
//  1. --config flag value
//  2. $BOXER_CONFIG env var
//  3. /etc/boxer/config.json
//  4. built-in defaults
func Load() (*BoxerConfig, error) {
	cfg := defaultConfig()

	path := ""
	// Check --config flag if it was registered.
	if f := flag.Lookup("config"); f != nil {
		path = f.Value.String()
	}
	if path == "" {
		path = os.Getenv("BOXER_CONFIG")
	}
	if path == "" {
		path = "/etc/boxer/config.json"
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
// Fields set in overrides take precedence; unset fields fall back to defaults.
func (c *BoxerConfig) ResolveLimits(overrides *ResourceLimits) ResourceLimits {
	result := c.Defaults
	if overrides == nil {
		return result
	}
	if overrides.CPUCores != nil {
		result.CPUCores = overrides.CPUCores
	}
	if overrides.MemoryMB != nil {
		result.MemoryMB = overrides.MemoryMB
	}
	if overrides.PidsLimit != nil {
		result.PidsLimit = overrides.PidsLimit
	}
	if overrides.WallClockSecs != nil {
		result.WallClockSecs = overrides.WallClockSecs
	}
	if overrides.NoFile != nil {
		result.NoFile = overrides.NoFile
	}
	return result
}
