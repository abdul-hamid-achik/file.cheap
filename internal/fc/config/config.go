package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey            string            `yaml:"api_key,omitempty"`
	BaseURL           string            `yaml:"base_url,omitempty"`
	DefaultTransforms []string          `yaml:"default_transforms,omitempty"`
	Parallel          int               `yaml:"parallel,omitempty"`
	Presets           map[string]Preset `yaml:"presets,omitempty"`
	Timeouts          TimeoutConfig     `yaml:"timeouts,omitempty"`
}

// TimeoutConfig holds configurable timeout durations for various operations.
// All durations are specified as strings parseable by time.ParseDuration (e.g., "5m", "30s", "1h").
type TimeoutConfig struct {
	HTTP        string `yaml:"http,omitempty"`         // HTTP client timeout (default: 5m)
	Auth        string `yaml:"auth,omitempty"`         // Device auth timeout (default: 15m)
	Upload      string `yaml:"upload,omitempty"`       // Upload wait timeout (default: 5m)
	BatchWait   string `yaml:"batch_wait,omitempty"`   // Batch completion wait (default: 30m)
	StatusWatch string `yaml:"status_watch,omitempty"` // File status watch (default: 10m)
}

type Preset struct {
	Transforms []string `yaml:"transforms"`
	Parallel   int      `yaml:"parallel,omitempty"`
	Quality    int      `yaml:"quality,omitempty"`
	Watermark  string   `yaml:"watermark,omitempty"`
}

const (
	DefaultBaseURL  = "https://file.cheap"
	DefaultParallel = 4

	// Environment variable names for configuration overrides
	EnvAPIKey  = "FC_API_KEY"
	EnvBaseURL = "FC_BASE_URL"

	// Default timeout durations
	DefaultHTTPTimeout        = 5 * time.Minute
	DefaultAuthTimeout        = 15 * time.Minute
	DefaultUploadTimeout      = 5 * time.Minute
	DefaultBatchWaitTimeout   = 30 * time.Minute
	DefaultStatusWatchTimeout = 10 * time.Minute
)

var BuiltinPresets = map[string]Preset{
	"ecommerce": {
		Transforms: []string{"thumbnail", "sm", "md", "lg", "webp"},
		Quality:    85,
	},
	"social": {
		Transforms: []string{"og", "twitter", "instagram_square", "instagram_portrait", "instagram_story"},
		Quality:    90,
	},
	"blog": {
		Transforms: []string{"md", "webp", "thumbnail"},
		Quality:    85,
	},
	"avatar": {
		Transforms: []string{"thumbnail", "sm"},
		Quality:    90,
	},
	"responsive": {
		Transforms: []string{"sm", "md", "lg", "xl", "webp"},
		Quality:    85,
	},
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "fc"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func Load() (*Config, error) {
	cfg := &Config{
		BaseURL:  DefaultBaseURL,
		Parallel: DefaultParallel,
		Presets:  make(map[string]Preset),
	}

	path, err := Path()
	if err != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Parallel == 0 {
		cfg.Parallel = DefaultParallel
	}

	// Environment variables take precedence over config file
	if envKey := os.Getenv(EnvAPIKey); envKey != "" {
		cfg.APIKey = envKey
	}
	if envURL := os.Getenv(EnvBaseURL); envURL != "" {
		cfg.BaseURL = envURL
	}

	return cfg, nil
}

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

func (c *Config) GetPreset(name string) (Preset, bool) {
	if preset, ok := c.Presets[name]; ok {
		return preset, true
	}
	if preset, ok := BuiltinPresets[name]; ok {
		return preset, true
	}
	return Preset{}, false
}

func (c *Config) IsAuthenticated() bool {
	return c.APIKey != ""
}

func (c *Config) ClearAuth() error {
	c.APIKey = ""
	return c.Save()
}

func (c *Config) SetAPIKey(key string) error {
	c.APIKey = key
	return c.Save()
}

// GetTimeout returns the configured timeout for the given operation, or the default if not set.
// Valid names: "http", "auth", "upload", "batch_wait", "status_watch"
func (c *Config) GetTimeout(name string) time.Duration {
	var configValue string
	var defaultValue time.Duration

	switch name {
	case "http":
		configValue = c.Timeouts.HTTP
		defaultValue = DefaultHTTPTimeout
	case "auth":
		configValue = c.Timeouts.Auth
		defaultValue = DefaultAuthTimeout
	case "upload":
		configValue = c.Timeouts.Upload
		defaultValue = DefaultUploadTimeout
	case "batch_wait":
		configValue = c.Timeouts.BatchWait
		defaultValue = DefaultBatchWaitTimeout
	case "status_watch":
		configValue = c.Timeouts.StatusWatch
		defaultValue = DefaultStatusWatchTimeout
	default:
		return 5 * time.Minute // fallback default
	}

	if configValue == "" {
		return defaultValue
	}

	parsed, err := time.ParseDuration(configValue)
	if err != nil {
		return defaultValue
	}
	return parsed
}
