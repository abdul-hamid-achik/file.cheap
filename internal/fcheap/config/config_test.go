package config

import (
	"os"
	"path/filepath"
	"runtime"
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
	if cfg.Parallel != 0 {
		t.Errorf("Parallel = %d, want 0 (default)", cfg.Parallel)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.CompressThreshold != DefaultCompressThreshold {
		t.Errorf("CompressThreshold = %d, want %d", cfg.CompressThreshold, DefaultCompressThreshold)
	}
}

func TestEffectiveParallel(t *testing.T) {
	cfg := &Config{Parallel: 0}
	if got := cfg.EffectiveParallel(); got != runtime.NumCPU() {
		t.Errorf("EffectiveParallel() = %d, want %d", got, runtime.NumCPU())
	}

	cfg.Parallel = 4
	if got := cfg.EffectiveParallel(); got != 4 {
		t.Errorf("EffectiveParallel() = %d, want 4", got)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	cfg := &Config{
		StashDir:           "/tmp/stash",
		Compression:        "gzip",
		CompressThreshold:  5 * 1024 * 1024,
		Parallel:           8,
		LogLevel:           "debug",
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
	if loaded.Parallel != cfg.Parallel {
		t.Errorf("Parallel = %d, want %d", loaded.Parallel, cfg.Parallel)
	}
	if loaded.LogLevel != cfg.LogLevel {
		t.Errorf("LogLevel = %s, want %s", loaded.LogLevel, cfg.LogLevel)
	}
}

func TestEnvOverrides(t *testing.T) {
	_ = os.Setenv(EnvStashDir, "/custom/stash")
	_ = os.Setenv(EnvJobs, "16")
	_ = os.Setenv(EnvLogLevel, "debug")
	defer func() {
		_ = os.Unsetenv(EnvStashDir)
		_ = os.Unsetenv(EnvJobs)
		_ = os.Unsetenv(EnvLogLevel)
	}()

	cfg := &Config{LogLevel: DefaultLogLevel}
	applyEnvOverrides(cfg)

	if cfg.StashDir != "/custom/stash" {
		t.Errorf("StashDir = %s, want /custom/stash", cfg.StashDir)
	}
	if cfg.Parallel != 16 {
		t.Errorf("Parallel = %d, want 16", cfg.Parallel)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
	}
}