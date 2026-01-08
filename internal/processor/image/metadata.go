package image

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
	_ "golang.org/x/image/webp"
)

type MetadataProcessor struct {
	cfg *processor.Config
}

func NewMetadataProcessor(cfg *processor.Config) *MetadataProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &MetadataProcessor{cfg: cfg}
}

func (p *MetadataProcessor) Name() string {
	return "metadata"
}

func (p *MetadataProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/bmp",
	}
}

type ImageMetadata struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Format    string `json:"format"`
	ColorMode string `json:"color_mode,omitempty"`
}

func (p *MetadataProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, processor.ErrCorruptedFile
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, processor.ErrCorruptedFile
	}

	meta := ImageMetadata{
		Width:  cfg.Width,
		Height: cfg.Height,
		Format: format,
	}

	jsonData, err := json.Marshal(meta)
	if err != nil {
		return nil, processor.ErrProcessingFailed
	}

	return &processor.Result{
		Data:        bytes.NewReader(jsonData),
		ContentType: "application/json",
		Size:        int64(len(jsonData)),
		Metadata: processor.ResultMetadata{
			Width:  cfg.Width,
			Height: cfg.Height,
			Format: format,
		},
	}, nil
}
