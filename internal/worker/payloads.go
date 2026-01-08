package worker

import (
	"github.com/abdul-hamid-achik/file-processor/internal/presets"
	"github.com/google/uuid"
)

type ThumbnailPayload struct {
	FileID  uuid.UUID `json:"file_id"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	Quality int       `json:"quality"`
}

type ResizePayload struct {
	FileID      uuid.UUID `json:"file_id"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Quality     int       `json:"quality"`
	VariantType string    `json:"variant_type"`
}

func NewThumbnailPayload(fileID uuid.UUID) ThumbnailPayload {
	return ThumbnailPayload{
		FileID:  fileID,
		Width:   presets.Thumbnail.Width,
		Height:  presets.Thumbnail.Height,
		Quality: presets.Thumbnail.Quality,
	}
}

func NewResizePayload(fileID uuid.UUID, variantType string, width, height int) ResizePayload {
	quality := 85
	if p, ok := presets.Get(variantType); ok {
		quality = p.Quality
	}
	return ResizePayload{
		FileID:      fileID,
		Width:       width,
		Height:      height,
		Quality:     quality,
		VariantType: variantType,
	}
}

func NewResponsivePayload(fileID uuid.UUID, variantType string) ResizePayload {
	p := presets.Responsive[variantType]
	return ResizePayload{
		FileID:      fileID,
		Width:       p.Width,
		Height:      p.Height,
		Quality:     p.Quality,
		VariantType: variantType,
	}
}

func NewSocialPayload(fileID uuid.UUID, variantType string) ResizePayload {
	p := presets.Social[variantType]
	return ResizePayload{
		FileID:      fileID,
		Width:       p.Width,
		Height:      p.Height,
		Quality:     p.Quality,
		VariantType: variantType,
	}
}

type WebPPayload struct {
	FileID  uuid.UUID `json:"file_id"`
	Quality int       `json:"quality"`
}

func NewWebPPayload(fileID uuid.UUID, quality int) WebPPayload {
	if quality <= 0 {
		quality = 85
	}
	return WebPPayload{
		FileID:  fileID,
		Quality: quality,
	}
}

type WatermarkPayload struct {
	FileID    uuid.UUID `json:"file_id"`
	Text      string    `json:"text"`
	Position  string    `json:"position"`
	Opacity   float64   `json:"opacity"`
	FontSize  int       `json:"font_size"`
	Color     string    `json:"color"`
	IsPremium bool      `json:"is_premium"`
}

func NewWatermarkPayload(fileID uuid.UUID, text, position string, opacity float64, isPremium bool) WatermarkPayload {
	if position == "" {
		position = "bottom-right"
	}
	if opacity <= 0 {
		opacity = 0.5
	}
	finalText := text
	if !isPremium && text == "" {
		finalText = "file.cheap"
	} else if !isPremium {
		finalText = text + " | file.cheap"
	}
	return WatermarkPayload{
		FileID:    fileID,
		Text:      finalText,
		Position:  position,
		Opacity:   opacity,
		FontSize:  24,
		Color:     "#FFFFFF",
		IsPremium: isPremium,
	}
}

type ConvertPayload struct {
	FileID  uuid.UUID `json:"file_id"`
	Format  string    `json:"format"`
	Quality int       `json:"quality"`
}

func NewConvertPayload(fileID uuid.UUID, format string, quality int) ConvertPayload {
	if quality <= 0 {
		quality = 85
	}
	return ConvertPayload{
		FileID:  fileID,
		Format:  format,
		Quality: quality,
	}
}

type PDFThumbnailPayload struct {
	FileID  uuid.UUID `json:"file_id"`
	Page    int       `json:"page"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	Quality int       `json:"quality"`
	Format  string    `json:"format"`
}

func NewPDFThumbnailPayload(fileID uuid.UUID) PDFThumbnailPayload {
	return PDFThumbnailPayload{
		FileID:  fileID,
		Page:    1,
		Width:   presets.PDFThumbnail.Width,
		Height:  presets.PDFThumbnail.Height,
		Quality: presets.PDFThumbnail.Quality,
		Format:  "png",
	}
}

func NewPDFThumbnailPayloadWithOptions(fileID uuid.UUID, page int, format string, width, height int) PDFThumbnailPayload {
	p := NewPDFThumbnailPayload(fileID)
	if page > 0 {
		p.Page = page
	}
	if format == "jpeg" || format == "jpg" {
		p.Format = "jpeg"
	}
	if width > 0 {
		p.Width = width
	}
	if height > 0 {
		p.Height = height
	}
	return p
}

type MetadataPayload struct {
	FileID uuid.UUID `json:"file_id"`
}

func NewMetadataPayload(fileID uuid.UUID) MetadataPayload {
	return MetadataPayload{FileID: fileID}
}

type OptimizePayload struct {
	FileID  uuid.UUID `json:"file_id"`
	Quality int       `json:"quality"`
}

func NewOptimizePayload(fileID uuid.UUID, quality int) OptimizePayload {
	if quality <= 0 {
		quality = 85
	}
	return OptimizePayload{FileID: fileID, Quality: quality}
}
