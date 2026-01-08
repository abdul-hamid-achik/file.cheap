package image

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

type OptimizeProcessor struct {
	cfg *processor.Config
}

func NewOptimizeProcessor(cfg *processor.Config) *OptimizeProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &OptimizeProcessor{cfg: cfg}
}

func (p *OptimizeProcessor) Name() string {
	return "optimize"
}

func (p *OptimizeProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
	}
}

func (p *OptimizeProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, processor.ErrCorruptedFile
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, processor.ErrCorruptedFile
	}

	quality := 85
	if opts != nil && opts.Quality > 0 {
		quality = opts.Quality
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	maxDim := 4096
	if width > maxDim || height > maxDim {
		if width > height {
			img = imaging.Resize(img, maxDim, 0, imaging.Lanczos)
		} else {
			img = imaging.Resize(img, 0, maxDim, imaging.Lanczos)
		}
		bounds = img.Bounds()
		width = bounds.Dx()
		height = bounds.Dy()
	}

	var buf bytes.Buffer
	var contentType string

	switch format {
	case "jpeg":
		contentType = "image/jpeg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case "png":
		contentType = "image/png"
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		err = encoder.Encode(&buf, img)
	default:
		contentType = "image/jpeg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	}

	if err != nil {
		return nil, processor.ErrProcessingFailed
	}

	return &processor.Result{
		Data:        bytes.NewReader(buf.Bytes()),
		ContentType: contentType,
		Size:        int64(buf.Len()),
		Metadata: processor.ResultMetadata{
			Width:  width,
			Height: height,
			Format: format,
		},
	}, nil
}
