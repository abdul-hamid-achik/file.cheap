package worker

import (
	"context"
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/job-queue/pkg/job"
	"github.com/google/uuid"
)

// ThumbnailHandlerV2 creates a thumbnail handler using the middleware pattern
// This is equivalent to the original ThumbnailHandler but with ~80% less code
func ThumbnailHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*ThumbnailPayload](deps, FileJobConfig{
		JobType:       "thumbnail",
		ProcessorName: "thumbnail",
		VariantType:   db.VariantTypeThumbnail,
		BuildFilename: func(_ any) string {
			return "thumb.jpg"
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*ThumbnailPayload)
			return &processor.Options{
				Width:    payload.Width,
				Height:   payload.Height,
				Quality:  payload.Quality,
				Position: payload.Position,
			}
		},
		UpdateFileStatus: true,
		EnableWebhooks:   true,
	}).Build()
}

// ResizeHandlerV2 creates a resize handler using the middleware pattern
func ResizeHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*ResizePayload](deps, FileJobConfig{
		JobType:       "resize",
		ProcessorName: "resize",
		VariantType:   db.VariantType("dynamic"), // set dynamically
		BuildFilename: func(p any) string {
			payload := p.(*ResizePayload)
			return fmt.Sprintf("%s.jpg", payload.VariantType)
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*ResizePayload)
			return &processor.Options{
				Width:       payload.Width,
				Height:      payload.Height,
				Quality:     payload.Quality,
				VariantType: payload.VariantType,
			}
		},
		BuildVariantKey: func(fileID uuid.UUID, p any, filename string) string {
			payload := p.(*ResizePayload)
			return buildVariantKey(fileID, payload.VariantType, filename)
		},
		UpdateFileStatus: true,
		EnableWebhooks:   true,
	}).Build()
}

// WebPHandlerV2 creates a WebP conversion handler using the middleware pattern
func WebPHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*WebPPayload](deps, FileJobConfig{
		JobType:       "webp",
		ProcessorName: "webp",
		VariantType:   db.VariantTypeWebp,
		BuildFilename: func(_ any) string {
			return "converted.webp"
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*WebPPayload)
			return &processor.Options{
				Quality: payload.Quality,
			}
		},
		UpdateFileStatus: true,
		EnableWebhooks:   false, // Original didn't have webhooks
	}).Build()
}

// WatermarkHandlerV2 creates a watermark handler using the middleware pattern
func WatermarkHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*WatermarkPayload](deps, FileJobConfig{
		JobType:       "watermark",
		ProcessorName: "watermark",
		VariantType:   db.VariantTypeWatermarked,
		BuildFilename: func(_ any) string {
			return "watermarked.jpg"
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*WatermarkPayload)
			return &processor.Options{
				VariantType: payload.Text,
				Fit:         payload.Position,
				Quality:     int(payload.Opacity * 100),
				Width:       payload.FontSize,
			}
		},
		UpdateFileStatus: true,
		EnableWebhooks:   false, // Original didn't have webhooks
	}).Build()
}

// OptimizeHandlerV2 creates an optimization handler using the middleware pattern
func OptimizeHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*OptimizePayload](deps, FileJobConfig{
		JobType:       "optimize",
		ProcessorName: "optimize",
		VariantType:   db.VariantTypeOptimized,
		BuildFilename: func(_ any) string {
			return "optimized.jpg"
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*OptimizePayload)
			return &processor.Options{
				Quality: payload.Quality,
			}
		},
		UpdateFileStatus: true,
		EnableWebhooks:   false, // Original didn't have webhooks
	}).Build()
}

// ConvertHandlerV2 creates a format conversion handler using the middleware pattern
func ConvertHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*ConvertPayload](deps, FileJobConfig{
		JobType:       "convert",
		ProcessorName: "convert",
		VariantType:   db.VariantType("converted"), // dynamic based on format
		BuildFilename: func(p any) string {
			payload := p.(*ConvertPayload)
			return fmt.Sprintf("converted.%s", payload.Format)
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*ConvertPayload)
			return &processor.Options{
				Format:  payload.Format,
				Quality: payload.Quality,
			}
		},
		UpdateFileStatus: false, // Original didn't update status
		EnableWebhooks:   false,
	}).Build()
}

// PDFThumbnailHandlerV2 creates a PDF thumbnail handler using the middleware pattern
func PDFThumbnailHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*PDFThumbnailPayload](deps, FileJobConfig{
		JobType:       "pdf_thumbnail",
		ProcessorName: "pdf_thumbnail",
		VariantType:   db.VariantTypePdfPreview,
		BuildFilename: func(p any) string {
			payload := p.(*PDFThumbnailPayload)
			ext := "png"
			if payload.Format == "jpeg" || payload.Format == "jpg" {
				ext = "jpg"
			}
			return fmt.Sprintf("preview.%s", ext)
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*PDFThumbnailPayload)
			return &processor.Options{
				Width:   payload.Width,
				Height:  payload.Height,
				Quality: payload.Quality,
				Format:  payload.Format,
				Page:    payload.Page,
			}
		},
		Validate: func(jc *JobContext, _ any) error {
			if jc.File.ContentType != "application/pdf" {
				return fmt.Errorf("file is not a PDF: %s", jc.File.ContentType)
			}
			return nil
		},
		UpdateFileStatus: true,
		EnableWebhooks:   false,
	}).Build()
}

// VideoThumbnailHandlerV2 creates a video thumbnail handler using the middleware pattern
func VideoThumbnailHandlerV2(deps *Dependencies) func(context.Context, *job.Job) error {
	return NewFileJobBuilder[*VideoThumbnailPayload](deps, FileJobConfig{
		JobType:       "video_thumbnail",
		ProcessorName: "video_thumbnail",
		VariantType:   db.VariantType("video_thumbnail"),
		BuildFilename: func(p any) string {
			payload := p.(*VideoThumbnailPayload)
			ext := "jpg"
			if payload.Format == "png" {
				ext = "png"
			}
			return fmt.Sprintf("thumbnail.%s", ext)
		},
		BuildOptions: func(p any) *processor.Options {
			payload := p.(*VideoThumbnailPayload)
			// Use Page field to pass percentage (1-100)
			pagePercent := int(payload.AtPercent * 100)
			if pagePercent <= 0 {
				pagePercent = 10 // default 10%
			}
			return &processor.Options{
				Width:   payload.Width,
				Height:  payload.Height,
				Quality: payload.Quality,
				Format:  payload.Format,
				Page:    pagePercent,
			}
		},
		UpdateFileStatus: true,
		EnableWebhooks:   false,
	}).Build()
}
