package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
	"github.com/fogleman/gg"
)

var _ processor.Processor = (*WatermarkProcessor)(nil)

type WatermarkProcessor struct {
	config *processor.Config
}

func NewWatermarkProcessor(cfg *processor.Config) *WatermarkProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &WatermarkProcessor{config: cfg}
}

func (p *WatermarkProcessor) Name() string {
	return "watermark"
}

func (p *WatermarkProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/bmp",
	}
}

type WatermarkOptions struct {
	Text      string
	Position  string
	Opacity   float64
	FontSize  float64
	Color     color.Color
	IsPremium bool
}

func parseWatermarkOptions(opts *processor.Options) WatermarkOptions {
	wo := WatermarkOptions{
		Text:      "file.cheap",
		Position:  "bottom-right",
		Opacity:   0.5,
		FontSize:  24,
		Color:     color.White,
		IsPremium: false,
	}

	if opts.VariantType != "" {
		wo.Text = opts.VariantType
	}

	if opts.Fit != "" {
		wo.Position = opts.Fit
	}

	if opts.Quality > 0 && opts.Quality <= 100 {
		wo.Opacity = float64(opts.Quality) / 100.0
	}

	if opts.Width > 0 {
		wo.FontSize = float64(opts.Width)
	}

	return wo
}

func (p *WatermarkProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	img, format, err := image.Decode(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", processor.ErrCorruptedFile, err)
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	wo := parseWatermarkOptions(opts)

	watermarked, err := p.applyWatermark(img, wo)
	if err != nil {
		return nil, fmt.Errorf("failed to apply watermark: %w", err)
	}

	quality := opts.Quality
	if quality <= 0 || quality > 100 {
		quality = p.config.Quality
	}

	var buf bytes.Buffer
	outputFormat := format
	contentType := "image/jpeg"

	switch format {
	case "png":
		contentType = "image/png"
		if err := encodePNG(&buf, watermarked); err != nil {
			return nil, fmt.Errorf("failed to encode png: %w", err)
		}
	default:
		if err := jpeg.Encode(&buf, watermarked, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("failed to encode jpeg: %w", err)
		}
		outputFormat = "jpeg"
	}

	return &processor.Result{
		Data:        bytes.NewReader(buf.Bytes()),
		ContentType: contentType,
		Filename:    "watermarked." + outputFormat,
		Size:        int64(buf.Len()),
		Metadata: processor.ResultMetadata{
			Width:   width,
			Height:  height,
			Format:  outputFormat,
			Quality: quality,
		},
	}, nil
}

func (p *WatermarkProcessor) applyWatermark(img image.Image, opts WatermarkOptions) (image.Image, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	dc := gg.NewContext(width, height)
	dc.DrawImage(img, 0, 0)

	fontSize := opts.FontSize
	if fontSize < 12 {
		fontSize = 12
	}
	minDim := float64(min(width, height))
	if fontSize > minDim/4 {
		fontSize = minDim / 4
	}

	if err := dc.LoadFontFace("/System/Library/Fonts/Helvetica.ttc", fontSize); err != nil {
		if err := dc.LoadFontFace("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", fontSize); err != nil {
			dc.SetRGB(1, 1, 1)
		}
	}

	x, y, ax, ay := calculateTextPosition(width, height, opts.Position, fontSize)

	dc.SetRGBA(0, 0, 0, opts.Opacity*0.5)
	dc.DrawStringAnchored(opts.Text, x+2, y+2, ax, ay)

	dc.SetRGBA(1, 1, 1, opts.Opacity)
	dc.DrawStringAnchored(opts.Text, x, y, ax, ay)

	return dc.Image(), nil
}

func calculateTextPosition(width, height int, position string, fontSize float64) (x, y, ax, ay float64) {
	padding := fontSize * 0.5
	w := float64(width)
	h := float64(height)

	switch strings.ToLower(position) {
	case "top-left":
		return padding, padding, 0, 0
	case "top-right":
		return w - padding, padding, 1, 0
	case "bottom-left":
		return padding, h - padding, 0, 1
	case "bottom-right":
		return w - padding, h - padding, 1, 1
	case "center":
		return w / 2, h / 2, 0.5, 0.5
	default:
		return w - padding, h - padding, 1, 1
	}
}

func encodePNG(w io.Writer, img image.Image) error {
	return png.Encode(w, img)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
