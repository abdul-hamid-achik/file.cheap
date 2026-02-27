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

	if cfg.Quality != DefaultQuality {
		t.Errorf("Quality = %d, want %d", cfg.Quality, DefaultQuality)
	}
	if cfg.Parallel != DefaultParallel {
		t.Errorf("Parallel = %d, want %d", cfg.Parallel, DefaultParallel)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, DefaultLogLevel)
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
		Quality:   90,
		OutputDir: "/tmp/output",
		Parallel:  8,
		Overwrite: true,
		LogLevel:  "debug",
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

	if loaded.Quality != cfg.Quality {
		t.Errorf("Quality = %d, want %d", loaded.Quality, cfg.Quality)
	}
	if loaded.OutputDir != cfg.OutputDir {
		t.Errorf("OutputDir = %s, want %s", loaded.OutputDir, cfg.OutputDir)
	}
	if loaded.Parallel != cfg.Parallel {
		t.Errorf("Parallel = %d, want %d", loaded.Parallel, cfg.Parallel)
	}
	if loaded.Overwrite != cfg.Overwrite {
		t.Errorf("Overwrite = %v, want %v", loaded.Overwrite, cfg.Overwrite)
	}
}

func TestGetPreset(t *testing.T) {
	cfg := &Config{
		Presets: map[string]Preset{
			"custom": {
				Transforms: []string{"webp"},
				Quality:    80,
			},
		},
	}

	preset, ok := cfg.GetPreset("ecommerce")
	if !ok {
		t.Error("GetPreset(ecommerce) should return builtin preset")
	}
	if len(preset.Transforms) == 0 {
		t.Error("ecommerce preset should have transforms")
	}

	preset, ok = cfg.GetPreset("custom")
	if !ok {
		t.Error("GetPreset(custom) should return custom preset")
	}
	if preset.Quality != 80 {
		t.Errorf("custom preset Quality = %d, want 80", preset.Quality)
	}

	_, ok = cfg.GetPreset("nonexistent")
	if ok {
		t.Error("GetPreset(nonexistent) should return false")
	}
}

func TestBuiltinPresets(t *testing.T) {
	expectedPresets := []string{"ecommerce", "social", "blog", "avatar", "responsive"}
	for _, name := range expectedPresets {
		if _, ok := BuiltinPresets[name]; !ok {
			t.Errorf("BuiltinPresets should contain %s", name)
		}
	}
}

func TestEnvOverrides(t *testing.T) {
	_ = os.Setenv(EnvQuality, "75")
	_ = os.Setenv(EnvOutputDir, "/custom/output")
	_ = os.Setenv(EnvJobs, "16")
	defer func() {
		_ = os.Unsetenv(EnvQuality)
		_ = os.Unsetenv(EnvOutputDir)
		_ = os.Unsetenv(EnvJobs)
	}()

	cfg := &Config{Quality: DefaultQuality}
	applyEnvOverrides(cfg)

	if cfg.Quality != 75 {
		t.Errorf("Quality = %d, want 75", cfg.Quality)
	}
	if cfg.OutputDir != "/custom/output" {
		t.Errorf("OutputDir = %s, want /custom/output", cfg.OutputDir)
	}
	if cfg.Parallel != 16 {
		t.Errorf("Parallel = %d, want 16", cfg.Parallel)
	}
}
