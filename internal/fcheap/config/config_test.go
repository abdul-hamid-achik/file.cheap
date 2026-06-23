package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Compression != DefaultCompression {
		t.Errorf("Compression = %s, want %s", cfg.Compression, DefaultCompression)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.CompressThreshold != DefaultCompressThreshold {
		t.Errorf("CompressThreshold = %d, want %d", cfg.CompressThreshold, DefaultCompressThreshold)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	cfg := &Config{
		StashDir:          "/tmp/stash",
		Compression:       "gzip",
		CompressThreshold: 5 * 1024 * 1024,
		LogLevel:          "debug",
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "fcheap", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Compression != cfg.Compression {
		t.Errorf("Compression = %s, want %s", loaded.Compression, cfg.Compression)
	}
	if loaded.CompressThreshold != cfg.CompressThreshold {
		t.Errorf("CompressThreshold = %d, want %d", loaded.CompressThreshold, cfg.CompressThreshold)
	}
	if loaded.LogLevel != cfg.LogLevel {
		t.Errorf("LogLevel = %s, want %s", loaded.LogLevel, cfg.LogLevel)
	}
}

func TestEnvOverrides(t *testing.T) {
	_ = os.Setenv(EnvStashDir, "/custom/stash")
	_ = os.Setenv(EnvLogLevel, "debug")
	defer func() {
		_ = os.Unsetenv(EnvStashDir)
		_ = os.Unsetenv(EnvLogLevel)
	}()

	cfg := &Config{LogLevel: DefaultLogLevel}
	applyEnvOverrides(cfg)

	if cfg.StashDir != "/custom/stash" {
		t.Errorf("StashDir = %s, want /custom/stash", cfg.StashDir)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
	}
}

// TestLoadFromDiskIgnoresEnvOverrides verifies that LoadFromDisk (used by
// `config set`/`config init`) does NOT apply env overrides, so transient
// FCHEAP_* vars are never persisted into config.yaml — while Load (runtime)
// still applies them. Regression test for "env overrides baked into config.yaml".
func TestLoadFromDiskIgnoresEnvOverrides(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv(EnvStashDir, "/tmp/env-override-stashdir")

	disk, err := LoadFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if disk.StashDir == "/tmp/env-override-stashdir" {
		t.Errorf("LoadFromDisk leaked the env override: %q", disk.StashDir)
	}

	rt, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if rt.StashDir != "/tmp/env-override-stashdir" {
		t.Errorf("Load() should apply the env override, got %q", rt.StashDir)
	}

	// Simulate `config set compression gzip` against the disk config, then ensure
	// the persisted file did not capture the env override.
	disk.Compression = "gzip"
	if err := disk.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.StashDir == "/tmp/env-override-stashdir" {
		t.Errorf("saved config leaked the env override onto disk: %q", reloaded.StashDir)
	}
	if reloaded.Compression != "gzip" {
		t.Errorf("expected persisted compression=gzip, got %q", reloaded.Compression)
	}
}
