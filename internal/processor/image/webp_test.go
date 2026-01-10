package image

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

func TestWebPProcessor_Name(t *testing.T) {
	p := NewWebPProcessor(nil)
	if got := p.Name(); got != "webp" {
		t.Errorf("Name() = %v, want webp", got)
	}
}

func TestWebPProcessor_SupportedTypes(t *testing.T) {
	p := NewWebPProcessor(nil)
	types := p.SupportedTypes()

	expected := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/bmp":  true,
	}

	if len(types) != len(expected) {
		t.Errorf("SupportedTypes() returned %d types, want %d", len(types), len(expected))
	}

	for _, typ := range types {
		if !expected[typ] {
			t.Errorf("unexpected type: %s", typ)
		}
	}
}

func TestWebPProcessor_Process(t *testing.T) {
	hasCwebp := cwebpInstalled()

	tests := []struct {
		name        string
		createImg   func() io.Reader
		opts        *processor.Options
		wantWidth   int
		wantHeight  int
		wantErr     bool
		skipNoCwebp bool
	}{
		{
			name:        "convert jpeg to webp",
			createImg:   func() io.Reader { return createTestJPEG(400, 300) },
			opts:        &processor.Options{Quality: 80},
			wantWidth:   400,
			wantHeight:  300,
			wantErr:     !hasCwebp,
			skipNoCwebp: true,
		},
		{
			name:        "convert png to webp",
			createImg:   func() io.Reader { return createTestPNG(200, 150) },
			opts:        &processor.Options{Quality: 90},
			wantWidth:   200,
			wantHeight:  150,
			wantErr:     !hasCwebp,
			skipNoCwebp: true,
		},
		{
			name:        "default quality",
			createImg:   func() io.Reader { return createTestJPEG(100, 100) },
			opts:        &processor.Options{},
			wantWidth:   100,
			wantHeight:  100,
			wantErr:     !hasCwebp,
			skipNoCwebp: true,
		},
		{
			name:      "invalid image",
			createImg: func() io.Reader { return createInvalidImage() },
			opts:      &processor.Options{},
			wantErr:   true,
		},
	}

	p := NewWebPProcessor(processor.DefaultConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipNoCwebp && !hasCwebp {
				t.Skip("cwebp not installed, skipping WebP conversion test")
			}

			imgReader := tt.createImg()
			result, err := p.Process(context.Background(), tt.opts, imgReader)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ContentType != "image/webp" {
				t.Errorf("ContentType = %v, want image/webp", result.ContentType)
			}

			if result.Metadata.Width != tt.wantWidth {
				t.Errorf("Width = %v, want %v", result.Metadata.Width, tt.wantWidth)
			}

			if result.Metadata.Height != tt.wantHeight {
				t.Errorf("Height = %v, want %v", result.Metadata.Height, tt.wantHeight)
			}

			if result.Size <= 0 {
				t.Error("Size should be greater than 0")
			}

			data, err := io.ReadAll(result.Data)
			if err != nil {
				t.Fatalf("failed to read result data: %v", err)
			}

			if len(data) == 0 {
				t.Error("result data is empty")
			}

			if !isWebP(data) {
				t.Error("result does not appear to be valid WebP (missing RIFF/WEBP header)")
			}
		})
	}
}

func TestWebPProcessor_Config(t *testing.T) {
	if !cwebpInstalled() {
		t.Skip("cwebp not installed, skipping config test")
	}

	cfg := &processor.Config{
		Quality:      75,
		MaxFileSize:  50 * 1024 * 1024,
		MaxDimension: 2048,
	}

	p := NewWebPProcessor(cfg)

	result, err := p.Process(context.Background(), &processor.Options{}, createTestJPEG(100, 100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Metadata.Quality != 75 {
		t.Errorf("Quality = %v, want 75 (from config)", result.Metadata.Quality)
	}

	if result.ContentType != "image/webp" {
		t.Errorf("ContentType = %v, want image/webp", result.ContentType)
	}
}

func TestWebPProcessor_QualityBounds(t *testing.T) {
	if !cwebpInstalled() {
		t.Skip("cwebp not installed, skipping quality bounds test")
	}

	p := NewWebPProcessor(processor.DefaultConfig())

	tests := []struct {
		name        string
		quality     int
		wantQuality int
	}{
		{"negative quality uses default", -1, 85},
		{"zero quality uses default", 0, 85},
		{"quality over 100 capped", 150, 100},
		{"valid quality preserved", 50, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Process(context.Background(), &processor.Options{Quality: tt.quality}, createTestJPEG(50, 50))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Metadata.Quality != tt.wantQuality {
				t.Errorf("Quality = %v, want %v", result.Metadata.Quality, tt.wantQuality)
			}
		})
	}
}

func cwebpInstalled() bool {
	_, err := exec.LookPath("cwebp")
	return err == nil
}

func isWebP(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	return bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
}
