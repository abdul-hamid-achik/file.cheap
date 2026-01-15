package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/disintegration/imaging"
)

var _ processor.Processor = (*ThumbnailProcessor)(nil)

type ThumbnailProcessor struct {
	config *processor.Config
}

func NewThumbnailProcessor(cfg *processor.Config) *ThumbnailProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &ThumbnailProcessor{config: cfg}
}

func (p *ThumbnailProcessor) Name() string {
	return "thumbnail"
}

func (p *ThumbnailProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/bmp",
	}
}

func (p *ThumbnailProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	if opts.Width <= 0 || opts.Height <= 0 {
		return nil, fmt.Errorf("%w: width and height are required", processor.ErrInvalidConfig)
	}

	img, _, err := decodeImage(input)
	if err != nil {
		return nil, err
	}

	thumb := createThumbnail(img, opts.Width, opts.Height, opts.Position)
	quality := getQuality(opts.Quality, p.config.Quality)

	buf, err := encodeJPEG(thumb, quality)
	if err != nil {
		return nil, err
	}

	return &processor.Result{
		Data:        bytes.NewReader(buf.Bytes()),
		ContentType: "image/jpeg",
		Size:        int64(buf.Len()),
		Metadata: processor.ResultMetadata{
			Width:   opts.Width,
			Height:  opts.Height,
			Quality: quality,
		},
	}, nil
}

func decodeImage(r io.Reader) (image.Image, string, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", processor.ErrCorruptedFile, err)
	}
	return img, format, nil
}

func createThumbnail(img image.Image, width, height int, position string) image.Image {
	return imaging.Fill(img, width, height, getAnchor(position), imaging.Lanczos)
}

func getAnchor(position string) imaging.Anchor {
	switch position {
	case "north", "top":
		return imaging.Top
	case "south", "bottom":
		return imaging.Bottom
	case "west", "left":
		return imaging.Left
	case "east", "right":
		return imaging.Right
	case "north-west", "top-left":
		return imaging.TopLeft
	case "north-east", "top-right":
		return imaging.TopRight
	case "south-west", "bottom-left":
		return imaging.BottomLeft
	case "south-east", "bottom-right":
		return imaging.BottomRight
	default:
		return imaging.Center
	}
}

func encodeJPEG(img image.Image, quality int) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(quality))
	if err != nil {
		return nil, fmt.Errorf("failed to encode jpeg: %w", err)
	}
	return &buf, nil
}

func getQuality(configQuality, defaultQuality int) int {
	if configQuality > 0 && configQuality <= 100 {
		return configQuality
	}
	return defaultQuality
}
