package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg, err := defaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunscPath != "/usr/local/bin/runsc" {
		t.Errorf("unexpected runsc_path: %s", cfg.RunscPath)
	}
	if cfg.Platform != "systrap" {
		t.Errorf("unexpected platform: %s", cfg.Platform)
	}
	if cfg.OutputLimitBytes != 10*1024*1024 {
		t.Errorf("unexpected output_limit_bytes: %d", cfg.OutputLimitBytes)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("unexpected listen_addr: %s", cfg.ListenAddr)
	}
	if cfg.Defaults.CPUCores == nil || *cfg.Defaults.CPUCores != 1.0 {
		t.Error("expected default cpu_cores=1.0")
	}
}

func TestDerivedPaths(t *testing.T) {
	cfg := BoxerConfig{Home: "/custom/boxer"}
	if cfg.StateRoot() != "/custom/boxer/run" {
		t.Errorf("unexpected StateRoot: %s", cfg.StateRoot())
	}
	if cfg.ImageStore() != "/custom/boxer/images" {
		t.Errorf("unexpected ImageStore: %s", cfg.ImageStore())
	}
	if cfg.ConfigFile() != "/custom/boxer/config.json" {
		t.Errorf("unexpected ConfigFile: %s", cfg.ConfigFile())
	}
}

func TestDefaultHome_UnderUserHome(t *testing.T) {
	cfg, err := defaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(cfg.Home, home) {
		t.Errorf("expected Home to be under %s, got %s", home, cfg.Home)
	}
	if filepath.Base(cfg.Home) != ".boxer" {
		t.Errorf("expected Home to end in .boxer, got %s", cfg.Home)
	}
}

func TestResolveLimits_NoOverride(t *testing.T) {
	cfg, _ := defaultConfig()
	limits := cfg.ResolveLimits(nil)
	if limits.CPUCores == nil || *limits.CPUCores != 1.0 {
		t.Error("expected cpu_cores from defaults")
	}
	if limits.MemoryMB == nil || *limits.MemoryMB != 256 {
		t.Error("expected memory_mb=256 from defaults")
	}
}

func TestResolveLimits_PartialOverride(t *testing.T) {
	cfg, _ := defaultConfig()
	mem := int64(512)
	limits := cfg.ResolveLimits(&ResourceLimits{MemoryMB: &mem})
	if *limits.MemoryMB != 512 {
		t.Errorf("expected memory_mb=512, got %d", *limits.MemoryMB)
	}
	if limits.CPUCores == nil || *limits.CPUCores != 1.0 {
		t.Error("expected cpu_cores from defaults when not overridden")
	}
	if limits.WallClockSecs == nil || *limits.WallClockSecs != 30 {
		t.Error("expected wall_clock_secs=30 from defaults")
	}
}

func TestResolveLimits_FullOverride(t *testing.T) {
	cfg, _ := defaultConfig()
	cpu := 0.5
	mem := int64(128)
	pids := int64(16)
	wall := int64(10)
	nofile := uint64(64)
	limits := cfg.ResolveLimits(&ResourceLimits{
		CPUCores:      &cpu,
		MemoryMB:      &mem,
		PidsLimit:     &pids,
		WallClockSecs: &wall,
		NoFile:        &nofile,
	})
	if *limits.CPUCores != 0.5 {
		t.Errorf("expected cpu_cores=0.5, got %f", *limits.CPUCores)
	}
	if *limits.MemoryMB != 128 {
		t.Errorf("expected memory_mb=128, got %d", *limits.MemoryMB)
	}
	if *limits.PidsLimit != 16 {
		t.Errorf("expected pids_limit=16, got %d", *limits.PidsLimit)
	}
	if *limits.WallClockSecs != 10 {
		t.Errorf("expected wall_clock_secs=10, got %d", *limits.WallClockSecs)
	}
	if *limits.NoFile != 64 {
		t.Errorf("expected nofile=64, got %d", *limits.NoFile)
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")
	data := `{
		"home": "/custom/boxer",
		"runsc_path": "/custom/runsc",
		"platform": "kvm",
		"listen_addr": ":9090"
	}`
	if err := os.WriteFile(cfgFile, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOXER_CONFIG", cfgFile)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Home != "/custom/boxer" {
		t.Errorf("unexpected home: %s", cfg.Home)
	}
	if cfg.RunscPath != "/custom/runsc" {
		t.Errorf("unexpected runsc_path: %s", cfg.RunscPath)
	}
	if cfg.Platform != "kvm" {
		t.Errorf("unexpected platform: %s", cfg.Platform)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("unexpected listen_addr: %s", cfg.ListenAddr)
	}
	// Derived paths use the overridden Home.
	if cfg.StateRoot() != "/custom/boxer/run" {
		t.Errorf("unexpected StateRoot: %s", cfg.StateRoot())
	}
	// unspecified field should retain default
	if cfg.OutputLimitBytes != 10*1024*1024 {
		t.Errorf("unspecified field should use default, got %d", cfg.OutputLimitBytes)
	}
}

func TestLoad_DefaultsWhenNoFile(t *testing.T) {
	t.Setenv("BOXER_CONFIG", "/nonexistent/path/config.json")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunscPath != "/usr/local/bin/runsc" {
		t.Errorf("expected default runsc_path, got %s", cfg.RunscPath)
	}
}

func TestConfigRoundtrip(t *testing.T) {
	cfg, _ := defaultConfig()
	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back BoxerConfig
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.RunscPath != cfg.RunscPath {
		t.Errorf("roundtrip mismatch: %s != %s", back.RunscPath, cfg.RunscPath)
	}
	if back.Home != cfg.Home {
		t.Errorf("roundtrip home mismatch: %s != %s", back.Home, cfg.Home)
	}
}
