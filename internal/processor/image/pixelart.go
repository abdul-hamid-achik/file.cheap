package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

var _ processor.Processor = (*PixelArtProcessor)(nil)

type PixelArtProcessor struct {
	config *processor.Config
}

func NewPixelArtProcessor(cfg *processor.Config) *PixelArtProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &PixelArtProcessor{config: cfg}
}

func (p *PixelArtProcessor) Name() string {
	return "pixelart"
}

func (p *PixelArtProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/bmp",
		"image/webp",
	}
}

func (p *PixelArtProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	// Decode the input image
	img, _, err := image.Decode(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", processor.ErrCorruptedFile, err)
	}

	// Determine pixel size (block size) from opts.Width
	// Default to 16 if not set
	pixelSize := 16
	if opts.Width > 0 {
		pixelSize = opts.Width
	}

	// Get original dimensions
	origBounds := img.Bounds()
	origW, origH := origBounds.Dx(), origBounds.Dy()

	// Calculate downscaled dimensions
	// Each pixel size block becomes one pixel
	downW := origW / pixelSize
	downH := origH / pixelSize
	
	// Ensure at least 1x1
	if downW < 1 {
		downW = 1
	}
	if downH < 1 {
		downH = 1
	}

	// Step 1: Downscale using nearest-neighbor (sample top-left of each block)
	downscaled := image.NewRGBA(image.Rect(0, 0, downW, downH))
	for y := 0; y < downH; y++ {
		for x := 0; x < downW; x++ {
			// Sample the top-left pixel of each block
			srcX := x * pixelSize
			srcY := y * pixelSize
			// Ensure we don't go out of bounds
			if srcX >= origW {
				srcX = origW - 1
			}
			if srcY >= origH {
				srcY = origH - 1
			}
			c := img.At(srcX, srcY)
			downscaled.Set(x, y, c)
		}
	}

	// Step 2: Upscale back to original dimensions by filling each block with single color
	upscaled := image.NewRGBA(image.Rect(0, 0, origW, origH))
	for y := 0; y < origH; y++ {
		for x := 0; x < origW; x++ {
			// Determine which block this pixel belongs to
			blockX := x / pixelSize
			blockY := y / pixelSize
			// Handle edge case where we exceed downscaled dimensions
			if blockX >= downW {
				blockX = downW - 1
			}
			if blockY >= downH {
				blockY = downH - 1
			}
			c := downscaled.At(blockX, blockY)
			upscaled.Set(x, y, c)
		}
	}

	// Determine output format
	outputFormat := "png" // Default to PNG (better for pixel art)
	if opts.Format != "" {
		outputFormat = opts.Format
	}

	// Determine quality
	quality := opts.Quality
	if quality <= 0 {
		quality = p.config.Quality
	}

	// Encode the result
	var buf bytes.Buffer
	var contentType string

	switch outputFormat {
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, upscaled, &jpeg.Options{Quality: quality})
		contentType = "image/jpeg"
	case "png":
		err = png.Encode(&buf, upscaled)
		contentType = "image/png"
	default:
		// Default to PNG
		err = png.Encode(&buf, upscaled)
		contentType = "image/png"
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode %s: %w", outputFormat, err)
	}

	return &processor.Result{
		Data:        bytes.NewReader(buf.Bytes()),
		ContentType: contentType,
		Size:        int64(buf.Len()),
		Metadata: processor.ResultMetadata{
			Width:   origW,
			Height:  origH,
			Format:  outputFormat,
			Quality: quality,
		},
	}, nil
}
