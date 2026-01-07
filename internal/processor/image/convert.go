package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
)

var _ processor.Processor = (*ConvertProcessor)(nil)

type ConvertProcessor struct {
	config *processor.Config
}

func NewConvertProcessor(cfg *processor.Config) *ConvertProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &ConvertProcessor{config: cfg}
}

func (p *ConvertProcessor) Name() string {
	return "convert"
}

func (p *ConvertProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/bmp",
	}
}

func (p *ConvertProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	img, _, err := image.Decode(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", processor.ErrCorruptedFile, err)
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	targetFormat := strings.ToLower(opts.Format)
	if targetFormat == "" {
		targetFormat = "jpeg"
	}

	quality := opts.Quality
	if quality <= 0 {
		quality = p.config.Quality
	}

	var buf bytes.Buffer
	var contentType string
	var filename string

	switch targetFormat {
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("failed to encode jpeg: %w", err)
		}
		contentType = "image/jpeg"
		filename = "converted.jpg"
		targetFormat = "jpeg"
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("failed to encode png: %w", err)
		}
		contentType = "image/png"
		filename = "converted.png"
	case "gif":
		if err := gif.Encode(&buf, img, nil); err != nil {
			return nil, fmt.Errorf("failed to encode gif: %w", err)
		}
		contentType = "image/gif"
		filename = "converted.gif"
	default:
		return nil, fmt.Errorf("%w: unsupported target format %q", processor.ErrInvalidConfig, targetFormat)
	}

	return &processor.Result{
		Data:        bytes.NewReader(buf.Bytes()),
		ContentType: contentType,
		Filename:    filename,
		Size:        int64(buf.Len()),
		Metadata: processor.ResultMetadata{
			Width:   width,
			Height:  height,
			Format:  targetFormat,
			Quality: quality,
		},
	}, nil
}
