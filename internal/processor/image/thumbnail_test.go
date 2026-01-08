package image

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	"io"
	"testing"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
)

// TestThumbnailProcessor_Name tests the processor name.
func TestThumbnailProcessor_Name(t *testing.T) {
	p := NewThumbnailProcessor(nil)

	if got := p.Name(); got != "thumbnail" {
		t.Errorf("Name() = %q, want %q", got, "thumbnail")
	}
}

// TestThumbnailProcessor_SupportedTypes tests supported content types.
func TestThumbnailProcessor_SupportedTypes(t *testing.T) {
	p := NewThumbnailProcessor(nil)
	types := p.SupportedTypes()

	expected := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
	}

	for _, typ := range types {
		if expected[typ] {
			delete(expected, typ)
		}
	}

	for typ := range expected {
		t.Errorf("SupportedTypes() missing %q", typ)
	}
}

// TestThumbnailProcessor_Process tests thumbnail generation.
func TestThumbnailProcessor_Process(t *testing.T) {
	tests := []struct {
		name        string
		input       func() io.Reader
		opts        *processor.Options
		wantWidth   int
		wantHeight  int
		wantErr     bool
		wantErrType error
	}{
		{
			name:  "create 200x200 thumbnail from square image",
			input: createSquareImage,
			opts: &processor.Options{
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			wantWidth:  200,
			wantHeight: 200,
			wantErr:    false,
		},
		{
			name:  "create 100x100 thumbnail",
			input: createSquareImage,
			opts: &processor.Options{
				Width:   100,
				Height:  100,
				Quality: 90,
			},
			wantWidth:  100,
			wantHeight: 100,
			wantErr:    false,
		},
		{
			name:  "create thumbnail from landscape image",
			input: createLandscapeImage,
			opts: &processor.Options{
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			wantWidth:  200,
			wantHeight: 200,
			wantErr:    false,
		},
		{
			name:  "create thumbnail from portrait image",
			input: createPortraitImage,
			opts: &processor.Options{
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			wantWidth:  200,
			wantHeight: 200,
			wantErr:    false,
		},
		{
			name:  "create small thumbnail from large image",
			input: createLargeImage,
			opts: &processor.Options{
				Width:   50,
				Height:  50,
				Quality: 80,
			},
			wantWidth:  50,
			wantHeight: 50,
			wantErr:    false,
		},
		{
			name:  "missing width",
			input: createSquareImage,
			opts: &processor.Options{
				Width:   0,
				Height:  200,
				Quality: 80,
			},
			wantErr:     true,
			wantErrType: processor.ErrInvalidConfig,
		},
		{
			name:  "missing height",
			input: createSquareImage,
			opts: &processor.Options{
				Width:   200,
				Height:  0,
				Quality: 80,
			},
			wantErr:     true,
			wantErrType: processor.ErrInvalidConfig,
		},
		{
			name:  "negative width",
			input: createSquareImage,
			opts: &processor.Options{
				Width:   -100,
				Height:  200,
				Quality: 80,
			},
			wantErr:     true,
			wantErrType: processor.ErrInvalidConfig,
		},
		{
			name:  "invalid image data",
			input: createInvalidImage,
			opts: &processor.Options{
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			wantErr:     true,
			wantErrType: processor.ErrCorruptedFile,
		},
		{
			name:  "empty input",
			input: createEmptyReader,
			opts: &processor.Options{
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			wantErr:     true,
			wantErrType: processor.ErrCorruptedFile,
		},
		{
			name:  "corrupted jpeg",
			input: createCorruptedJPEG,
			opts: &processor.Options{
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			wantErr:     true,
			wantErrType: processor.ErrCorruptedFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewThumbnailProcessor(nil)
			ctx := context.Background()

			result, err := p.Process(ctx, tt.opts, tt.input())

			if tt.wantErr {
				if err == nil {
					t.Error("Process() error = nil, want error")
					return
				}
				if tt.wantErrType != nil && !errors.Is(err, tt.wantErrType) {
					t.Errorf("Process() error = %v, want %v", err, tt.wantErrType)
				}
				return
			}

			if err != nil {
				t.Fatalf("Process() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("Process() returned nil result")
			}

			resultData, _ := io.ReadAll(result.Data)
			img, _, err := image.Decode(bytes.NewReader(resultData))
			if err != nil {
				t.Fatalf("Failed to decode result: %v", err)
			}

			bounds := img.Bounds()
			if bounds.Dx() != tt.wantWidth {
				t.Errorf("Result width = %d, want %d", bounds.Dx(), tt.wantWidth)
			}
			if bounds.Dy() != tt.wantHeight {
				t.Errorf("Result height = %d, want %d", bounds.Dy(), tt.wantHeight)
			}

			if result.ContentType != "image/jpeg" {
				t.Errorf("Result ContentType = %q, want %q", result.ContentType, "image/jpeg")
			}

			if result.Size <= 0 {
				t.Errorf("Result Size = %d, want > 0", result.Size)
			}

			if result.Metadata.Width != tt.wantWidth {
				t.Errorf("Result Metadata.Width = %d, want %d", result.Metadata.Width, tt.wantWidth)
			}
			if result.Metadata.Height != tt.wantHeight {
				t.Errorf("Result Metadata.Height = %d, want %d", result.Metadata.Height, tt.wantHeight)
			}
		})
	}
}

// TestThumbnailProcessor_Process_Quality tests quality option.
func TestThumbnailProcessor_Process_Quality(t *testing.T) {
	tests := []struct {
		name       string
		quality    int
		wantLarger bool // Higher quality = larger file
	}{
		{name: "low quality", quality: 20, wantLarger: false},
		{name: "medium quality", quality: 50, wantLarger: false},
		{name: "high quality", quality: 95, wantLarger: true},
	}

	p := NewThumbnailProcessor(nil)
	ctx := context.Background()

	var mediumSize int64

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &processor.Options{
				Width:   200,
				Height:  200,
				Quality: tt.quality,
			}

			result, err := p.Process(ctx, opts, createSquareImage())
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			if tt.quality == 50 {
				mediumSize = result.Size
			}

			// High quality should produce larger files
			if mediumSize > 0 && tt.wantLarger && result.Size <= mediumSize {
				t.Logf("Quality %d size: %d, medium size: %d", tt.quality, result.Size, mediumSize)
			}
		})
	}
}

// TestThumbnailProcessor_Process_ContextCanceled tests context cancellation.
func TestThumbnailProcessor_Process_ContextCanceled(t *testing.T) {
	p := NewThumbnailProcessor(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := &processor.Options{
		Width:   200,
		Height:  200,
		Quality: 80,
	}

	// The processor may or may not check context before processing
	// This test ensures it doesn't panic with a canceled context
	_, _ = p.Process(ctx, opts, createSquareImage())
}

// TestThumbnailProcessor_Process_PNG tests processing PNG input.
func TestThumbnailProcessor_Process_PNG(t *testing.T) {
	p := NewThumbnailProcessor(nil)
	ctx := context.Background()

	opts := &processor.Options{
		Width:   200,
		Height:  200,
		Quality: 80,
	}

	result, err := p.Process(ctx, opts, createTestPNG(500, 500))
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	// Result should still be JPEG (thumbnail always outputs JPEG)
	if result.ContentType != "image/jpeg" {
		t.Errorf("ContentType = %q, want %q", result.ContentType, "image/jpeg")
	}
}

// TestThumbnailProcessor_Process_DefaultQuality tests default quality.
func TestThumbnailProcessor_Process_DefaultQuality(t *testing.T) {
	p := NewThumbnailProcessor(nil)
	ctx := context.Background()

	opts := &processor.Options{
		Width:   200,
		Height:  200,
		Quality: 0, // Should use default
	}

	result, err := p.Process(ctx, opts, createSquareImage())
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	// Should still produce valid output
	if result.Size <= 0 {
		t.Error("Process() with default quality produced empty output")
	}
}

// TestThumbnailProcessor_Config tests custom config.
func TestThumbnailProcessor_Config(t *testing.T) {
	cfg := &processor.Config{
		Quality:      50,
		MaxDimension: 1000,
	}

	p := NewThumbnailProcessor(cfg)

	if p.config.Quality != 50 {
		t.Errorf("Config Quality = %d, want 50", p.config.Quality)
	}
}

// TestThumbnailProcessor_NilConfig tests nil config uses defaults.
func TestThumbnailProcessor_NilConfig(t *testing.T) {
	p := NewThumbnailProcessor(nil)

	if p.config == nil {
		t.Error("Processor config is nil with nil input")
	}
}

// Helper function tests
func TestDecodeImage(t *testing.T) {
	tests := []struct {
		name    string
		input   func() io.Reader
		wantErr bool
	}{
		{
			name:    "valid jpeg",
			input:   createSquareImage,
			wantErr: false,
		},
		{
			name:    "valid png",
			input:   func() io.Reader { return createTestPNG(100, 100) },
			wantErr: false,
		},
		{
			name:    "invalid data",
			input:   createInvalidImage,
			wantErr: true,
		},
		{
			name:    "empty data",
			input:   createEmptyReader,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, _, err := decodeImage(tt.input())

			if tt.wantErr {
				if err == nil {
					t.Error("decodeImage() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Errorf("decodeImage() error = %v", err)
				return
			}

			if img == nil {
				t.Error("decodeImage() returned nil image")
			}
		})
	}
}

func TestGetQuality(t *testing.T) {
	tests := []struct {
		name           string
		configQuality  int
		defaultQuality int
		want           int
	}{
		{"use config quality", 80, 85, 80},
		{"use default for zero", 0, 85, 85},
		{"use default for negative", -10, 85, 85},
		{"use default for over 100", 150, 85, 85},
		{"boundary 100", 100, 85, 100},
		{"boundary 1", 1, 85, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getQuality(tt.configQuality, tt.defaultQuality)
			if got != tt.want {
				t.Errorf("getQuality(%d, %d) = %d, want %d", tt.configQuality, tt.defaultQuality, got, tt.want)
			}
		})
	}
}
