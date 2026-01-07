package image

import (
	"bytes"
	"context"
	"image"
	"io"
	"testing"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
)

func TestWatermarkProcessor_Name(t *testing.T) {
	p := NewWatermarkProcessor(nil)
	if got := p.Name(); got != "watermark" {
		t.Errorf("Name() = %v, want watermark", got)
	}
}

func TestWatermarkProcessor_SupportedTypes(t *testing.T) {
	p := NewWatermarkProcessor(nil)
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

func TestWatermarkProcessor_Process(t *testing.T) {
	tests := []struct {
		name       string
		createImg  func() io.Reader
		opts       *processor.Options
		wantWidth  int
		wantHeight int
		wantErr    bool
	}{
		{
			name:       "watermark jpeg",
			createImg:  func() io.Reader { return createTestJPEG(400, 300) },
			opts:       &processor.Options{VariantType: "Test Watermark", Quality: 80},
			wantWidth:  400,
			wantHeight: 300,
			wantErr:    false,
		},
		{
			name:       "watermark png",
			createImg:  func() io.Reader { return createTestPNG(200, 150) },
			opts:       &processor.Options{VariantType: "PNG Test"},
			wantWidth:  200,
			wantHeight: 150,
			wantErr:    false,
		},
		{
			name:       "default watermark text",
			createImg:  func() io.Reader { return createTestJPEG(100, 100) },
			opts:       &processor.Options{},
			wantWidth:  100,
			wantHeight: 100,
			wantErr:    false,
		},
		{
			name:       "bottom-left position",
			createImg:  func() io.Reader { return createTestJPEG(300, 200) },
			opts:       &processor.Options{Fit: "bottom-left"},
			wantWidth:  300,
			wantHeight: 200,
			wantErr:    false,
		},
		{
			name:       "center position",
			createImg:  func() io.Reader { return createTestJPEG(300, 200) },
			opts:       &processor.Options{Fit: "center"},
			wantWidth:  300,
			wantHeight: 200,
			wantErr:    false,
		},
		{
			name:      "invalid image",
			createImg: func() io.Reader { return createInvalidImage() },
			opts:      &processor.Options{},
			wantErr:   true,
		},
	}

	p := NewWatermarkProcessor(processor.DefaultConfig())

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

			_, _, err = image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Errorf("result is not a valid image: %v", err)
			}
		})
	}
}

func TestWatermarkProcessor_Positions(t *testing.T) {
	positions := []string{
		"top-left",
		"top-right",
		"bottom-left",
		"bottom-right",
		"center",
	}

	p := NewWatermarkProcessor(processor.DefaultConfig())

	for _, pos := range positions {
		t.Run(pos, func(t *testing.T) {
			result, err := p.Process(
				context.Background(),
				&processor.Options{Fit: pos, VariantType: "Test"},
				createTestJPEG(200, 200),
			)
			if err != nil {
				t.Errorf("position %s failed: %v", pos, err)
				return
			}
			if result.Size <= 0 {
				t.Errorf("position %s produced empty result", pos)
			}
		})
	}
}

func TestCalculateTextPosition(t *testing.T) {
	tests := []struct {
		position string
		wantAx   float64
		wantAy   float64
	}{
		{"top-left", 0, 0},
		{"top-right", 1, 0},
		{"bottom-left", 0, 1},
		{"bottom-right", 1, 1},
		{"center", 0.5, 0.5},
		{"unknown", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.position, func(t *testing.T) {
			_, _, ax, ay := calculateTextPosition(100, 100, tt.position, 12)
			if ax != tt.wantAx || ay != tt.wantAy {
				t.Errorf("position %s: got anchors (%.1f, %.1f), want (%.1f, %.1f)",
					tt.position, ax, ay, tt.wantAx, tt.wantAy)
			}
		})
	}
}

func TestParseWatermarkOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     *processor.Options
		wantText string
		wantPos  string
	}{
		{
			name:     "defaults",
			opts:     &processor.Options{},
			wantText: "file.cheap",
			wantPos:  "bottom-right",
		},
		{
			name:     "custom text via VariantType",
			opts:     &processor.Options{VariantType: "Custom Text"},
			wantText: "Custom Text",
			wantPos:  "bottom-right",
		},
		{
			name:     "custom position via Fit",
			opts:     &processor.Options{Fit: "center"},
			wantText: "file.cheap",
			wantPos:  "center",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wo := parseWatermarkOptions(tt.opts)
			if wo.Text != tt.wantText {
				t.Errorf("Text = %v, want %v", wo.Text, tt.wantText)
			}
			if wo.Position != tt.wantPos {
				t.Errorf("Position = %v, want %v", wo.Position, tt.wantPos)
			}
		})
	}
}
