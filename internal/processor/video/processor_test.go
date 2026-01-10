package video

import (
	"testing"
)

func TestIsVideoType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"mp4 is video", "video/mp4", true},
		{"webm is video", "video/webm", true},
		{"quicktime is video", "video/quicktime", true},
		{"avi is video", "video/x-msvideo", true},
		{"mkv is video", "video/x-matroska", true},
		{"mpeg is video", "video/mpeg", true},
		{"ogg is video", "video/ogg", true},
		{"3gpp is video", "video/3gpp", true},
		{"3gpp2 is video", "video/3gpp2", true},
		{"image/jpeg is not video", "image/jpeg", false},
		{"application/pdf is not video", "application/pdf", false},
		{"text/plain is not video", "text/plain", false},
		{"empty string is not video", "", false},
		{"audio/mp3 is not video", "audio/mp3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsVideoType(tt.contentType)
			if got != tt.want {
				t.Errorf("IsVideoType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestGetResolutionPreset(t *testing.T) {
	tests := []struct {
		name          string
		height        int
		wantWidth     int
		wantVideoBR   string
		wantAudioBR   string
	}{
		{"360p", 360, 640, "800k", "64k"},
		{"480p", 480, 854, "1500k", "96k"},
		{"720p", 720, 1280, "3000k", "128k"},
		{"1080p", 1080, 1920, "5000k", "192k"},
		{"4K (2160p)", 2160, 3840, "15000k", "256k"},
		{"below 360p uses 360p preset", 240, 640, "800k", "64k"},
		{"between 360p and 480p uses 480p", 400, 854, "1500k", "96k"},
		{"between 720p and 1080p uses 1080p", 900, 1920, "5000k", "192k"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, videoBR, audioBR := GetResolutionPreset(tt.height)
			if width != tt.wantWidth {
				t.Errorf("GetResolutionPreset(%d) width = %d, want %d", tt.height, width, tt.wantWidth)
			}
			if videoBR != tt.wantVideoBR {
				t.Errorf("GetResolutionPreset(%d) videoBitrate = %s, want %s", tt.height, videoBR, tt.wantVideoBR)
			}
			if audioBR != tt.wantAudioBR {
				t.Errorf("GetResolutionPreset(%d) audioBitrate = %s, want %s", tt.height, audioBR, tt.wantAudioBR)
			}
		})
	}
}

func TestDefaultVideoConfig(t *testing.T) {
	cfg := DefaultVideoConfig()

	if cfg == nil {
		t.Fatal("DefaultVideoConfig() returned nil")
	}

	if cfg.FFmpegPath != "ffmpeg" {
		t.Errorf("FFmpegPath = %q, want %q", cfg.FFmpegPath, "ffmpeg")
	}

	if cfg.FFprobePath != "ffprobe" {
		t.Errorf("FFprobePath = %q, want %q", cfg.FFprobePath, "ffprobe")
	}

	if cfg.DefaultPreset != "medium" {
		t.Errorf("DefaultPreset = %q, want %q", cfg.DefaultPreset, "medium")
	}

	if cfg.DefaultCRF != 23 {
		t.Errorf("DefaultCRF = %d, want %d", cfg.DefaultCRF, 23)
	}

	if cfg.MaxDuration != 30*60 {
		t.Errorf("MaxDuration = %d, want %d", cfg.MaxDuration, 30*60)
	}

	if cfg.MaxResolution != 1080 {
		t.Errorf("MaxResolution = %d, want %d", cfg.MaxResolution, 1080)
	}

	if cfg.MaxFileSize != 500*1024*1024 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 500*1024*1024)
	}

	if cfg.HLSSegmentDuration != 10 {
		t.Errorf("HLSSegmentDuration = %d, want %d", cfg.HLSSegmentDuration, 10)
	}

	expectedResolutions := []int{360, 480, 720, 1080}
	if len(cfg.HLSResolutions) != len(expectedResolutions) {
		t.Errorf("HLSResolutions length = %d, want %d", len(cfg.HLSResolutions), len(expectedResolutions))
	}
	for i, r := range expectedResolutions {
		if i < len(cfg.HLSResolutions) && cfg.HLSResolutions[i] != r {
			t.Errorf("HLSResolutions[%d] = %d, want %d", i, cfg.HLSResolutions[i], r)
		}
	}
}

func TestSupportedVideoTypes(t *testing.T) {
	expectedTypes := map[string]bool{
		"video/mp4":         true,
		"video/webm":        true,
		"video/quicktime":   true,
		"video/x-msvideo":   true,
		"video/x-matroska":  true,
		"video/mpeg":        true,
		"video/ogg":         true,
		"video/3gpp":        true,
		"video/3gpp2":       true,
	}

	if len(SupportedVideoTypes) != len(expectedTypes) {
		t.Errorf("SupportedVideoTypes has %d types, want %d", len(SupportedVideoTypes), len(expectedTypes))
	}

	for _, vt := range SupportedVideoTypes {
		if !expectedTypes[vt] {
			t.Errorf("Unexpected video type in SupportedVideoTypes: %q", vt)
		}
	}
}

func TestVideoMetadata(t *testing.T) {
	metadata := VideoMetadata{
		Duration:   120.5,
		Width:      1920,
		Height:     1080,
		Bitrate:    5000000,
		VideoCodec: "h264",
		AudioCodec: "aac",
		FrameRate:  30.0,
		FileSize:   75000000,
		Container:  "mp4",
		HasAudio:   true,
	}

	if metadata.Duration != 120.5 {
		t.Errorf("Duration = %f, want %f", metadata.Duration, 120.5)
	}
	if metadata.Width != 1920 {
		t.Errorf("Width = %d, want %d", metadata.Width, 1920)
	}
	if metadata.Height != 1080 {
		t.Errorf("Height = %d, want %d", metadata.Height, 1080)
	}
	if !metadata.HasAudio {
		t.Error("HasAudio = false, want true")
	}
}

func TestVideoOptions(t *testing.T) {
	opts := VideoOptions{
		Preset:       "fast",
		CRF:          20,
		VideoBitrate: "5M",
		AudioBitrate: "192k",
		OutputFormat: "mp4",
		MaxResolution: 1080,
		ThumbnailAt:  0.5,
		HLSSegmentDuration: 6,
	}

	if opts.Preset != "fast" {
		t.Errorf("Preset = %q, want %q", opts.Preset, "fast")
	}
	if opts.CRF != 20 {
		t.Errorf("CRF = %d, want %d", opts.CRF, 20)
	}
	if opts.OutputFormat != "mp4" {
		t.Errorf("OutputFormat = %q, want %q", opts.OutputFormat, "mp4")
	}
	if opts.ThumbnailAt != 0.5 {
		t.Errorf("ThumbnailAt = %f, want %f", opts.ThumbnailAt, 0.5)
	}
}

func TestHLSResult(t *testing.T) {
	result := HLSResult{
		ManifestPath:  "/tmp/output/playlist.m3u8",
		SegmentPaths:  []string{"/tmp/output/segment_000.ts", "/tmp/output/segment_001.ts"},
		TotalDuration: 120.0,
		SegmentCount:  12,
		Resolutions:   []int{720, 1080},
	}

	if result.ManifestPath != "/tmp/output/playlist.m3u8" {
		t.Errorf("ManifestPath = %q, want %q", result.ManifestPath, "/tmp/output/playlist.m3u8")
	}
	if len(result.SegmentPaths) != 2 {
		t.Errorf("SegmentPaths length = %d, want %d", len(result.SegmentPaths), 2)
	}
	if result.SegmentCount != 12 {
		t.Errorf("SegmentCount = %d, want %d", result.SegmentCount, 12)
	}
}
