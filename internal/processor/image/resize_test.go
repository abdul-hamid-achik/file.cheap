package image

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	"io"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

// TestResizeProcessor_Name tests the processor name.
func TestResizeProcessor_Name(t *testing.T) {
	p := NewResizeProcessor(nil)

	if got := p.Name(); got != "resize" {
		t.Errorf("Name() = %q, want %q", got, "resize")
	}
}

// TestResizeProcessor_SupportedTypes tests supported content types.
func TestResizeProcessor_SupportedTypes(t *testing.T) {
	p := NewResizeProcessor(nil)
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

// TestResizeProcessor_Process_Fit tests the "fit" mode.
// Fit mode preserves aspect ratio and fits within the target bounds.
func TestResizeProcessor_Process_Fit(t *testing.T) {
	tests := []struct {
		name          string
		inputWidth    int
		inputHeight   int
		targetWidth   int
		targetHeight  int
		wantMaxWidth  int
		wantMaxHeight int
		wantRatioKept bool
	}{
		{
			name:          "landscape image fit",
			inputWidth:    800,
			inputHeight:   400,
			targetWidth:   200,
			targetHeight:  200,
			wantMaxWidth:  200,
			wantMaxHeight: 200,
			wantRatioKept: true,
		},
		{
			name:          "portrait image fit",
			inputWidth:    400,
			inputHeight:   800,
			targetWidth:   200,
			targetHeight:  200,
			wantMaxWidth:  200,
			wantMaxHeight: 200,
			wantRatioKept: true,
		},
		{
			name:          "square image fit",
			inputWidth:    600,
			inputHeight:   600,
			targetWidth:   200,
			targetHeight:  200,
			wantMaxWidth:  200,
			wantMaxHeight: 200,
			wantRatioKept: true,
		},
		{
			name:          "scale up small image",
			inputWidth:    100,
			inputHeight:   50,
			targetWidth:   400,
			targetHeight:  400,
			wantMaxWidth:  400,
			wantMaxHeight: 400,
			wantRatioKept: true,
		},
	}

	p := NewResizeProcessor(nil)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := createTestJPEG(tt.inputWidth, tt.inputHeight)

			opts := &processor.Options{
				Width:  tt.targetWidth,
				Height: tt.targetHeight,
				Fit:    "fit",
			}

			result, err := p.Process(ctx, opts, input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			// Decode result
			resultData, _ := io.ReadAll(result.Data)
			img, _, _ := image.Decode(bytes.NewReader(resultData))
			bounds := img.Bounds()

			// Fit mode should not exceed target dimensions
			if bounds.Dx() > tt.wantMaxWidth {
				t.Errorf("Width = %d, exceeds max %d", bounds.Dx(), tt.wantMaxWidth)
			}
			if bounds.Dy() > tt.wantMaxHeight {
				t.Errorf("Height = %d, exceeds max %d", bounds.Dy(), tt.wantMaxHeight)
			}

			if tt.wantRatioKept {
				inputRatio := float64(tt.inputWidth) / float64(tt.inputHeight)
				outputRatio := float64(bounds.Dx()) / float64(bounds.Dy())
				diff := inputRatio - outputRatio
				if diff < -0.1 || diff > 0.1 {
					t.Errorf("Aspect ratio changed: input %.2f, output %.2f", inputRatio, outputRatio)
				}
			}
		})
	}
}

// TestResizeProcessor_Process_Cover tests the "cover" mode.
// Cover mode fills the target dimensions exactly, cropping if necessary.
func TestResizeProcessor_Process_Cover(t *testing.T) {
	tests := []struct {
		name         string
		inputWidth   int
		inputHeight  int
		targetWidth  int
		targetHeight int
	}{
		{
			name:         "landscape to square",
			inputWidth:   800,
			inputHeight:  400,
			targetWidth:  200,
			targetHeight: 200,
		},
		{
			name:         "portrait to square",
			inputWidth:   400,
			inputHeight:  800,
			targetWidth:  200,
			targetHeight: 200,
		},
		{
			name:         "square to rectangle",
			inputWidth:   600,
			inputHeight:  600,
			targetWidth:  300,
			targetHeight: 200,
		},
	}

	p := NewResizeProcessor(nil)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := createTestJPEG(tt.inputWidth, tt.inputHeight)

			opts := &processor.Options{
				Width:  tt.targetWidth,
				Height: tt.targetHeight,
				Fit:    "cover",
			}

			result, err := p.Process(ctx, opts, input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			// Decode result
			resultData, _ := io.ReadAll(result.Data)
			img, _, _ := image.Decode(bytes.NewReader(resultData))
			bounds := img.Bounds()

			// Cover mode should produce exact dimensions
			if bounds.Dx() != tt.targetWidth {
				t.Errorf("Width = %d, want %d", bounds.Dx(), tt.targetWidth)
			}
			if bounds.Dy() != tt.targetHeight {
				t.Errorf("Height = %d, want %d", bounds.Dy(), tt.targetHeight)
			}
		})
	}
}

// TestResizeProcessor_Process_Fill tests the "fill" mode.
// Fill mode stretches to exact dimensions (may distort).
func TestResizeProcessor_Process_Fill(t *testing.T) {
	tests := []struct {
		name         string
		inputWidth   int
		inputHeight  int
		targetWidth  int
		targetHeight int
	}{
		{
			name:         "stretch landscape to square",
			inputWidth:   800,
			inputHeight:  400,
			targetWidth:  200,
			targetHeight: 200,
		},
		{
			name:         "stretch square to rectangle",
			inputWidth:   600,
			inputHeight:  600,
			targetWidth:  400,
			targetHeight: 200,
		},
	}

	p := NewResizeProcessor(nil)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := createTestJPEG(tt.inputWidth, tt.inputHeight)

			opts := &processor.Options{
				Width:  tt.targetWidth,
				Height: tt.targetHeight,
				Fit:    "fill",
			}

			result, err := p.Process(ctx, opts, input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			// Decode result
			resultData, _ := io.ReadAll(result.Data)
			img, _, _ := image.Decode(bytes.NewReader(resultData))
			bounds := img.Bounds()

			// Fill mode should produce exact dimensions
			if bounds.Dx() != tt.targetWidth {
				t.Errorf("Width = %d, want %d", bounds.Dx(), tt.targetWidth)
			}
			if bounds.Dy() != tt.targetHeight {
				t.Errorf("Height = %d, want %d", bounds.Dy(), tt.targetHeight)
			}
		})
	}
}

// TestResizeProcessor_Process_WidthOnly tests resizing with only width specified.
func TestResizeProcessor_Process_WidthOnly(t *testing.T) {
	p := NewResizeProcessor(nil)
	ctx := context.Background()

	tests := []struct {
		name        string
		inputWidth  int
		inputHeight int
		targetWidth int
	}{
		{
			name:        "landscape image",
			inputWidth:  800,
			inputHeight: 400,
			targetWidth: 400,
		},
		{
			name:        "portrait image",
			inputWidth:  400,
			inputHeight: 800,
			targetWidth: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := createTestJPEG(tt.inputWidth, tt.inputHeight)

			opts := &processor.Options{
				Width:  tt.targetWidth,
				Height: 0, // Height not specified
			}

			result, err := p.Process(ctx, opts, input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			// Decode result
			resultData, _ := io.ReadAll(result.Data)
			img, _, _ := image.Decode(bytes.NewReader(resultData))
			bounds := img.Bounds()

			// Width should match target
			if bounds.Dx() != tt.targetWidth {
				t.Errorf("Width = %d, want %d", bounds.Dx(), tt.targetWidth)
			}

			// Height should be calculated to preserve ratio
			expectedHeight := int(float64(tt.targetWidth) * float64(tt.inputHeight) / float64(tt.inputWidth))
			if bounds.Dy() != expectedHeight && bounds.Dy() != expectedHeight+1 && bounds.Dy() != expectedHeight-1 {
				t.Errorf("Height = %d, want ~%d (calculated from ratio)", bounds.Dy(), expectedHeight)
			}
		})
	}
}

// TestResizeProcessor_Process_HeightOnly tests resizing with only height specified.
func TestResizeProcessor_Process_HeightOnly(t *testing.T) {
	p := NewResizeProcessor(nil)
	ctx := context.Background()

	input := createTestJPEG(800, 400) // 2:1 ratio

	opts := &processor.Options{
		Width:  0,   // Width not specified
		Height: 200, // Only height
	}

	result, err := p.Process(ctx, opts, input)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	// Decode result
	resultData, _ := io.ReadAll(result.Data)
	img, _, _ := image.Decode(bytes.NewReader(resultData))
	bounds := img.Bounds()

	// Height should match
	if bounds.Dy() != 200 {
		t.Errorf("Height = %d, want 200", bounds.Dy())
	}

	// Width should be ~400 (2:1 ratio)
	expectedWidth := 400
	if bounds.Dx() < expectedWidth-5 || bounds.Dx() > expectedWidth+5 {
		t.Errorf("Width = %d, want ~%d", bounds.Dx(), expectedWidth)
	}
}

// TestResizeProcessor_Process_NoDimensions tests error when no dimensions specified.
func TestResizeProcessor_Process_NoDimensions(t *testing.T) {
	p := NewResizeProcessor(nil)
	ctx := context.Background()

	opts := &processor.Options{
		Width:  0,
		Height: 0,
	}

	_, err := p.Process(ctx, opts, createSquareImage())

	if err == nil {
		t.Error("Process() error = nil, want error for no dimensions")
	}

	if !errors.Is(err, processor.ErrInvalidConfig) {
		t.Errorf("Process() error = %v, want ErrInvalidConfig", err)
	}
}

// TestResizeProcessor_Process_DefaultFit tests that default fit mode is "fit".
func TestResizeProcessor_Process_DefaultFit(t *testing.T) {
	p := NewResizeProcessor(nil)
	ctx := context.Background()

	input := createTestJPEG(800, 400) // 2:1 ratio

	opts := &processor.Options{
		Width:  200,
		Height: 200,
		Fit:    "", // Empty fit mode
	}

	result, err := p.Process(ctx, opts, input)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	// With default "fit" mode, a 2:1 image resized to 200x200 should be 200x100
	resultData, _ := io.ReadAll(result.Data)
	img, _, _ := image.Decode(bytes.NewReader(resultData))
	bounds := img.Bounds()

	// Width should be 200
	if bounds.Dx() != 200 {
		t.Errorf("Width = %d, want 200", bounds.Dx())
	}

	// Height should be ~100 (maintaining 2:1 ratio)
	if bounds.Dy() > 200 {
		t.Errorf("Height = %d, should be <= 200 in fit mode", bounds.Dy())
	}
}

// TestResizeProcessor_Process_InvalidImage tests error handling for invalid input.
func TestResizeProcessor_Process_InvalidImage(t *testing.T) {
	tests := []struct {
		name    string
		input   func() io.Reader
		wantErr error
	}{
		{
			name:    "invalid data",
			input:   createInvalidImage,
			wantErr: processor.ErrCorruptedFile,
		},
		{
			name:    "empty data",
			input:   createEmptyReader,
			wantErr: processor.ErrCorruptedFile,
		},
		{
			name:    "corrupted jpeg",
			input:   createCorruptedJPEG,
			wantErr: processor.ErrCorruptedFile,
		},
	}

	p := NewResizeProcessor(nil)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &processor.Options{
				Width:  200,
				Height: 200,
			}

			_, err := p.Process(ctx, opts, tt.input())

			if err == nil {
				t.Error("Process() error = nil, want error")
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Process() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestResizeProcessor_Process_Metadata tests that metadata is correctly populated.
func TestResizeProcessor_Process_Metadata(t *testing.T) {
	p := NewResizeProcessor(nil)
	ctx := context.Background()

	opts := &processor.Options{
		Width:  300,
		Height: 200,
		Fit:    "cover",
	}

	result, err := p.Process(ctx, opts, createTestJPEG(800, 600))
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if result.Metadata.Width != 300 {
		t.Errorf("Metadata.Width = %d, want 300", result.Metadata.Width)
	}
	if result.Metadata.Height != 200 {
		t.Errorf("Metadata.Height = %d, want 200", result.Metadata.Height)
	}
	if result.ContentType != "image/jpeg" {
		t.Errorf("ContentType = %q, want %q", result.ContentType, "image/jpeg")
	}
	if result.Size <= 0 {
		t.Errorf("Size = %d, want > 0", result.Size)
	}
}

// TestResizeProcessor_Process_PNG tests processing PNG input.
func TestResizeProcessor_Process_PNG(t *testing.T) {
	p := NewResizeProcessor(nil)
	ctx := context.Background()

	opts := &processor.Options{
		Width:  200,
		Height: 200,
		Fit:    "cover",
	}

	result, err := p.Process(ctx, opts, createTestPNG(500, 500))
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	// Should produce valid output
	if result.Size <= 0 {
		t.Error("Process() produced empty output for PNG input")
	}
}

// Helper function tests

func TestCalculateDimensions(t *testing.T) {
	tests := []struct {
		name         string
		origWidth    int
		origHeight   int
		targetWidth  int
		targetHeight int
		wantWidth    int
		wantHeight   int
	}{
		{
			name:         "both specified",
			origWidth:    800,
			origHeight:   600,
			targetWidth:  400,
			targetHeight: 300,
			wantWidth:    400,
			wantHeight:   300,
		},
		{
			name:         "width only, landscape",
			origWidth:    800,
			origHeight:   400,
			targetWidth:  400,
			targetHeight: 0,
			wantWidth:    400,
			wantHeight:   200,
		},
		{
			name:         "height only, portrait",
			origWidth:    400,
			origHeight:   800,
			targetWidth:  0,
			targetHeight: 400,
			wantWidth:    200,
			wantHeight:   400,
		},
		{
			name:         "neither specified",
			origWidth:    800,
			origHeight:   600,
			targetWidth:  0,
			targetHeight: 0,
			wantWidth:    800,
			wantHeight:   600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := calculateDimensions(tt.origWidth, tt.origHeight, tt.targetWidth, tt.targetHeight)

			if gotW != tt.wantWidth {
				t.Errorf("calculateDimensions() width = %d, want %d", gotW, tt.wantWidth)
			}
			if gotH != tt.wantHeight {
				t.Errorf("calculateDimensions() height = %d, want %d", gotH, tt.wantHeight)
			}
		})
	}
}
