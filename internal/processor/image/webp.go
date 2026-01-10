package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

var _ processor.Processor = (*WebPProcessor)(nil)

type WebPProcessor struct {
	config *processor.Config
}

func NewWebPProcessor(cfg *processor.Config) *WebPProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &WebPProcessor{config: cfg}
}

func (p *WebPProcessor) Name() string {
	return "webp"
}

func (p *WebPProcessor) SupportedTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/bmp",
	}
}

func (p *WebPProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	quality := opts.Quality
	if quality <= 0 {
		quality = p.config.Quality
	}
	if quality > 100 {
		quality = 100
	}

	inputData, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	img, _, err := image.DecodeConfig(bytes.NewReader(inputData))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", processor.ErrCorruptedFile, err)
	}
	width, height := img.Width, img.Height

	if cwebpAvailable() {
		return p.processWithCwebp(ctx, inputData, width, height, quality)
	}

	return p.processFallback(inputData, width, height, quality)
}

func cwebpAvailable() bool {
	_, err := exec.LookPath("cwebp")
	return err == nil
}

func (p *WebPProcessor) processWithCwebp(ctx context.Context, inputData []byte, width, height, quality int) (*processor.Result, error) {
	tempDir := p.config.TempDir
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	inputFile, err := os.CreateTemp(tempDir, "webp-input-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create input temp file: %w", err)
	}
	inputPath := inputFile.Name()
	defer func() { _ = os.Remove(inputPath) }()

	if _, err := inputFile.Write(inputData); err != nil {
		_ = inputFile.Close()
		return nil, fmt.Errorf("failed to write input data: %w", err)
	}
	_ = inputFile.Close()

	outputPath := filepath.Join(tempDir, fmt.Sprintf("webp-output-%d.webp", os.Getpid()))
	defer func() { _ = os.Remove(outputPath) }()

	args := []string{
		"-q", strconv.Itoa(quality),
		inputPath,
		"-o", outputPath,
	}

	cmd := exec.CommandContext(ctx, "cwebp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cwebp failed: %w, stderr: %s", err, stderr.String())
	}

	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	return &processor.Result{
		Data:        bytes.NewReader(outputData),
		ContentType: "image/webp",
		Filename:    "converted.webp",
		Size:        int64(len(outputData)),
		Metadata: processor.ResultMetadata{
			Width:   width,
			Height:  height,
			Format:  "webp",
			Quality: quality,
		},
	}, nil
}

func (p *WebPProcessor) processFallback(inputData []byte, width, height, quality int) (*processor.Result, error) {
	return &processor.Result{
		Data:        bytes.NewReader(inputData),
		ContentType: "image/webp",
		Filename:    "converted.webp",
		Size:        int64(len(inputData)),
		Metadata: processor.ResultMetadata{
			Width:   width,
			Height:  height,
			Format:  "webp",
			Quality: quality,
		},
	}, fmt.Errorf("cwebp binary not available, returning original image: %w", processor.ErrProcessingFailed)
}
