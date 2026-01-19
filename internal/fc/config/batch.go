package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type BatchConfig struct {
	Defaults BatchDefaults     `yaml:"defaults,omitempty"`
	Files    []BatchFileConfig `yaml:"files,omitempty"`
}

type BatchDefaults struct {
	Transforms []string `yaml:"transforms,omitempty"`
	Quality    int      `yaml:"quality,omitempty"`
	Position   string   `yaml:"position,omitempty"`
}

type BatchFileConfig struct {
	Pattern     string   `yaml:"pattern,omitempty"`
	Path        string   `yaml:"path,omitempty"`
	Transforms  []string `yaml:"transforms,omitempty"`
	Quality     int      `yaml:"quality,omitempty"`
	Position    string   `yaml:"position,omitempty"`
	ThumbnailAt string   `yaml:"thumbnail_at,omitempty"`
	Watermark   string   `yaml:"watermark,omitempty"`
}

func LoadBatchConfig(path string) (*BatchConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &BatchConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (bc *BatchConfig) GetFileConfig(filename string) *BatchFileConfig {
	baseName := filepath.Base(filename)

	for _, fc := range bc.Files {
		if fc.Path != "" && (fc.Path == filename || fc.Path == baseName) {
			return &fc
		}

		if fc.Pattern != "" {
			matched, err := filepath.Match(fc.Pattern, baseName)
			if err == nil && matched {
				return &fc
			}
		}
	}

	return nil
}

func (bc *BatchConfig) GetTransforms(filename string) []string {
	fc := bc.GetFileConfig(filename)
	if fc != nil && len(fc.Transforms) > 0 {
		return fc.Transforms
	}
	return bc.Defaults.Transforms
}

func (bc *BatchConfig) GetQuality(filename string) int {
	fc := bc.GetFileConfig(filename)
	if fc != nil && fc.Quality > 0 {
		return fc.Quality
	}
	return bc.Defaults.Quality
}

func (bc *BatchConfig) GetPosition(filename string) string {
	fc := bc.GetFileConfig(filename)
	if fc != nil && fc.Position != "" {
		return fc.Position
	}
	return bc.Defaults.Position
}

func (bc *BatchConfig) GetThumbnailAt(filename string) string {
	fc := bc.GetFileConfig(filename)
	if fc != nil {
		return fc.ThumbnailAt
	}
	return ""
}

func (bc *BatchConfig) GetWatermark(filename string) string {
	fc := bc.GetFileConfig(filename)
	if fc != nil && fc.Watermark != "" {
		return fc.Watermark
	}
	return ""
}

func (bc *BatchConfig) BuildTransformsForFile(filename string) []string {
	transforms := bc.GetTransforms(filename)
	result := make([]string, 0, len(transforms)+3)
	result = append(result, transforms...)

	if pos := bc.GetPosition(filename); pos != "" {
		result = append(result, "position:"+pos)
	}

	if thumbAt := bc.GetThumbnailAt(filename); thumbAt != "" {
		result = append(result, "thumbnail_at:"+thumbAt)
	}

	return result
}

func IsVideoFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4", ".webm", ".mov", ".avi", ".mkv", ".m4v", ".wmv", ".flv":
		return true
	}
	return false
}
