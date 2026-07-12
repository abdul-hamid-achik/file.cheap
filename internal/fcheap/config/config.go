package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds fcheap stash configuration. JSON tags mirror the YAML keys so
// `config show --json` emits snake_case identical to the on-disk file.
type Config struct {
	// StashDir is the root directory for stash storage.
	// Defaults to ~/.local/share/fcheap (XDG_DATA_HOME or ~/.local/share).
	StashDir string `yaml:"stash_dir,omitempty" json:"stash_dir,omitempty"`

	// Compression algorithm: "zstd" (default), "gzip", or "none".
	Compression string `yaml:"compression,omitempty" json:"compression,omitempty"`

	// CompressThreshold is the size in bytes above which a stash is auto-compressed.
	// Default 10MB.
	CompressThreshold int64 `yaml:"compress_threshold,omitempty" json:"compress_threshold,omitempty"`

	// LogLevel: "debug", "info", "warn" (default), "error".
	LogLevel string `yaml:"log_level,omitempty" json:"log_level,omitempty"`

	// VecgrepPath is the path to the vecgrep binary for external analysis.
	// If empty, searches PATH.
	VecgrepPath string `yaml:"vecgrep_path,omitempty" json:"vecgrep_path,omitempty"`

	// Embedder config for semantic search via veclite.
	// If empty, only BM25 keyword search is available.
	Embedder   string `yaml:"embedder,omitempty" json:"embedder,omitempty"`
	EmbedModel string `yaml:"embed_model,omitempty" json:"embed_model,omitempty"`
	OllamaURL  string `yaml:"ollama_url,omitempty" json:"ollama_url,omitempty"`
	// AllowRemoteSecrets permits an explicitly configured remote embedder to
	// receive content from stashes flagged by the save-time secret scanner.
	// It is false by default so local-first behavior remains the safe default.
	AllowRemoteSecrets bool `yaml:"allow_remote_secrets,omitempty" json:"allow_remote_secrets,omitempty"`

	// DefaultTTL is the default time-to-live applied to stashes when the
	// saving tool doesn't have a specific TTL rule. Examples: "14d", "7d",
	// "never" (or empty = no default). Applied by `fcheap save` when
	// --ttl is not explicitly set and no per-tool rule matches.
	DefaultTTL string `yaml:"default_ttl,omitempty" json:"default_ttl,omitempty"`

	// TTLRules maps tool names to TTL strings (e.g. "vidtrace": "30d").
	// The value "never" or "" means no TTL for that tool. Checked first
	// before falling back to DefaultTTL.
	TTLRules map[string]string `yaml:"ttl_rules,omitempty" json:"ttl_rules,omitempty"`
}

const (
	DefaultCompression       = "zstd"
	DefaultCompressThreshold = 10 * 1024 * 1024 // 10MB
	DefaultLogLevel          = "warn"

	// DefaultDefaultTTL is the default value for the default_ttl config key.
	// Empty means "no default TTL" — stashes are permanent unless a per-tool
	// rule or --ttl says otherwise.
	DefaultDefaultTTL = ""

	EnvStashDir    = "FCHEAP_STASH_DIR"
	EnvLogLevel    = "FCHEAP_LOG_LEVEL"
	EnvVecgrepPath = "FCHEAP_VECGREP_PATH"
)

// Dir returns the XDG config directory for fcheap.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", fmt.Errorf("XDG_CONFIG_HOME must be an absolute path: %q", xdg)
		}
		return filepath.Join(filepath.Clean(xdg), "fcheap"), nil
	}
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

// LoadFromDisk reads the config from file (filling in defaults) WITHOUT applying
// environment overrides. Use this for `config set`/`config init` so transient
// env vars (FCHEAP_*) are never persisted back into config.yaml.
func LoadFromDisk() (*Config, error) {
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
		return normalizePaths(cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return normalizePaths(cfg)
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

	return normalizePaths(cfg)
}

// Load reads the config from disk and applies env overrides. This is the runtime
// view of config; do NOT Save() the result, or env overrides leak onto disk.
func Load() (*Config, error) {
	cfg, err := LoadFromDisk()
	if err != nil {
		return nil, err
	}
	applyEnvOverrides(cfg)
	return normalizePaths(cfg)
}

// normalizePaths expands a leading ~ and resolves relative configured paths
// against the config directory, never the caller's current working directory.
// This makes a config file behave identically from CLI, MCP, and Studio.
func normalizePaths(cfg *Config) (*Config, error) {
	var err error
	cfg.StashDir, err = normalizePath(cfg.StashDir)
	if err != nil {
		return nil, fmt.Errorf("stash_dir: %w", err)
	}
	if cfg.VecgrepPath != "" {
		cfg.VecgrepPath, err = normalizePath(cfg.VecgrepPath)
		if err != nil {
			return nil, fmt.Errorf("vecgrep_path: %w", err)
		}
	}
	return cfg, nil
}

func normalizePath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	} else if strings.HasPrefix(value, "~") {
		return "", fmt.Errorf("unsupported home-directory form %q; use ~/path", value)
	}
	if !filepath.IsAbs(value) {
		dir, err := Dir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(dir, value)
	}
	return filepath.Clean(value), nil
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

// TTLForTool resolves the TTL that should be applied to a stash saved by the
// given tool. It checks TTLRules first, then falls back to DefaultTTL, and
// finally returns "" (no TTL / permanent). The sentinel "never" is
// normalized to "" so callers can treat both as "no expiry".
func (c *Config) TTLForTool(tool string) string {
	if tool != "" {
		if v, ok := c.TTLRules[tool]; ok {
			if v == "never" {
				return ""
			}
			return v
		}
	}
	v := c.DefaultTTL
	if v == "never" {
		return ""
	}
	return v
}
