package worker

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewThumbnailPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	payload := NewThumbnailPayload(fileID)

	if payload.FileID != fileID {
		t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
	}
	if payload.Width != 300 {
		t.Errorf("Width = %d, want 300", payload.Width)
	}
	if payload.Height != 300 {
		t.Errorf("Height = %d, want 300", payload.Height)
	}
	if payload.Quality != 85 {
		t.Errorf("Quality = %d, want 85", payload.Quality)
	}
	if payload.Position != "center" {
		t.Errorf("Position = %s, want center", payload.Position)
	}
}

func TestNewThumbnailPayloadWithPosition(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name         string
		position     string
		wantPosition string
	}{
		{"custom position", "north", "north"},
		{"empty position uses default", "", "center"},
		{"south position", "south", "south"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewThumbnailPayloadWithPosition(fileID, tt.position)
			if payload.Position != tt.wantPosition {
				t.Errorf("Position = %s, want %s", payload.Position, tt.wantPosition)
			}
		})
	}
}

func TestNewResizePayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		variantType string
		width       int
		height      int
		wantQuality int
	}{
		{"custom dimensions", "custom", 800, 600, 85},
		{"preset lg", "lg", 1200, 800, 85},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewResizePayload(fileID, tt.variantType, tt.width, tt.height)
			if payload.FileID != fileID {
				t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
			}
			if payload.Width != tt.width {
				t.Errorf("Width = %d, want %d", payload.Width, tt.width)
			}
			if payload.Height != tt.height {
				t.Errorf("Height = %d, want %d", payload.Height, tt.height)
			}
			if payload.VariantType != tt.variantType {
				t.Errorf("VariantType = %s, want %s", payload.VariantType, tt.variantType)
			}
		})
	}
}

func TestNewResponsivePayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		variantType string
		wantWidth   int
		wantHeight  int
	}{
		{"sm variant", "sm", 640, 0},
		{"md variant", "md", 1024, 0},
		{"lg variant", "lg", 1920, 0},
		{"xl variant", "xl", 2560, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewResponsivePayload(fileID, tt.variantType)
			if payload.FileID != fileID {
				t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
			}
			if payload.Width != tt.wantWidth {
				t.Errorf("Width = %d, want %d", payload.Width, tt.wantWidth)
			}
			if payload.Height != tt.wantHeight {
				t.Errorf("Height = %d, want %d", payload.Height, tt.wantHeight)
			}
			if payload.VariantType != tt.variantType {
				t.Errorf("VariantType = %s, want %s", payload.VariantType, tt.variantType)
			}
		})
	}
}

func TestNewSocialPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		variantType string
		wantWidth   int
		wantHeight  int
	}{
		{"og variant", "og", 1200, 630},
		{"twitter variant", "twitter", 1200, 675},
		{"instagram_square", "instagram_square", 1080, 1080},
		{"instagram_portrait", "instagram_portrait", 1080, 1350},
		{"instagram_story", "instagram_story", 1080, 1920},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewSocialPayload(fileID, tt.variantType)
			if payload.FileID != fileID {
				t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
			}
			if payload.Width != tt.wantWidth {
				t.Errorf("Width = %d, want %d", payload.Width, tt.wantWidth)
			}
			if payload.Height != tt.wantHeight {
				t.Errorf("Height = %d, want %d", payload.Height, tt.wantHeight)
			}
		})
	}
}

func TestNewWebPPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		quality     int
		wantQuality int
	}{
		{"default quality", 0, 85},
		{"negative quality", -10, 85},
		{"custom quality", 75, 75},
		{"max quality", 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewWebPPayload(fileID, tt.quality)
			if payload.FileID != fileID {
				t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
			}
			if payload.Quality != tt.wantQuality {
				t.Errorf("Quality = %d, want %d", payload.Quality, tt.wantQuality)
			}
		})
	}
}

func TestNewWatermarkPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		text        string
		position    string
		opacity     float64
		isPremium   bool
		wantText    string
		wantOpacity float64
	}{
		// Non-premium with text appends " | file.cheap"
		{"basic watermark", "test", "bottom-right", 0.5, false, "test | file.cheap", 0.5},
		// Premium keeps original text
		{"premium watermark", "custom", "center", 0.8, true, "custom", 0.8},
		// Non-premium with empty text uses default "file.cheap"
		{"empty text uses default", "", "top-left", 0.3, false, "file.cheap", 0.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewWatermarkPayload(fileID, tt.text, tt.position, tt.opacity, tt.isPremium)
			if payload.FileID != fileID {
				t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
			}
			if payload.Text != tt.wantText {
				t.Errorf("Text = %s, want %s", payload.Text, tt.wantText)
			}
			if payload.Position != tt.position {
				t.Errorf("Position = %s, want %s", payload.Position, tt.position)
			}
			if payload.Opacity != tt.wantOpacity {
				t.Errorf("Opacity = %f, want %f", payload.Opacity, tt.wantOpacity)
			}
			if payload.IsPremium != tt.isPremium {
				t.Errorf("IsPremium = %v, want %v", payload.IsPremium, tt.isPremium)
			}
		})
	}
}

func TestNewOptimizePayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		quality     int
		wantQuality int
	}{
		{"default quality", 0, 85},
		{"custom quality", 70, 70},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewOptimizePayload(fileID, tt.quality)
			if payload.FileID != fileID {
				t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
			}
			if payload.Quality != tt.wantQuality {
				t.Errorf("Quality = %d, want %d", payload.Quality, tt.wantQuality)
			}
		})
	}
}

func TestNewConvertPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		format      string
		quality     int
		wantQuality int
	}{
		{"convert to png", "png", 0, 85},
		{"convert to jpg with quality", "jpg", 90, 90},
		{"convert to webp", "webp", 75, 75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := NewConvertPayload(fileID, tt.format, tt.quality)
			if payload.FileID != fileID {
				t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
			}
			if payload.Format != tt.format {
				t.Errorf("Format = %s, want %s", payload.Format, tt.format)
			}
			if payload.Quality != tt.wantQuality {
				t.Errorf("Quality = %d, want %d", payload.Quality, tt.wantQuality)
			}
		})
	}
}

func TestNewPDFThumbnailPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	payload := NewPDFThumbnailPayload(fileID)

	if payload.FileID != fileID {
		t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
	}
	if payload.Page != 1 {
		t.Errorf("Page = %d, want 1", payload.Page)
	}
	if payload.Width != 300 {
		t.Errorf("Width = %d, want 300", payload.Width)
	}
}

func TestNewVideoThumbnailPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	payload := NewVideoThumbnailPayload(fileID)

	if payload.FileID != fileID {
		t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
	}
	if payload.Width != 320 {
		t.Errorf("Width = %d, want 320", payload.Width)
	}
	if payload.Height != 180 {
		t.Errorf("Height = %d, want 180", payload.Height)
	}
	if payload.Quality != 85 {
		t.Errorf("Quality = %d, want 85", payload.Quality)
	}
	if payload.Format != "jpeg" {
		t.Errorf("Format = %s, want jpeg", payload.Format)
	}
	if payload.AtPercent != 0.1 {
		t.Errorf("AtPercent = %f, want 0.1", payload.AtPercent)
	}
}

func TestNewVideoTranscodePayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	payload := NewVideoTranscodePayload(fileID, "720p", 720)

	if payload.FileID != fileID {
		t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
	}
	if payload.VariantType != "720p" {
		t.Errorf("VariantType = %s, want 720p", payload.VariantType)
	}
	if payload.MaxResolution != 720 {
		t.Errorf("MaxResolution = %d, want 720", payload.MaxResolution)
	}
	if payload.OutputFormat != "mp4" {
		t.Errorf("OutputFormat = %s, want mp4", payload.OutputFormat)
	}
	if payload.Preset != "medium" {
		t.Errorf("Preset = %s, want medium", payload.Preset)
	}
	if payload.CRF != 23 {
		t.Errorf("CRF = %d, want 23", payload.CRF)
	}
}

func TestNewVideoHLSPayload(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	resolutions := []int{360, 720, 1080}
	payload := NewVideoHLSPayload(fileID, resolutions)

	if payload.FileID != fileID {
		t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
	}
	if payload.SegmentDuration != 10 {
		t.Errorf("SegmentDuration = %d, want 10", payload.SegmentDuration)
	}
	if len(payload.Resolutions) != len(resolutions) {
		t.Errorf("Resolutions length = %d, want %d", len(payload.Resolutions), len(resolutions))
	}
}

func TestVideoWatermarkPayloadFields(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	payload := VideoWatermarkPayload{
		FileID:   fileID,
		Text:     "watermark text",
		Position: "center",
		Opacity:  0.7,
	}

	if payload.FileID != fileID {
		t.Errorf("FileID = %v, want %v", payload.FileID, fileID)
	}
	if payload.Text != "watermark text" {
		t.Errorf("Text = %s, want watermark text", payload.Text)
	}
	if payload.Position != "center" {
		t.Errorf("Position = %s, want center", payload.Position)
	}
	if payload.Opacity != 0.7 {
		t.Errorf("Opacity = %f, want 0.7", payload.Opacity)
	}
}

func TestNewZipDownloadPayload(t *testing.T) {
	zipDownloadID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	userID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")
	fileIDs := []uuid.UUID{
		uuid.MustParse("770e8400-e29b-41d4-a716-446655440001"),
		uuid.MustParse("770e8400-e29b-41d4-a716-446655440002"),
		uuid.MustParse("770e8400-e29b-41d4-a716-446655440003"),
	}

	payload := NewZipDownloadPayload(zipDownloadID, userID, fileIDs)

	if payload.ZipDownloadID != zipDownloadID {
		t.Errorf("ZipDownloadID = %v, want %v", payload.ZipDownloadID, zipDownloadID)
	}
	if payload.UserID != userID {
		t.Errorf("UserID = %v, want %v", payload.UserID, userID)
	}
	if len(payload.FileIDs) != len(fileIDs) {
		t.Errorf("FileIDs length = %d, want %d", len(payload.FileIDs), len(fileIDs))
	}
}

func TestGetFileID(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name    string
		payload JobPayload
	}{
		{"thumbnail", &ThumbnailPayload{FileID: fileID}},
		{"resize", &ResizePayload{FileID: fileID}},
		{"webp", &WebPPayload{FileID: fileID}},
		{"watermark", &WatermarkPayload{FileID: fileID}},
		{"optimize", &OptimizePayload{FileID: fileID}},
		{"convert", &ConvertPayload{FileID: fileID}},
		{"pdf_thumbnail", &PDFThumbnailPayload{FileID: fileID}},
		{"video_thumbnail", &VideoThumbnailPayload{FileID: fileID}},
		{"video_transcode", &VideoTranscodePayload{FileID: fileID}},
		{"video_hls", &VideoHLSPayload{FileID: fileID}},
		{"video_watermark", &VideoWatermarkPayload{FileID: fileID}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.payload.GetFileID()
			if got.Bytes != fileID {
				t.Errorf("GetFileID().Bytes = %v, want %v", got.Bytes, fileID)
			}
			if !got.Valid {
				t.Error("GetFileID().Valid = false, want true")
			}
		})
	}
}
