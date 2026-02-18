package image

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

func TestPixelArtProcessor_Name(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	if got := p.Name(); got != "pixelart" {
		t.Errorf("Name() = %q, want %q", got, "pixelart")
	}
}

func TestPixelArtProcessor_SupportedTypes(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	types := p.SupportedTypes()

	expected := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/bmp":  true,
		"image/webp": true,
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

func TestPixelArtProcessor_Process_DefaultPixelSize(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	ctx := context.Background()

	input := createTestJPEG(320, 240)
	opts := &processor.Options{} // No options, should use default pixel size 16

	result, err := p.Process(ctx, opts, input)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if result == nil {
		t.Fatal("Process() returned nil result")
	}

	if result.Data == nil {
		t.Error("Process() returned nil Data")
	}

	if result.Size <= 0 {
		t.Errorf("Process() Size = %d, want > 0", result.Size)
	}

	if result.ContentType != "image/png" {
		t.Errorf("Process() ContentType = %q, want %q", result.ContentType, "image/png")
	}

	if result.Metadata.Width != 320 {
		t.Errorf("Process() Width = %d, want 320", result.Metadata.Width)
	}

	if result.Metadata.Height != 240 {
		t.Errorf("Process() Height = %d, want 240", result.Metadata.Height)
	}

	// Verify the result is a valid image
	resultData, _ := io.ReadAll(result.Data)
	_, _, err = image.Decode(bytes.NewReader(resultData))
	if err != nil {
		t.Errorf("Result is not a valid image: %v", err)
	}
}

func TestPixelArtProcessor_Process_CustomPixelSize8(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	ctx := context.Background()

	input := createTestPNG(400, 300)
	opts := &processor.Options{
		Width: 8, // Pixel size 8
	}

	result, err := p.Process(ctx, opts, input)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if result == nil {
		t.Fatal("Process() returned nil result")
	}

	if result.Size <= 0 {
		t.Errorf("Process() Size = %d, want > 0", result.Size)
	}

	// Verify the result is a valid image
	resultData, _ := io.ReadAll(result.Data)
	img, _, err := image.Decode(bytes.NewReader(resultData))
	if err != nil {
		t.Errorf("Result is not a valid image: %v", err)
	}

	// Check dimensions are preserved
	bounds := img.Bounds()
	if bounds.Dx() != 400 || bounds.Dy() != 300 {
		t.Errorf("Output dimensions = %dx%d, want 400x300", bounds.Dx(), bounds.Dy())
	}
}

func TestPixelArtProcessor_Process_CustomPixelSize32(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	ctx := context.Background()

	input := createTestJPEG(640, 480)
	opts := &processor.Options{
		Width: 32, // Pixel size 32
	}

	result, err := p.Process(ctx, opts, input)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if result == nil {
		t.Fatal("Process() returned nil result")
	}

	if result.Size <= 0 {
		t.Errorf("Process() Size = %d, want > 0", result.Size)
	}

	// Verify the result is a valid image
	resultData, _ := io.ReadAll(result.Data)
	_, _, err = image.Decode(bytes.NewReader(resultData))
	if err != nil {
		t.Errorf("Result is not a valid image: %v", err)
	}
}

func TestPixelArtProcessor_Process_JPEGOutput(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	ctx := context.Background()

	input := createTestJPEG(320, 240)
	opts := &processor.Options{
		Width:   16,
		Format:  "jpeg",
		Quality: 80,
	}

	result, err := p.Process(ctx, opts, input)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if result == nil {
		t.Fatal("Process() returned nil result")
	}

	if result.ContentType != "image/jpeg" {
		t.Errorf("Process() ContentType = %q, want %q", result.ContentType, "image/jpeg")
	}

	if result.Metadata.Format != "jpeg" {
		t.Errorf("Process() Format = %q, want %q", result.Metadata.Format, "jpeg")
	}

	if result.Metadata.Quality != 80 {
		t.Errorf("Process() Quality = %d, want 80", result.Metadata.Quality)
	}

	// Verify the result is a valid image
	resultData, _ := io.ReadAll(result.Data)
	_, _, err = image.Decode(bytes.NewReader(resultData))
	if err != nil {
		t.Errorf("Result is not a valid image: %v", err)
	}
}

func TestPixelArtProcessor_Process_InvalidImage(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	ctx := context.Background()

	tests := []struct {
		name  string
		input func() io.Reader
	}{
		{
			name:  "invalid data",
			input: createInvalidImage,
		},
		{
			name:  "empty data",
			input: createEmptyReader,
		},
		{
			name:  "corrupted jpeg",
			input: createCorruptedJPEG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &processor.Options{
				Width: 16,
			}

			_, err := p.Process(ctx, opts, tt.input())
			if err == nil {
				t.Error("Process() error = nil, want error")
			}
		})
	}
}

func TestPixelArtProcessor_Process_PixelSizeLargerThanImage(t *testing.T) {
	p := NewPixelArtProcessor(nil)
	ctx := context.Background()

	// Create a small 4x4 image
	input := createTestJPEG(4, 4)
	opts := &processor.Options{
		Width: 16, // Pixel size larger than image
	}

	result, err := p.Process(ctx, opts, input)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if result == nil {
		t.Fatal("Process() returned nil result")
	}

	// Should produce at least a 1x1 result (downscaled to 1x1, then upscaled back to 4x4)
	if result.Size <= 0 {
		t.Errorf("Process() Size = %d, want > 0", result.Size)
	}

	// Verify the result is a valid image
	resultData, _ := io.ReadAll(result.Data)
	img, _, err := image.Decode(bytes.NewReader(resultData))
	if err != nil {
		t.Errorf("Result is not a valid image: %v", err)
	}

	// Check dimensions are preserved (4x4 original)
	bounds := img.Bounds()
	if bounds.Dx() != 4 || bounds.Dy() != 4 {
		t.Errorf("Output dimensions = %dx%d, want 4x4", bounds.Dx(), bounds.Dy())
	}
}
