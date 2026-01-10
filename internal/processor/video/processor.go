package video

import (
	"context"
	"errors"
	"io"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

var (
	ErrVideoTooLong      = errors.New("video: duration exceeds limit")
	ErrUnsupportedCodec  = errors.New("video: unsupported codec")
	ErrTranscodeFailed   = errors.New("video: transcoding failed")
	ErrFFmpegNotFound    = errors.New("video: ffmpeg not found in PATH")
	ErrFFprobeNotFound   = errors.New("video: ffprobe not found in PATH")
	ErrInvalidVideo      = errors.New("video: invalid or corrupted video file")
	ErrResolutionTooHigh = errors.New("video: resolution exceeds tier limit")
)

// VideoOptions extends processor.Options with video-specific settings
type VideoOptions struct {
	*processor.Options

	// Encoding settings
	Preset     string // ultrafast, superfast, veryfast, faster, fast, medium, slow, slower, veryslow
	CRF        int    // Constant Rate Factor (0-51, lower = better quality, 23 is default)
	VideoBitrate string // e.g., "2M", "5M"
	AudioBitrate string // e.g., "128k", "192k"

	// Output format
	OutputFormat string // mp4, webm, hls

	// Resolution
	MaxResolution int // Max height in pixels (480, 720, 1080, 2160)

	// Thumbnail extraction
	ThumbnailAt float64 // Extract thumbnail at this percentage of duration (0.0-1.0)

	// HLS specific
	HLSSegmentDuration int // Segment duration in seconds (default 10)
}

// VideoMetadata contains detailed video information
type VideoMetadata struct {
	Duration    float64 `json:"duration"`     // Duration in seconds
	Width       int     `json:"width"`        // Video width
	Height      int     `json:"height"`       // Video height
	Bitrate     int64   `json:"bitrate"`      // Total bitrate in bits/s
	VideoCodec  string  `json:"video_codec"`  // e.g., h264, vp9, hevc
	AudioCodec  string  `json:"audio_codec"`  // e.g., aac, opus, mp3
	FrameRate   float64 `json:"frame_rate"`   // Frames per second
	FileSize    int64   `json:"file_size"`    // File size in bytes
	Container   string  `json:"container"`    // e.g., mp4, webm, mkv
	HasAudio    bool    `json:"has_audio"`    // Whether video has audio track
}

// VideoProcessor defines the interface for video processing operations
type VideoProcessor interface {
	processor.Processor

	// Transcode converts a video to the specified format and settings
	Transcode(ctx context.Context, opts *VideoOptions, input io.Reader) (*processor.Result, error)

	// ExtractThumbnail extracts a frame from the video at the specified position
	ExtractThumbnail(ctx context.Context, input io.Reader, atPercent float64) (*processor.Result, error)

	// GetMetadata extracts metadata from a video file
	GetMetadata(ctx context.Context, input io.Reader) (*VideoMetadata, error)

	// GenerateHLS creates HLS segments and manifest from a video
	GenerateHLS(ctx context.Context, opts *VideoOptions, input io.Reader) (*HLSResult, error)
}

// HLSResult contains the output of HLS generation
type HLSResult struct {
	ManifestPath   string   // Path to the m3u8 manifest
	SegmentPaths   []string // Paths to all segment files
	TotalDuration  float64  // Total duration in seconds
	SegmentCount   int      // Number of segments
	Resolutions    []int    // Available resolutions (heights)
}

// VideoConfig holds configuration for video processors
type VideoConfig struct {
	*processor.Config

	// FFmpeg settings
	FFmpegPath  string // Path to ffmpeg binary (default: "ffmpeg")
	FFprobePath string // Path to ffprobe binary (default: "ffprobe")

	// Default encoding settings
	DefaultPreset     string
	DefaultCRF        int
	DefaultMaxBitrate string

	// Limits
	MaxDuration   int   // Maximum video duration in seconds
	MaxResolution int   // Maximum output resolution (height)
	MaxFileSize   int64 // Maximum input file size

	// HLS settings
	HLSSegmentDuration int      // Default segment duration
	HLSResolutions     []int    // Available resolutions for adaptive streaming
}

// DefaultVideoConfig returns default video configuration
func DefaultVideoConfig() *VideoConfig {
	return &VideoConfig{
		Config:             processor.DefaultConfig(),
		FFmpegPath:         "ffmpeg",
		FFprobePath:        "ffprobe",
		DefaultPreset:      "medium",
		DefaultCRF:         23,
		DefaultMaxBitrate:  "5M",
		MaxDuration:        30 * 60, // 30 minutes
		MaxResolution:      1080,
		MaxFileSize:        500 * 1024 * 1024, // 500 MB
		HLSSegmentDuration: 10,
		HLSResolutions:     []int{360, 480, 720, 1080},
	}
}

// Supported video content types
var SupportedVideoTypes = []string{
	"video/mp4",
	"video/webm",
	"video/quicktime",
	"video/x-msvideo",
	"video/x-matroska",
	"video/mpeg",
	"video/ogg",
	"video/3gpp",
	"video/3gpp2",
}

// IsVideoType checks if the content type is a supported video type
func IsVideoType(contentType string) bool {
	for _, t := range SupportedVideoTypes {
		if t == contentType {
			return true
		}
	}
	return false
}

// GetResolutionPreset returns encoding settings for a given resolution
func GetResolutionPreset(height int) (width int, videoBitrate string, audioBitrate string) {
	switch {
	case height <= 360:
		return 640, "800k", "64k"
	case height <= 480:
		return 854, "1500k", "96k"
	case height <= 720:
		return 1280, "3000k", "128k"
	case height <= 1080:
		return 1920, "5000k", "192k"
	default: // 4K
		return 3840, "15000k", "256k"
	}
}
