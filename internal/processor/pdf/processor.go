package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
)

var (
	ErrPDFEncrypted   = errors.New("pdf: document is encrypted or password-protected")
	ErrPDFEmpty       = errors.New("pdf: document has no pages")
	ErrPageOutOfRange = errors.New("pdf: requested page is out of range")
)

type ThumbnailProcessor struct {
	config *processor.Config
}

var _ processor.Processor = (*ThumbnailProcessor)(nil)

func NewThumbnailProcessor(cfg *processor.Config) *ThumbnailProcessor {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &ThumbnailProcessor{config: cfg}
}

func (p *ThumbnailProcessor) Name() string {
	return "pdf_thumbnail"
}

func (p *ThumbnailProcessor) SupportedTypes() []string {
	return []string{"application/pdf"}
}

func (p *ThumbnailProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	if opts == nil {
		opts = &processor.Options{}
	}

	page := opts.Page
	if page <= 0 {
		page = 1
	}

	width := opts.Width
	if width <= 0 {
		width = 300
	}

	height := opts.Height
	if height <= 0 {
		height = 300
	}

	quality := opts.Quality
	if quality <= 0 {
		quality = p.config.Quality
	}

	format := strings.ToLower(opts.Format)
	if format == "" {
		format = "png"
	}
	if format == "jpg" {
		format = "jpeg"
	}
	if format != "png" && format != "jpeg" {
		format = "png"
	}

	tempDir, err := os.MkdirTemp(p.config.TempDir, "pdf-thumb-*")
	if err != nil {
		if os.IsNotExist(err) {
			tempDir, err = os.MkdirTemp("", "pdf-thumb-*")
			if err != nil {
				return nil, fmt.Errorf("%w: failed to create temp dir: %v", processor.ErrProcessingFailed, err)
			}
		} else {
			return nil, fmt.Errorf("%w: failed to create temp dir: %v", processor.ErrProcessingFailed, err)
		}
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	inputPath := filepath.Join(tempDir, "input.pdf")
	inputFile, err := os.Create(inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create input file: %v", processor.ErrProcessingFailed, err)
	}

	written, err := io.Copy(inputFile, input)
	if err != nil {
		_ = inputFile.Close()
		return nil, fmt.Errorf("%w: failed to write input file: %v", processor.ErrProcessingFailed, err)
	}
	_ = inputFile.Close()

	if written == 0 {
		return nil, fmt.Errorf("%w: empty input", processor.ErrCorruptedFile)
	}

	pageCount, err := p.getPageCount(ctx, inputPath)
	if err != nil {
		if strings.Contains(err.Error(), "Incorrect password") || strings.Contains(err.Error(), "encrypted") {
			return nil, ErrPDFEncrypted
		}
		return nil, fmt.Errorf("%w: %v", processor.ErrCorruptedFile, err)
	}

	if pageCount == 0 {
		return nil, ErrPDFEmpty
	}

	if page > pageCount {
		return nil, fmt.Errorf("%w: requested page %d but document has %d pages", ErrPageOutOfRange, page, pageCount)
	}

	outputPath := filepath.Join(tempDir, "output")

	var args []string
	if format == "jpeg" {
		args = []string{
			"-jpeg",
			"-jpegopt", fmt.Sprintf("quality=%d", quality),
		}
	} else {
		args = []string{"-png"}
	}

	scaleWidth := width
	if height > width {
		scaleWidth = height
	}

	args = append(args,
		"-f", strconv.Itoa(page),
		"-l", strconv.Itoa(page),
		"-scale-to", strconv.Itoa(scaleWidth),
		"-singlefile",
		inputPath,
		outputPath,
	)

	cmd := exec.CommandContext(ctx, "pdftoppm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		if strings.Contains(errMsg, "Incorrect password") || strings.Contains(errMsg, "encrypted") {
			return nil, ErrPDFEncrypted
		}
		return nil, fmt.Errorf("%w: pdftoppm failed: %v, output: %s", processor.ErrProcessingFailed, err, errMsg)
	}

	var thumbnailPath string
	if format == "jpeg" {
		thumbnailPath = outputPath + ".jpg"
	} else {
		thumbnailPath = outputPath + ".png"
	}

	thumbnailData, err := os.ReadFile(thumbnailPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read output: %v", processor.ErrProcessingFailed, err)
	}

	var contentType string
	var ext string
	if format == "jpeg" {
		contentType = "image/jpeg"
		ext = "jpg"
	} else {
		contentType = "image/png"
		ext = "png"
	}

	return &processor.Result{
		Data:        bytes.NewReader(thumbnailData),
		ContentType: contentType,
		Filename:    fmt.Sprintf("preview.%s", ext),
		Size:        int64(len(thumbnailData)),
		Metadata: processor.ResultMetadata{
			Width:   width,
			Height:  height,
			Format:  format,
			Quality: quality,
		},
	}, nil
}

func (p *ThumbnailProcessor) getPageCount(ctx context.Context, pdfPath string) (int, error) {
	cmd := exec.CommandContext(ctx, "pdfinfo", pdfPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo failed: %w, output: %s", err, string(output))
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				count, err := strconv.Atoi(parts[1])
				if err != nil {
					return 0, fmt.Errorf("failed to parse page count: %w", err)
				}
				return count, nil
			}
		}
	}

	return 0, fmt.Errorf("could not determine page count")
}
