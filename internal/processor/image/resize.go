package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/disintegration/imaging"
)

var _ processor.Processor = (*ResizeProcessor)(nil)

type ResizeProcessor struct {
	config *processor.Config
}

func NewResizeProcessor(cfg *processor.Config) *ResizeProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &ResizeProcessor{config: cfg}
}

func (p *ResizeProcessor) Name() string {
	return "resize"
}

func (p *ResizeProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/bmp",
	}
}

func (p *ResizeProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	if opts.Width <= 0 && opts.Height <= 0 {
		return nil, fmt.Errorf("%w: width or height is required", processor.ErrInvalidConfig)
	}

	img, format, err := image.Decode(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", processor.ErrCorruptedFile, err)
	}

	origBounds := img.Bounds()
	origW, origH := origBounds.Dx(), origBounds.Dy()

	targetW, targetH := calculateDimensions(origW, origH, opts.Width, opts.Height)
	resized := resizeImage(img, targetW, targetH, opts.Fit)

	actualBounds := resized.Bounds()
	actualW, actualH := actualBounds.Dx(), actualBounds.Dy()

	quality := opts.Quality
	if quality <= 0 {
		quality = p.config.Quality
	}

	outputFormat := format
	if opts.Format != "" {
		outputFormat = opts.Format
	}

	buf, contentType, err := encodeImage(resized, outputFormat, quality)
	if err != nil {
		return nil, err
	}

	return &processor.Result{
		Data:        bytes.NewReader(buf.Bytes()),
		ContentType: contentType,
		Size:        int64(buf.Len()),
		Metadata: processor.ResultMetadata{
			Width:   actualW,
			Height:  actualH,
			Format:  outputFormat,
			Quality: quality,
		},
	}, nil
}

func resizeImage(img image.Image, width, height int, fit string) image.Image {
	switch fit {
	case "cover":
		return imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)
	case "fill":
		return imaging.Resize(img, width, height, imaging.Lanczos)
	default:
		return imaging.Fit(img, width, height, imaging.Lanczos)
	}
}

func calculateDimensions(origWidth, origHeight, targetWidth, targetHeight int) (int, int) {
	if targetWidth == 0 && targetHeight == 0 {
		return origWidth, origHeight
	}

	if targetWidth == 0 {
		ratio := float64(origWidth) / float64(origHeight)
		targetWidth = int(float64(targetHeight) * ratio)
	} else if targetHeight == 0 {
		ratio := float64(origHeight) / float64(origWidth)
		targetHeight = int(float64(targetWidth) * ratio)
	}

	return targetWidth, targetHeight
}

func encodeImage(img image.Image, format string, quality int) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	var contentType string
	var err error

	switch format {
	case "png":
		err = imaging.Encode(&buf, img, imaging.PNG)
		contentType = "image/png"
	case "gif":
		err = imaging.Encode(&buf, img, imaging.GIF)
		contentType = "image/gif"
	case "bmp":
		err = imaging.Encode(&buf, img, imaging.BMP)
		contentType = "image/bmp"
	default:
		err = imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(quality))
		contentType = "image/jpeg"
	}

	if err != nil {
		return nil, "", fmt.Errorf("failed to encode %s: %w", format, err)
	}

	return &buf, contentType, nil
}
