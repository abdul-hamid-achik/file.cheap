package video

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

// ThumbnailProcessor extracts thumbnails from video files
type ThumbnailProcessor struct {
	config *VideoConfig
}

var _ processor.Processor = (*ThumbnailProcessor)(nil)

// NewThumbnailProcessor creates a new video thumbnail processor
func NewThumbnailProcessor(cfg *VideoConfig) (*ThumbnailProcessor, error) {
	if cfg == nil {
		cfg = DefaultVideoConfig()
	}

	// Verify ffmpeg is available
	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFFmpegNotFound, err)
	}

	// Verify ffprobe is available
	if _, err := exec.LookPath(cfg.FFprobePath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFFprobeNotFound, err)
	}

	return &ThumbnailProcessor{config: cfg}, nil
}

func (p *ThumbnailProcessor) Name() string {
	return "video_thumbnail"
}

func (p *ThumbnailProcessor) SupportedTypes() []string {
	return SupportedVideoTypes
}

// Process extracts a thumbnail from the video
func (p *ThumbnailProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	// Create temp directory
	tempDir, err := p.createTempDir()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Write input to temp file
	inputPath := filepath.Join(tempDir, "input")
	if err := p.writeInputFile(inputPath, input); err != nil {
		return nil, err
	}

	// Get video duration to calculate thumbnail position
	duration, err := p.getVideoDuration(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideo, err)
	}

	// Extract thumbnail at 10% into the video (or specified position)
	thumbnailPercent := 0.1
	if opts != nil && opts.Page > 0 && opts.Page <= 100 {
		// Use Page as percentage (1-100)
		thumbnailPercent = float64(opts.Page) / 100.0
	}
	timestamp := duration * thumbnailPercent

	// Determine output dimensions
	width := 320
	height := 180
	if opts != nil {
		if opts.Width > 0 {
			width = opts.Width
		}
		if opts.Height > 0 {
			height = opts.Height
		}
	}

	// Determine output format
	format := "jpeg"
	if opts != nil && opts.Format != "" {
		format = opts.Format
		if format == "jpg" {
			format = "jpeg"
		}
	}

	outputExt := "jpg"
	if format == "png" {
		outputExt = "png"
	}
	outputPath := filepath.Join(tempDir, fmt.Sprintf("thumbnail.%s", outputExt))

	// Determine quality
	quality := 85
	if opts != nil && opts.Quality > 0 {
		quality = opts.Quality
	}

	// Build ffmpeg command
	args := []string{
		"-ss", fmt.Sprintf("%.2f", timestamp),
		"-i", inputPath,
		"-vframes", "1",
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", width, height, width, height),
	}

	if format == "jpeg" {
		args = append(args, "-q:v", strconv.Itoa(31-quality*31/100)) // ffmpeg jpeg quality is inverted (2-31, lower is better)
	}

	args = append(args, "-y", outputPath)

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to extract thumbnail: %v, output: %s", ErrTranscodeFailed, err, string(output))
	}

	// Read thumbnail
	thumbnailData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read thumbnail: %v", processor.ErrProcessingFailed, err)
	}

	contentType := "image/jpeg"
	if format == "png" {
		contentType = "image/png"
	}

	return &processor.Result{
		Data:        bytes.NewReader(thumbnailData),
		ContentType: contentType,
		Filename:    fmt.Sprintf("thumbnail.%s", outputExt),
		Size:        int64(len(thumbnailData)),
		Metadata: processor.ResultMetadata{
			Width:   width,
			Height:  height,
			Format:  format,
			Quality: quality,
		},
	}, nil
}

func (p *ThumbnailProcessor) createTempDir() (string, error) {
	tempDir, err := os.MkdirTemp(p.config.TempDir, "video-thumb-*")
	if err != nil {
		if os.IsNotExist(err) {
			tempDir, err = os.MkdirTemp("", "video-thumb-*")
			if err != nil {
				return "", fmt.Errorf("%w: failed to create temp dir: %v", processor.ErrProcessingFailed, err)
			}
		} else {
			return "", fmt.Errorf("%w: failed to create temp dir: %v", processor.ErrProcessingFailed, err)
		}
	}
	return tempDir, nil
}

func (p *ThumbnailProcessor) writeInputFile(path string, input io.Reader) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("%w: failed to create input file: %v", processor.ErrProcessingFailed, err)
	}
	defer func() { _ = file.Close() }()

	written, err := io.Copy(file, input)
	if err != nil {
		return fmt.Errorf("%w: failed to write input file: %v", processor.ErrProcessingFailed, err)
	}

	if written == 0 {
		return fmt.Errorf("%w: empty input", processor.ErrCorruptedFile)
	}

	return nil
}

func (p *ThumbnailProcessor) getVideoDuration(ctx context.Context, path string) (float64, error) {
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	}

	cmd := exec.CommandContext(ctx, p.config.FFprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	duration, err := strconv.ParseFloat(string(bytes.TrimSpace(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

// GenerateThumbnailSprite creates a sprite sheet of thumbnails for video scrubbing
func (p *ThumbnailProcessor) GenerateThumbnailSprite(ctx context.Context, input io.Reader, cols, rows int, thumbWidth, thumbHeight int) (*processor.Result, error) {
	if cols <= 0 {
		cols = 10
	}
	if rows <= 0 {
		rows = 10
	}
	if thumbWidth <= 0 {
		thumbWidth = 160
	}
	if thumbHeight <= 0 {
		thumbHeight = 90
	}

	// Create temp directory
	tempDir, err := p.createTempDir()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Write input to temp file
	inputPath := filepath.Join(tempDir, "input")
	if err := p.writeInputFile(inputPath, input); err != nil {
		return nil, err
	}

	// Get duration
	duration, err := p.getVideoDuration(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideo, err)
	}

	totalFrames := cols * rows
	interval := duration / float64(totalFrames)

	outputPath := filepath.Join(tempDir, "sprite.jpg")

	// Build ffmpeg command for sprite generation
	// Extract frames at intervals and tile them into a sprite sheet
	args := []string{
		"-i", inputPath,
		"-vf", fmt.Sprintf("fps=1/%.2f,scale=%d:%d,tile=%dx%d", interval, thumbWidth, thumbHeight, cols, rows),
		"-frames:v", "1",
		"-q:v", "5",
		"-y",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate sprite: %v, output: %s", ErrTranscodeFailed, err, string(output))
	}

	// Read sprite
	spriteData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read sprite: %v", processor.ErrProcessingFailed, err)
	}

	return &processor.Result{
		Data:        bytes.NewReader(spriteData),
		ContentType: "image/jpeg",
		Filename:    "sprite.jpg",
		Size:        int64(len(spriteData)),
		Metadata: processor.ResultMetadata{
			Width:  thumbWidth * cols,
			Height: thumbHeight * rows,
			Format: "jpeg",
		},
	}, nil
}
