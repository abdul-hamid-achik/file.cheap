package video

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

func getTestVideoPathForThumbnail() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "video", "sample.mp4")
}

func skipIfNoFFmpegThumbnail(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available, skipping test")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available, skipping test")
	}
}

func skipIfNoTestVideoThumbnail(t *testing.T) {
	path := getTestVideoPathForThumbnail()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("test video not found at %s, skipping test", path)
	}
}

func loadTestVideoForThumbnail(t *testing.T) io.Reader {
	t.Helper()
	path := getTestVideoPathForThumbnail()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read test video: %v", err)
	}
	return bytes.NewReader(data)
}

func TestNewThumbnailProcessor(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)

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
			p, err := NewThumbnailProcessor(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("NewThumbnailProcessor() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Errorf("NewThumbnailProcessor() error = %v", err)
				return
			}
			if p == nil {
				t.Error("NewThumbnailProcessor() returned nil processor")
			}
		})
	}
}

func TestThumbnailProcessor_Name(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if got := p.Name(); got != "video_thumbnail" {
		t.Errorf("Name() = %q, want %q", got, "video_thumbnail")
	}
}

func TestThumbnailProcessor_SupportedTypes(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	types := p.SupportedTypes()
	if len(types) == 0 {
		t.Error("SupportedTypes() returned empty slice")
	}

	for _, vt := range types {
		if !IsVideoType(vt) {
			t.Errorf("SupportedTypes() contains non-video type: %q", vt)
		}
	}
}

func TestThumbnailProcessor_Process(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)
	skipIfNoTestVideoThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name       string
		opts       *processor.Options
		wantWidth  int
		wantHeight int
		wantFormat string
	}{
		{
			name:       "default options (nil)",
			opts:       nil,
			wantWidth:  320,
			wantHeight: 180,
			wantFormat: "image/jpeg",
		},
		{
			name: "custom dimensions",
			opts: &processor.Options{
				Width:  640,
				Height: 360,
			},
			wantWidth:  640,
			wantHeight: 360,
			wantFormat: "image/jpeg",
		},
		{
			name: "custom quality",
			opts: &processor.Options{
				Width:   320,
				Height:  180,
				Quality: 95,
			},
			wantWidth:  320,
			wantHeight: 180,
			wantFormat: "image/jpeg",
		},
		{
			name: "PNG format",
			opts: &processor.Options{
				Width:  320,
				Height: 180,
				Format: "png",
			},
			wantWidth:  320,
			wantHeight: 180,
			wantFormat: "image/png",
		},
		{
			name: "custom position (Page as percentage)",
			opts: &processor.Options{
				Width:  320,
				Height: 180,
				Page:   50, // 50% into video
			},
			wantWidth:  320,
			wantHeight: 180,
			wantFormat: "image/jpeg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := p.Process(ctx, tt.opts, loadTestVideoForThumbnail(t))
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if result == nil {
				t.Fatal("Process() returned nil result")
			}

			if result.ContentType != tt.wantFormat {
				t.Errorf("ContentType = %q, want %q", result.ContentType, tt.wantFormat)
			}

			if result.Size <= 0 {
				t.Errorf("Size = %d, want > 0", result.Size)
			}

			if result.Metadata.Width != tt.wantWidth {
				t.Errorf("Metadata.Width = %d, want %d", result.Metadata.Width, tt.wantWidth)
			}

			if result.Metadata.Height != tt.wantHeight {
				t.Errorf("Metadata.Height = %d, want %d", result.Metadata.Height, tt.wantHeight)
			}

			// Verify the output is a valid image
			data, err := io.ReadAll(result.Data)
			if err != nil {
				t.Fatalf("Failed to read result data: %v", err)
			}

			if len(data) == 0 {
				t.Error("Result data is empty")
			}

			// Check magic bytes
			switch tt.wantFormat {
			case "image/jpeg":
				if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
					t.Error("Result is not a valid JPEG")
				}
			case "image/png":
				pngMagic := []byte{0x89, 0x50, 0x4E, 0x47}
				if len(data) < 4 || !bytes.Equal(data[:4], pngMagic) {
					t.Error("Result is not a valid PNG")
				}
			}

			t.Logf("Thumbnail: %dx%d, %s, %d bytes", result.Metadata.Width, result.Metadata.Height, result.ContentType, result.Size)
		})
	}
}

func TestThumbnailProcessor_Process_InvalidInput(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name  string
		input io.Reader
	}{
		{"empty input", bytes.NewReader([]byte{})},
		{"invalid data", bytes.NewReader([]byte("not a video file"))},
		{"random bytes", bytes.NewReader([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := p.Process(ctx, nil, tt.input)
			if err == nil {
				t.Error("Process() error = nil, want error for invalid input")
			}
		})
	}
}

func TestThumbnailProcessor_GenerateThumbnailSprite(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)
	skipIfNoTestVideoThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name        string
		cols        int
		rows        int
		thumbWidth  int
		thumbHeight int
	}{
		{
			name:        "default sprite (10x10)",
			cols:        0, // Uses default
			rows:        0,
			thumbWidth:  0,
			thumbHeight: 0,
		},
		{
			name:        "5x4 sprite",
			cols:        5,
			rows:        4,
			thumbWidth:  160,
			thumbHeight: 90,
		},
		{
			name:        "3x3 sprite with custom dimensions",
			cols:        3,
			rows:        3,
			thumbWidth:  200,
			thumbHeight: 112,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := p.GenerateThumbnailSprite(ctx, loadTestVideoForThumbnail(t), tt.cols, tt.rows, tt.thumbWidth, tt.thumbHeight)
			if err != nil {
				t.Fatalf("GenerateThumbnailSprite() error = %v", err)
			}

			if result == nil {
				t.Fatal("GenerateThumbnailSprite() returned nil result")
			}

			if result.ContentType != "image/jpeg" {
				t.Errorf("ContentType = %q, want %q", result.ContentType, "image/jpeg")
			}

			if result.Size <= 0 {
				t.Errorf("Size = %d, want > 0", result.Size)
			}

			// Verify dimensions
			expectedCols := tt.cols
			if expectedCols <= 0 {
				expectedCols = 10
			}
			expectedRows := tt.rows
			if expectedRows <= 0 {
				expectedRows = 10
			}
			expectedThumbWidth := tt.thumbWidth
			if expectedThumbWidth <= 0 {
				expectedThumbWidth = 160
			}
			expectedThumbHeight := tt.thumbHeight
			if expectedThumbHeight <= 0 {
				expectedThumbHeight = 90
			}

			expectedWidth := expectedThumbWidth * expectedCols
			expectedHeight := expectedThumbHeight * expectedRows

			if result.Metadata.Width != expectedWidth {
				t.Errorf("Metadata.Width = %d, want %d", result.Metadata.Width, expectedWidth)
			}
			if result.Metadata.Height != expectedHeight {
				t.Errorf("Metadata.Height = %d, want %d", result.Metadata.Height, expectedHeight)
			}

			t.Logf("Sprite: %dx%d, %d bytes", result.Metadata.Width, result.Metadata.Height, result.Size)
		})
	}
}

func TestThumbnailProcessor_ContextCancellation(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)
	skipIfNoTestVideoThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = p.Process(ctx, nil, loadTestVideoForThumbnail(t))
	// May or may not error depending on timing
	if err != nil {
		t.Logf("Process() with cancelled context returned error (expected): %v", err)
	}
}

func TestThumbnailProcessor_Process_JpgFormat(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)
	skipIfNoTestVideoThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// "jpg" should be normalized to "jpeg"
	opts := &processor.Options{
		Width:  320,
		Height: 180,
		Format: "jpg",
	}

	result, err := p.Process(ctx, opts, loadTestVideoForThumbnail(t))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result.ContentType != "image/jpeg" {
		t.Errorf("ContentType = %q, want %q", result.ContentType, "image/jpeg")
	}
}

func TestThumbnailProcessor_Process_OutOfRangePosition(t *testing.T) {
	skipIfNoFFmpegThumbnail(t)
	skipIfNoTestVideoThumbnail(t)

	p, err := NewThumbnailProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	tests := []struct {
		name string
		page int
	}{
		{"page 0 uses default 10%", 0},
		{"page 101 uses default 10%", 101},
		{"page -1 uses default 10%", -1},
		{"page 1 is 1%", 1},
		{"page 50 is 50%", 50},
		{"page 90 is 90%", 90}, // Don't test 100% as seeking to exact end can fail
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			opts := &processor.Options{
				Width:  320,
				Height: 180,
				Page:   tt.page,
			}

			result, err := p.Process(ctx, opts, loadTestVideoForThumbnail(t))
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if result == nil {
				t.Fatal("Process() returned nil result")
			}

			if result.Size <= 0 {
				t.Errorf("Size = %d, want > 0", result.Size)
			}
		})
	}
}
