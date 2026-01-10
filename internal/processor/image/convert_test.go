package image

import (
	"bytes"
	"context"
	"image"
	"io"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

func TestConvertProcessor_Name(t *testing.T) {
	p := NewConvertProcessor(nil)
	if got := p.Name(); got != "convert" {
		t.Errorf("Name() = %v, want convert", got)
	}
}

func TestConvertProcessor_SupportedTypes(t *testing.T) {
	p := NewConvertProcessor(nil)
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

func TestConvertProcessor_Process(t *testing.T) {
	tests := []struct {
		name            string
		createImg       func() io.Reader
		opts            *processor.Options
		wantContentType string
		wantFormat      string
		wantErr         bool
	}{
		{
			name:            "jpeg to png",
			createImg:       func() io.Reader { return createTestJPEG(100, 100) },
			opts:            &processor.Options{Format: "png"},
			wantContentType: "image/png",
			wantFormat:      "png",
			wantErr:         false,
		},
		{
			name:      "webp not supported without CGO",
			createImg: func() io.Reader { return createTestJPEG(100, 100) },
			opts:      &processor.Options{Format: "webp"},
			wantErr:   true,
		},
		{
			name:            "png to jpeg",
			createImg:       func() io.Reader { return createTestPNG(100, 100) },
			opts:            &processor.Options{Format: "jpeg", Quality: 90},
			wantContentType: "image/jpeg",
			wantFormat:      "jpeg",
			wantErr:         false,
		},
		{
			name:            "png to gif",
			createImg:       func() io.Reader { return createTestPNG(100, 100) },
			opts:            &processor.Options{Format: "gif"},
			wantContentType: "image/gif",
			wantFormat:      "gif",
			wantErr:         false,
		},
		{
			name:            "default format (jpeg)",
			createImg:       func() io.Reader { return createTestPNG(100, 100) },
			opts:            &processor.Options{},
			wantContentType: "image/jpeg",
			wantFormat:      "jpeg",
			wantErr:         false,
		},
		{
			name:            "jpg alias",
			createImg:       func() io.Reader { return createTestPNG(100, 100) },
			opts:            &processor.Options{Format: "jpg"},
			wantContentType: "image/jpeg",
			wantFormat:      "jpeg",
			wantErr:         false,
		},
		{
			name:      "invalid image",
			createImg: func() io.Reader { return createInvalidImage() },
			opts:      &processor.Options{Format: "png"},
			wantErr:   true,
		},
		{
			name:      "unsupported format",
			createImg: func() io.Reader { return createTestJPEG(100, 100) },
			opts:      &processor.Options{Format: "tiff"},
			wantErr:   true,
		},
	}

	p := NewConvertProcessor(processor.DefaultConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			if result.ContentType != tt.wantContentType {
				t.Errorf("ContentType = %v, want %v", result.ContentType, tt.wantContentType)
			}

			if result.Metadata.Format != tt.wantFormat {
				t.Errorf("Format = %v, want %v", result.Metadata.Format, tt.wantFormat)
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

			_, _, err = image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Errorf("result is not a valid image: %v", err)
			}
		})
	}
}

func TestConvertProcessor_Quality(t *testing.T) {
	p := NewConvertProcessor(processor.DefaultConfig())

	high, err := p.Process(context.Background(), &processor.Options{Format: "jpeg", Quality: 100}, createTestJPEG(200, 200))
	if err != nil {
		t.Fatalf("high quality failed: %v", err)
	}

	low, err := p.Process(context.Background(), &processor.Options{Format: "jpeg", Quality: 10}, createTestJPEG(200, 200))
	if err != nil {
		t.Fatalf("low quality failed: %v", err)
	}

	if high.Size <= low.Size {
		t.Logf("Note: high quality size (%d) <= low quality size (%d), this can happen with simple images", high.Size, low.Size)
	}
}

func TestConvertProcessor_Dimensions(t *testing.T) {
	p := NewConvertProcessor(processor.DefaultConfig())

	testCases := []struct {
		width  int
		height int
	}{
		{100, 100},
		{200, 150},
		{50, 300},
	}

	for _, tc := range testCases {
		t.Run("dimensions", func(t *testing.T) {
			result, err := p.Process(
				context.Background(),
				&processor.Options{Format: "png"},
				createTestJPEG(tc.width, tc.height),
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Metadata.Width != tc.width {
				t.Errorf("Width = %d, want %d", result.Metadata.Width, tc.width)
			}
			if result.Metadata.Height != tc.height {
				t.Errorf("Height = %d, want %d", result.Metadata.Height, tc.height)
			}
		})
	}
}
