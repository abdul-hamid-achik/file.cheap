package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

// FFmpegProcessor implements VideoProcessor using FFmpeg
type FFmpegProcessor struct {
	config *VideoConfig
}

var _ VideoProcessor = (*FFmpegProcessor)(nil)
var _ processor.Processor = (*FFmpegProcessor)(nil)

// NewFFmpegProcessor creates a new FFmpeg-based video processor
func NewFFmpegProcessor(cfg *VideoConfig) (*FFmpegProcessor, error) {
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

	return &FFmpegProcessor{config: cfg}, nil
}

func (p *FFmpegProcessor) Name() string {
	return "video_transcode"
}

func (p *FFmpegProcessor) SupportedTypes() []string {
	return SupportedVideoTypes
}

// Process implements the standard Processor interface for basic transcoding
func (p *FFmpegProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	videoOpts := &VideoOptions{
		Options:       opts,
		Preset:        p.config.DefaultPreset,
		CRF:           p.config.DefaultCRF,
		MaxResolution: p.config.MaxResolution,
		OutputFormat:  "mp4",
	}

	if opts != nil && opts.Quality > 0 {
		// Map quality (1-100) to CRF (51-0, inverted)
		videoOpts.CRF = 51 - (opts.Quality * 51 / 100)
	}

	return p.Transcode(ctx, videoOpts, input)
}

// Transcode converts a video to the specified format and settings
func (p *FFmpegProcessor) Transcode(ctx context.Context, opts *VideoOptions, input io.Reader) (*processor.Result, error) {
	if opts == nil {
		opts = &VideoOptions{
			Preset:        p.config.DefaultPreset,
			CRF:           p.config.DefaultCRF,
			MaxResolution: p.config.MaxResolution,
			OutputFormat:  "mp4",
		}
	}

	// Create temp directory
	tempDir, err := p.createTempDir("transcode")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Write input to temp file
	inputPath := filepath.Join(tempDir, "input")
	if err := p.writeInputFile(inputPath, input); err != nil {
		return nil, err
	}

	// Get video metadata
	metadata, err := p.getMetadataFromFile(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideo, err)
	}

	// Check duration limit
	if p.config.MaxDuration > 0 && int(metadata.Duration) > p.config.MaxDuration {
		return nil, fmt.Errorf("%w: video is %.0fs, max is %ds", ErrVideoTooLong, metadata.Duration, p.config.MaxDuration)
	}

	// Determine output format and extension
	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		outputFormat = "mp4"
	}

	ext := outputFormat
	if outputFormat == "hls" {
		ext = "m3u8"
	}
	outputPath := filepath.Join(tempDir, fmt.Sprintf("output.%s", ext))

	// Build ffmpeg command
	args := p.buildTranscodeArgs(opts, metadata, inputPath, outputPath)

	// Execute ffmpeg
	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: ffmpeg failed: %v, output: %s", ErrTranscodeFailed, err, string(output))
	}

	// Read output file
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read output: %v", ErrTranscodeFailed, err)
	}

	// Determine content type
	contentType := p.getContentType(outputFormat)

	return &processor.Result{
		Data:        bytes.NewReader(outputData),
		ContentType: contentType,
		Filename:    fmt.Sprintf("video.%s", ext),
		Size:        int64(len(outputData)),
		Metadata: processor.ResultMetadata{
			Width:    metadata.Width,
			Height:   metadata.Height,
			Duration: metadata.Duration,
			Format:   outputFormat,
		},
	}, nil
}

// ExtractThumbnail extracts a frame from the video at the specified position
func (p *FFmpegProcessor) ExtractThumbnail(ctx context.Context, input io.Reader, atPercent float64) (*processor.Result, error) {
	if atPercent < 0 || atPercent > 1 {
		atPercent = 0.1 // Default to 10% into the video
	}

	// Create temp directory
	tempDir, err := p.createTempDir("thumbnail")
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
	metadata, err := p.getMetadataFromFile(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideo, err)
	}

	// Calculate timestamp
	timestamp := metadata.Duration * atPercent

	outputPath := filepath.Join(tempDir, "thumbnail.jpg")

	// Extract frame with ffmpeg
	args := []string{
		"-ss", fmt.Sprintf("%.2f", timestamp),
		"-i", inputPath,
		"-vframes", "1",
		"-q:v", "2", // High quality JPEG
		"-y",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to extract thumbnail: %v, output: %s", ErrTranscodeFailed, err, string(output))
	}

	// Read thumbnail
	thumbnailData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read thumbnail: %v", ErrTranscodeFailed, err)
	}

	return &processor.Result{
		Data:        bytes.NewReader(thumbnailData),
		ContentType: "image/jpeg",
		Filename:    "thumbnail.jpg",
		Size:        int64(len(thumbnailData)),
		Metadata: processor.ResultMetadata{
			Width:  metadata.Width,
			Height: metadata.Height,
			Format: "jpeg",
		},
	}, nil
}

// GetMetadata extracts metadata from a video file
func (p *FFmpegProcessor) GetMetadata(ctx context.Context, input io.Reader) (*VideoMetadata, error) {
	// Create temp directory
	tempDir, err := p.createTempDir("metadata")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Write input to temp file
	inputPath := filepath.Join(tempDir, "input")
	if err := p.writeInputFile(inputPath, input); err != nil {
		return nil, err
	}

	return p.getMetadataFromFile(ctx, inputPath)
}

// GenerateHLS creates HLS segments and manifest from a video
func (p *FFmpegProcessor) GenerateHLS(ctx context.Context, opts *VideoOptions, input io.Reader) (*HLSResult, error) {
	// Create temp directory
	tempDir, err := p.createTempDir("hls")
	if err != nil {
		return nil, err
	}
	// Note: Don't defer cleanup here, caller needs the files

	// Write input to temp file
	inputPath := filepath.Join(tempDir, "input")
	if err := p.writeInputFile(inputPath, input); err != nil {
		return nil, err
	}

	// Get metadata
	metadata, err := p.getMetadataFromFile(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideo, err)
	}

	segmentDuration := opts.HLSSegmentDuration
	if segmentDuration <= 0 {
		segmentDuration = p.config.HLSSegmentDuration
	}

	outputDir := filepath.Join(tempDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	manifestPath := filepath.Join(outputDir, "playlist.m3u8")

	// Build ffmpeg HLS command
	args := []string{
		"-i", inputPath,
		"-c:v", "libx264",
		"-preset", opts.Preset,
		"-crf", strconv.Itoa(opts.CRF),
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "hls",
		"-hls_time", strconv.Itoa(segmentDuration),
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(outputDir, "segment_%03d.ts"),
		"-y",
		manifestPath,
	}

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: HLS generation failed: %v, output: %s", ErrTranscodeFailed, err, string(output))
	}

	// Find all segment files
	segments, err := filepath.Glob(filepath.Join(outputDir, "segment_*.ts"))
	if err != nil {
		return nil, fmt.Errorf("failed to list segments: %w", err)
	}

	return &HLSResult{
		ManifestPath:  manifestPath,
		SegmentPaths:  segments,
		TotalDuration: metadata.Duration,
		SegmentCount:  len(segments),
		Resolutions:   []int{metadata.Height},
	}, nil
}

func (p *FFmpegProcessor) AddWatermark(ctx context.Context, input io.Reader, text, position string, opacity float64) (*processor.Result, error) {
	tempDir, err := p.createTempDir("watermark")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	inputPath := filepath.Join(tempDir, "input")
	if err := p.writeInputFile(inputPath, input); err != nil {
		return nil, err
	}

	metadata, err := p.getMetadataFromFile(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideo, err)
	}

	if p.config.MaxDuration > 0 && int(metadata.Duration) > p.config.MaxDuration {
		return nil, ErrVideoTooLong
	}

	outputPath := filepath.Join(tempDir, "output.mp4")

	x, y := p.getWatermarkPosition(position)
	fontsize := max(metadata.Height/20, 16)

	// Escape single quotes to prevent FFmpeg filter injection
	// In FFmpeg drawtext, single quotes are escaped by replacing ' with '\''
	escapedText := escapeFFmpegText(text)

	drawtext := fmt.Sprintf("drawtext=text='%s':fontsize=%d:fontcolor=white@%.1f:x=%s:y=%s",
		escapedText, fontsize, opacity, x, y)

	args := []string{
		"-i", inputPath,
		"-vf", drawtext,
		"-c:v", "libx264",
		"-preset", p.config.DefaultPreset,
		"-crf", strconv.Itoa(p.config.DefaultCRF),
		"-c:a", "aac",
		"-b:a", "128k",
		"-y",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: watermark failed: %v, output: %s", ErrTranscodeFailed, err, string(output))
	}

	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read output: %v", ErrTranscodeFailed, err)
	}

	return &processor.Result{
		Data:        bytes.NewReader(outputData),
		ContentType: "video/mp4",
		Filename:    "video.mp4",
		Size:        int64(len(outputData)),
		Metadata: processor.ResultMetadata{
			Width:    metadata.Width,
			Height:   metadata.Height,
			Duration: metadata.Duration,
			Format:   "mp4",
		},
	}, nil
}

func (p *FFmpegProcessor) getWatermarkPosition(position string) (x, y string) {
	padding := 10
	switch position {
	case "top-left":
		return strconv.Itoa(padding), strconv.Itoa(padding)
	case "top-right":
		return fmt.Sprintf("w-tw-%d", padding), strconv.Itoa(padding)
	case "bottom-left":
		return strconv.Itoa(padding), fmt.Sprintf("h-th-%d", padding)
	case "center":
		return "(w-tw)/2", "(h-th)/2"
	default:
		return fmt.Sprintf("w-tw-%d", padding), fmt.Sprintf("h-th-%d", padding)
	}
}

// escapeFFmpegText escapes text for use in FFmpeg drawtext filter.
// This prevents command injection through the watermark text parameter.
// FFmpeg drawtext escaping: single quotes become '\” and backslashes become '\\\\'
func escapeFFmpegText(text string) string {
	// First escape backslashes, then escape single quotes
	escaped := strings.ReplaceAll(text, "\\", "\\\\\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "'\\''")
	// Escape colons which are FFmpeg filter separators
	escaped = strings.ReplaceAll(escaped, ":", "\\:")
	return escaped
}

// Helper methods

func (p *FFmpegProcessor) createTempDir(prefix string) (string, error) {
	tempDir, err := os.MkdirTemp(p.config.TempDir, fmt.Sprintf("video-%s-*", prefix))
	if err != nil {
		if os.IsNotExist(err) {
			tempDir, err = os.MkdirTemp("", fmt.Sprintf("video-%s-*", prefix))
			if err != nil {
				return "", fmt.Errorf("%w: failed to create temp dir: %v", processor.ErrProcessingFailed, err)
			}
		} else {
			return "", fmt.Errorf("%w: failed to create temp dir: %v", processor.ErrProcessingFailed, err)
		}
	}
	return tempDir, nil
}

func (p *FFmpegProcessor) writeInputFile(path string, input io.Reader) error {
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

// ffprobeOutput represents the JSON output from ffprobe
type ffprobeOutput struct {
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
		BitRate    string `json:"bit_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
		BitRate  string `json:"bit_rate"`
		Name     string `json:"format_name"`
	} `json:"format"`
}

func (p *FFmpegProcessor) getMetadataFromFile(ctx context.Context, path string) (*VideoMetadata, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	}

	cmd := exec.CommandContext(ctx, p.config.FFprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	metadata := &VideoMetadata{}

	// Parse duration
	if probe.Format.Duration != "" {
		if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			metadata.Duration = d
		}
	}

	// Parse file size
	if probe.Format.Size != "" {
		if s, err := strconv.ParseInt(probe.Format.Size, 10, 64); err == nil {
			metadata.FileSize = s
		}
	}

	// Parse bitrate
	if probe.Format.BitRate != "" {
		if b, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
			metadata.Bitrate = b
		}
	}

	// Container format
	metadata.Container = strings.Split(probe.Format.Name, ",")[0]

	// Parse streams
	for _, stream := range probe.Streams {
		switch stream.CodecType {
		case "video":
			metadata.VideoCodec = stream.CodecName
			metadata.Width = stream.Width
			metadata.Height = stream.Height
			if stream.RFrameRate != "" {
				// Parse frame rate (format: "30/1" or "30000/1001")
				parts := strings.Split(stream.RFrameRate, "/")
				if len(parts) == 2 {
					num, _ := strconv.ParseFloat(parts[0], 64)
					den, _ := strconv.ParseFloat(parts[1], 64)
					if den > 0 {
						metadata.FrameRate = num / den
					}
				}
			}
		case "audio":
			metadata.AudioCodec = stream.CodecName
			metadata.HasAudio = true
		}
	}

	return metadata, nil
}

func (p *FFmpegProcessor) buildTranscodeArgs(opts *VideoOptions, metadata *VideoMetadata, inputPath, outputPath string) []string {
	args := []string{"-i", inputPath}

	// Video codec
	args = append(args, "-c:v", "libx264")

	// Preset
	preset := opts.Preset
	if preset == "" {
		preset = p.config.DefaultPreset
	}
	args = append(args, "-preset", preset)

	// CRF (quality)
	crf := opts.CRF
	if crf <= 0 {
		crf = p.config.DefaultCRF
	}
	args = append(args, "-crf", strconv.Itoa(crf))

	// Resolution scaling if needed
	maxRes := opts.MaxResolution
	if maxRes <= 0 {
		maxRes = p.config.MaxResolution
	}
	if metadata.Height > maxRes {
		// Scale down maintaining aspect ratio
		args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", maxRes))
	}

	// Video bitrate (optional)
	if opts.VideoBitrate != "" {
		args = append(args, "-b:v", opts.VideoBitrate)
	}

	// Audio codec
	if metadata.HasAudio {
		args = append(args, "-c:a", "aac")
		audioBitrate := opts.AudioBitrate
		if audioBitrate == "" {
			_, _, audioBitrate = GetResolutionPreset(maxRes)
		}
		args = append(args, "-b:a", audioBitrate)
	} else {
		args = append(args, "-an") // No audio
	}

	// Output format specific options
	if opts.OutputFormat == "mp4" {
		args = append(args, "-movflags", "+faststart") // Web optimization
	}

	// Overwrite output
	args = append(args, "-y", outputPath)

	return args
}

func (p *FFmpegProcessor) getContentType(format string) string {
	switch format {
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "hls":
		return "application/x-mpegURL"
	default:
		return "video/mp4"
	}
}
