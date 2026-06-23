package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds fcheap stash configuration.
type Config struct {
	// StashDir is the root directory for stash storage.
	// Defaults to ~/.local/share/fcheap (XDG_DATA_HOME or ~/.local/share).
	StashDir string `yaml:"stash_dir,omitempty"`

	// Compression algorithm: "zstd" (default), "gzip", or "none".
	Compression string `yaml:"compression,omitempty"`

	// CompressThreshold is the size in bytes above which a stash is auto-compressed.
	// Default 10MB.
	CompressThreshold int64 `yaml:"compress_threshold,omitempty"`

	// LogLevel: "debug", "info", "warn" (default), "error".
	LogLevel string `yaml:"log_level,omitempty"`

	// VecgrepPath is the path to the vecgrep binary for external analysis.
	// If empty, searches PATH.
	VecgrepPath string `yaml:"vecgrep_path,omitempty"`

	// Embedder config for semantic search via veclite.
	// If empty, only BM25 keyword search is available.
	Embedder   string `yaml:"embedder,omitempty"`
	EmbedModel string `yaml:"embed_model,omitempty"`
	OllamaURL  string `yaml:"ollama_url,omitempty"`
}

const (
	DefaultCompression       = "zstd"
	DefaultCompressThreshold = 10 * 1024 * 1024 // 10MB
	DefaultLogLevel          = "warn"

	EnvStashDir    = "FCHEAP_STASH_DIR"
	EnvLogLevel    = "FCHEAP_LOG_LEVEL"
	EnvVecgrepPath = "FCHEAP_VECGREP_PATH"
)

// Dir returns the config directory path (~/.config/fcheap).
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "fcheap"), nil
}

// Path returns the config file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DefaultStashDir returns the default XDG data directory for fcheap.
func DefaultStashDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "fcheap"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "fcheap"), nil
}

// Load reads the config from file and applies env overrides.
func Load() (*Config, error) {
	stashDir, err := DefaultStashDir()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		StashDir:          stashDir,
		Compression:       DefaultCompression,
		CompressThreshold: DefaultCompressThreshold,
		LogLevel:          DefaultLogLevel,
	}

	path, err := Path()
	if err != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.StashDir == "" {
		cfg.StashDir = stashDir
	}
	if cfg.Compression == "" {
		cfg.Compression = DefaultCompression
	}
	if cfg.CompressThreshold <= 0 {
		cfg.CompressThreshold = DefaultCompressThreshold
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv(EnvStashDir); v != "" {
		cfg.StashDir = v
	}
	if v := os.Getenv(EnvLogLevel); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv(EnvVecgrepPath); v != "" {
		cfg.VecgrepPath = v
	}
}

// Save writes the config to disk.
func (c *Config) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
