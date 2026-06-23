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
