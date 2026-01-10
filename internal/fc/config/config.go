package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey            string            `yaml:"api_key,omitempty"`
	BaseURL           string            `yaml:"base_url,omitempty"`
	DefaultTransforms []string          `yaml:"default_transforms,omitempty"`
	Parallel          int               `yaml:"parallel,omitempty"`
	Presets           map[string]Preset `yaml:"presets,omitempty"`
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
