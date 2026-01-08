package processor

import (
	"context"
	"errors"
	"io"
)

var (
	ErrUnsupportedType  = errors.New("processor: unsupported file type")
	ErrProcessingFailed = errors.New("processor: processing failed")
	ErrInvalidConfig    = errors.New("processor: invalid configuration")
	ErrFileTooLarge     = errors.New("processor: file too large")
	ErrCorruptedFile    = errors.New("processor: file appears corrupted")
)

type Processor interface {
	Process(ctx context.Context, opts *Options, input io.Reader) (*Result, error)
	SupportedTypes() []string
	Name() string
}

type Options struct {
	Width       int
	Height      int
	Quality     int
	Fit         string
	Format      string
	VariantType string
	Page        int
}

type Result struct {
	Data        io.Reader
	ContentType string
	Filename    string
	Size        int64
	Metadata    ResultMetadata
}

type ResultMetadata struct {
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
	Format     string  `json:"format,omitempty"`
	Quality    int     `json:"quality,omitempty"`
	Compressed bool    `json:"compressed,omitempty"`
}

type Config struct {
	MaxFileSize  int64
	TempDir      string
	Quality      int
	MaxDimension int
}

func DefaultConfig() *Config {
	return &Config{
		MaxFileSize:  100 * 1024 * 1024,
		TempDir:      "/tmp/processor",
		Quality:      85,
		MaxDimension: 4096,
	}
}
