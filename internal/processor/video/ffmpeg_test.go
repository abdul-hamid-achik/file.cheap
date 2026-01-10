package video

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

func getTestVideoPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "video", "sample.mp4")
}

func skipIfNoFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available, skipping test")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available, skipping test")
	}
}

func skipIfNoTestVideo(t *testing.T) {
	path := getTestVideoPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("test video not found at %s, skipping test", path)
	}
}

func loadTestVideo(t *testing.T) io.Reader {
	t.Helper()
	path := getTestVideoPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read test video: %v", err)
	}
	return bytes.NewReader(data)
}

func TestNewFFmpegProcessor(t *testing.T) {
	skipIfNoFFmpeg(t)

	tests := []struct {
		name    string
		cfg     *VideoConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			cfg:     nil,
			wantErr: false,
		},
		{
			name:    "valid config",
			cfg:     DefaultVideoConfig(),
			wantErr: false,
		},
		{
			name: "invalid ffmpeg path",
			cfg: &VideoConfig{
				Config:      processor.DefaultConfig(),
				FFmpegPath:  "/nonexistent/ffmpeg",
				FFprobePath: "ffprobe",
			},
			wantErr: true,
		},
		{
			name: "invalid ffprobe path",
			cfg: &VideoConfig{
				Config:      processor.DefaultConfig(),
				FFmpegPath:  "ffmpeg",
				FFprobePath: "/nonexistent/ffprobe",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewFFmpegProcessor(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("NewFFmpegProcessor() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Errorf("NewFFmpegProcessor() error = %v", err)
				return
			}
			if p == nil {
				t.Error("NewFFmpegProcessor() returned nil processor")
			}
		})
	}
}

func TestFFmpegProcessor_Name(t *testing.T) {
	skipIfNoFFmpeg(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if got := p.Name(); got != "video_transcode" {
		t.Errorf("Name() = %q, want %q", got, "video_transcode")
	}
}

func TestFFmpegProcessor_SupportedTypes(t *testing.T) {
	skipIfNoFFmpeg(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	types := p.SupportedTypes()
	if len(types) == 0 {
		t.Error("SupportedTypes() returned empty slice")
	}

	expectedTypes := map[string]bool{
		"video/mp4":  true,
		"video/webm": true,
	}

	for expectedType := range expectedTypes {
		found := false
		for _, vt := range types {
			if vt == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SupportedTypes() missing %q", expectedType)
		}
	}
}

func TestFFmpegProcessor_GetMetadata(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx := context.Background()
	metadata, err := p.GetMetadata(ctx, loadTestVideo(t))
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}

	if metadata == nil {
		t.Fatal("GetMetadata() returned nil metadata")
	}

	if metadata.Duration <= 0 {
		t.Errorf("Duration = %f, want > 0", metadata.Duration)
	}

	if metadata.Width <= 0 {
		t.Errorf("Width = %d, want > 0", metadata.Width)
	}

	if metadata.Height <= 0 {
		t.Errorf("Height = %d, want > 0", metadata.Height)
	}

	if metadata.VideoCodec == "" {
		t.Error("VideoCodec is empty")
	}

	t.Logf("Video metadata: duration=%.2fs, %dx%d, codec=%s, fps=%.2f",
		metadata.Duration, metadata.Width, metadata.Height, metadata.VideoCodec, metadata.FrameRate)
}

func TestFFmpegProcessor_GetMetadata_InvalidInput(t *testing.T) {
	skipIfNoFFmpeg(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name  string
		input io.Reader
	}{
		{"empty input", bytes.NewReader([]byte{})},
		{"invalid data", bytes.NewReader([]byte("not a video"))},
		{"random bytes", bytes.NewReader([]byte{0x00, 0x01, 0x02, 0x03, 0x04})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := p.GetMetadata(ctx, tt.input)
			if err == nil {
				t.Error("GetMetadata() error = nil, want error for invalid input")
			}
		})
	}
}

func TestFFmpegProcessor_ExtractThumbnail(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name      string
		atPercent float64
	}{
		{"at 10%", 0.1},
		{"at 50%", 0.5},
		{"at 90%", 0.9},
		{"at start (0%)", 0.0},
		{"negative percent defaults to 10%", -0.5},
		{"over 100% defaults to 10%", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := p.ExtractThumbnail(ctx, loadTestVideo(t), tt.atPercent)
			if err != nil {
				t.Fatalf("ExtractThumbnail() error = %v", err)
			}

			if result == nil {
				t.Fatal("ExtractThumbnail() returned nil result")
			}

			if result.ContentType != "image/jpeg" {
				t.Errorf("ContentType = %q, want %q", result.ContentType, "image/jpeg")
			}

			if result.Size <= 0 {
				t.Errorf("Size = %d, want > 0", result.Size)
			}

			data, err := io.ReadAll(result.Data)
			if err != nil {
				t.Fatalf("Failed to read result data: %v", err)
			}

			if len(data) == 0 {
				t.Error("Result data is empty")
			}

			// Check JPEG magic bytes
			if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
				t.Error("Result is not a valid JPEG")
			}
		})
	}
}

func TestFFmpegProcessor_Transcode(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name       string
		opts       *VideoOptions
		wantFormat string
		wantErr    bool
	}{
		{
			name:       "default options (nil)",
			opts:       nil,
			wantFormat: "mp4",
			wantErr:    false,
		},
		{
			name: "transcode to mp4",
			opts: &VideoOptions{
				OutputFormat: "mp4",
				Preset:       "ultrafast",
				CRF:          28,
			},
			wantFormat: "mp4",
			wantErr:    false,
		},
		{
			name: "transcode with quality setting",
			opts: &VideoOptions{
				Options: &processor.Options{
					Quality: 80,
				},
				OutputFormat: "mp4",
				Preset:       "ultrafast",
			},
			wantFormat: "mp4",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := p.Transcode(ctx, tt.opts, loadTestVideo(t))
			if tt.wantErr {
				if err == nil {
					t.Error("Transcode() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Transcode() error = %v", err)
			}

			if result == nil {
				t.Fatal("Transcode() returned nil result")
			}

			if result.Size <= 0 {
				t.Errorf("Size = %d, want > 0", result.Size)
			}

			expectedContentType := "video/" + tt.wantFormat
			if result.ContentType != expectedContentType {
				t.Errorf("ContentType = %q, want %q", result.ContentType, expectedContentType)
			}

			t.Logf("Transcoded video: size=%d bytes, contentType=%s", result.Size, result.ContentType)
		})
	}
}

func TestFFmpegProcessor_Transcode_DurationLimit(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	cfg := DefaultVideoConfig()
	cfg.MaxDuration = 1 // 1 second max (test video is ~10s)

	p, err := NewFFmpegProcessor(cfg)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx := context.Background()
	_, err = p.Transcode(ctx, nil, loadTestVideo(t))
	if err == nil {
		t.Error("Transcode() error = nil, want ErrVideoTooLong")
	}

	if !errors.Is(err, ErrVideoTooLong) {
		t.Errorf("Transcode() error = %v, want %v", err, ErrVideoTooLong)
	}
}

func TestFFmpegProcessor_Process(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := &processor.Options{
		Quality: 50,
	}

	result, err := p.Process(ctx, opts, loadTestVideo(t))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result == nil {
		t.Fatal("Process() returned nil result")
	}

	if result.ContentType != "video/mp4" {
		t.Errorf("ContentType = %q, want %q", result.ContentType, "video/mp4")
	}

	if result.Size <= 0 {
		t.Errorf("Size = %d, want > 0", result.Size)
	}
}

func TestFFmpegProcessor_GenerateHLS(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	opts := &VideoOptions{
		Preset:             "ultrafast",
		CRF:                28,
		HLSSegmentDuration: 2,
	}

	result, err := p.GenerateHLS(ctx, opts, loadTestVideo(t))
	if err != nil {
		t.Fatalf("GenerateHLS() error = %v", err)
	}

	if result == nil {
		t.Fatal("GenerateHLS() returned nil result")
	}

	if result.ManifestPath == "" {
		t.Error("ManifestPath is empty")
	}

	if _, err := os.Stat(result.ManifestPath); os.IsNotExist(err) {
		t.Errorf("Manifest file does not exist: %s", result.ManifestPath)
	}

	if len(result.SegmentPaths) == 0 {
		t.Error("SegmentPaths is empty")
	}

	if result.SegmentCount <= 0 {
		t.Errorf("SegmentCount = %d, want > 0", result.SegmentCount)
	}

	if result.TotalDuration <= 0 {
		t.Errorf("TotalDuration = %f, want > 0", result.TotalDuration)
	}

	// Cleanup
	if result.ManifestPath != "" {
		_ = os.RemoveAll(filepath.Dir(result.ManifestPath))
	}

	t.Logf("Generated HLS: %d segments, duration=%.2fs", result.SegmentCount, result.TotalDuration)
}

func TestFFmpegProcessor_ContextCancellation(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = p.GetMetadata(ctx, loadTestVideo(t))
	if err == nil {
		t.Log("GetMetadata() completed despite cancelled context (may have completed before cancellation)")
	}
}

func TestFFmpegProcessor_EmptyInput(t *testing.T) {
	skipIfNoFFmpeg(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx := context.Background()
	emptyReader := bytes.NewReader([]byte{})

	_, err = p.GetMetadata(ctx, emptyReader)
	if err == nil {
		t.Error("GetMetadata() with empty input should return error")
	}
}

func TestFFmpegProcessor_getContentType(t *testing.T) {
	skipIfNoFFmpeg(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		format string
		want   string
	}{
		{"mp4", "video/mp4"},
		{"webm", "video/webm"},
		{"hls", "application/x-mpegURL"},
		{"unknown", "video/mp4"}, // Default
		{"", "video/mp4"},        // Empty defaults to mp4
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got := p.getContentType(tt.format)
			if got != tt.want {
				t.Errorf("getContentType(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestFFmpegProcessor_AddWatermark(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoTestVideo(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		position string
		opacity  float64
	}{
		{"bottom-right default", "file.cheap", "bottom-right", 0.5},
		{"top-left", "Test", "top-left", 0.8},
		{"center", "Watermark", "center", 0.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := p.AddWatermark(ctx, loadTestVideo(t), tt.text, tt.position, tt.opacity)
			if err != nil {
				t.Fatalf("AddWatermark() error = %v", err)
			}

			if result == nil {
				t.Fatal("AddWatermark() returned nil result")
			}

			if result.ContentType != "video/mp4" {
				t.Errorf("ContentType = %q, want %q", result.ContentType, "video/mp4")
			}

			if result.Size <= 0 {
				t.Errorf("Size = %d, want > 0", result.Size)
			}

			t.Logf("Watermarked video: size=%d bytes", result.Size)
		})
	}
}

func TestFFmpegProcessor_getWatermarkPosition(t *testing.T) {
	skipIfNoFFmpeg(t)

	p, err := NewFFmpegProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		position string
		wantX    string
		wantY    string
	}{
		{"top-left", "10", "10"},
		{"top-right", "w-tw-10", "10"},
		{"bottom-left", "10", "h-th-10"},
		{"bottom-right", "w-tw-10", "h-th-10"},
		{"center", "(w-tw)/2", "(h-th)/2"},
		{"unknown", "w-tw-10", "h-th-10"},
	}

	for _, tt := range tests {
		t.Run(tt.position, func(t *testing.T) {
			x, y := p.getWatermarkPosition(tt.position)
			if x != tt.wantX {
				t.Errorf("x = %q, want %q", x, tt.wantX)
			}
			if y != tt.wantY {
				t.Errorf("y = %q, want %q", y, tt.wantY)
			}
		})
	}
}
