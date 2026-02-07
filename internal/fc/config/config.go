package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Quality   int               `yaml:"quality,omitempty"`
	OutputDir string            `yaml:"output_dir,omitempty"`
	Parallel  int               `yaml:"parallel,omitempty"`
	Overwrite bool              `yaml:"overwrite,omitempty"`
	TempDir   string            `yaml:"temp_dir,omitempty"`
	LogLevel  string            `yaml:"log_level,omitempty"`
	Presets   map[string]Preset `yaml:"presets,omitempty"`
}

type Preset struct {
	Transforms []string `yaml:"transforms"`
	Parallel   int      `yaml:"parallel,omitempty"`
	Quality    int      `yaml:"quality,omitempty"`
	Watermark  string   `yaml:"watermark,omitempty"`
}

const (
	DefaultQuality  = 85
	DefaultParallel = 0 // 0 means runtime.NumCPU()
	DefaultLogLevel = "warn"

	EnvQuality   = "FC_QUALITY"
	EnvOutputDir = "FC_OUTPUT_DIR"
	EnvJobs      = "FC_JOBS"
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
		Quality:  DefaultQuality,
		Parallel: DefaultParallel,
		LogLevel: DefaultLogLevel,
		Presets:  make(map[string]Preset),
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

	if cfg.Quality <= 0 {
		cfg.Quality = DefaultQuality
	}
	if cfg.Presets == nil {
		cfg.Presets = make(map[string]Preset)
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv(EnvQuality); v != "" {
		if q, err := strconv.Atoi(v); err == nil && q > 0 && q <= 100 {
			cfg.Quality = q
		}
	}
	if v := os.Getenv(EnvOutputDir); v != "" {
		cfg.OutputDir = v
	}
	if v := os.Getenv(EnvJobs); v != "" {
		if j, err := strconv.Atoi(v); err == nil && j > 0 {
			cfg.Parallel = j
		}
	}
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

// EffectiveParallel returns the number of parallel workers to use.
// Returns runtime.NumCPU() if Parallel is 0 or negative.
func (c *Config) EffectiveParallel() int {
	if c.Parallel <= 0 {
		return runtime.NumCPU()
	}
	return c.Parallel
}
