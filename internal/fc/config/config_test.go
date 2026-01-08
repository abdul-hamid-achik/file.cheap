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

	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %s, want %s", cfg.BaseURL, DefaultBaseURL)
	}
	if cfg.Parallel != DefaultParallel {
		t.Errorf("Parallel = %d, want %d", cfg.Parallel, DefaultParallel)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := &Config{
		APIKey:            "fp_test123",
		BaseURL:           "https://test.file.cheap",
		DefaultTransforms: []string{"webp", "thumbnail"},
		Parallel:          8,
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "fc", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.APIKey != cfg.APIKey {
		t.Errorf("APIKey = %s, want %s", loaded.APIKey, cfg.APIKey)
	}
	if loaded.BaseURL != cfg.BaseURL {
		t.Errorf("BaseURL = %s, want %s", loaded.BaseURL, cfg.BaseURL)
	}
	if loaded.Parallel != cfg.Parallel {
		t.Errorf("Parallel = %d, want %d", loaded.Parallel, cfg.Parallel)
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

func TestIsAuthenticated(t *testing.T) {
	cfg := &Config{}
	if cfg.IsAuthenticated() {
		t.Error("Empty config should not be authenticated")
	}

	cfg.APIKey = "fp_test"
	if !cfg.IsAuthenticated() {
		t.Error("Config with APIKey should be authenticated")
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
