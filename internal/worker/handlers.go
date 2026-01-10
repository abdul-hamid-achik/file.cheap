package worker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	"github.com/abdul-hamid-achik/job-queue/pkg/job"
	"github.com/abdul-hamid-achik/job-queue/pkg/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Dependencies struct {
	Storage  storage.Storage
	Registry *processor.Registry
	Queries  *db.Queries
}

func ThumbnailHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "thumbnail")
		log.Info("job started")
		start := time.Now()

		var payload ThumbnailPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original file reader")
		log.Debug("file downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("thumbnail")

		opts := &processor.Options{
			Width:   payload.Width,
			Height:  payload.Height,
			Quality: payload.Quality,
		}

		log.Debug("processing thumbnail", "width", payload.Width, "height", payload.Height)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to process thumbnail", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to process thumbnail: %w", err))
		}
		log.Debug("thumbnail processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		variantKey := buildVariantKey(payload.FileID, "thumbnail", "thumb.jpg")
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantTypeThumbnail,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			return fmt.Errorf("failed to update file status: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height)
		return nil
	}
}

func ResizeHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "resize")
		log.Info("job started")
		start := time.Now()

		var payload ResizePayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String(), "variant_type", payload.VariantType)

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original file reader")
		log.Debug("file downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("resize")

		opts := &processor.Options{
			Width:       payload.Width,
			Height:      payload.Height,
			Quality:     payload.Quality,
			VariantType: payload.VariantType,
		}

		log.Debug("processing resize", "width", payload.Width, "height", payload.Height)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to process resize", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to process resize: %w", err))
		}
		log.Debug("resize processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		filename := fmt.Sprintf("%s.jpg", payload.VariantType)
		variantKey := buildVariantKey(payload.FileID, payload.VariantType, filename)
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		variantType := db.VariantType(payload.VariantType)
		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: variantType,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			return fmt.Errorf("failed to update file status: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height)
		return nil
	}
}

func WebPHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "webp")
		log.Info("job started")
		start := time.Now()

		var payload WebPPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original file reader")
		log.Debug("file downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("webp")

		opts := &processor.Options{
			Quality: payload.Quality,
		}

		log.Debug("processing webp conversion", "quality", payload.Quality)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to process webp conversion", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to process webp: %w", err))
		}
		log.Debug("webp processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		variantKey := buildVariantKey(payload.FileID, "webp", "converted.webp")
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantTypeWebp,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			return fmt.Errorf("failed to update file status: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height)
		return nil
	}
}

func WatermarkHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "watermark")
		log.Info("job started")
		start := time.Now()

		var payload WatermarkPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original file reader")
		log.Debug("file downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("watermark")

		opts := &processor.Options{
			VariantType: payload.Text,
			Fit:         payload.Position,
			Quality:     int(payload.Opacity * 100),
			Width:       payload.FontSize,
		}

		log.Debug("processing watermark", "text", payload.Text, "position", payload.Position)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to process watermark", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to process watermark: %w", err))
		}
		log.Debug("watermark processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		variantKey := buildVariantKey(payload.FileID, "watermarked", "watermarked.jpg")
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantTypeWatermarked,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			return fmt.Errorf("failed to update file status: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height)
		return nil
	}
}

func PDFThumbnailHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "pdf_thumbnail")
		log.Info("job started")
		start := time.Now()

		var payload PDFThumbnailPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String(), "page", payload.Page)

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		if file.ContentType != "application/pdf" {
			log.Error("file is not a PDF", "content_type", file.ContentType)
			return middleware.Permanent(fmt.Errorf("file is not a PDF: %s", file.ContentType))
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original file reader")
		log.Debug("file downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("pdf_thumbnail")

		opts := &processor.Options{
			Width:   payload.Width,
			Height:  payload.Height,
			Quality: payload.Quality,
			Format:  payload.Format,
			Page:    payload.Page,
		}

		log.Debug("processing pdf thumbnail", "width", payload.Width, "height", payload.Height, "page", payload.Page, "format", payload.Format)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to process pdf thumbnail", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to process pdf thumbnail: %w", err))
		}
		log.Debug("pdf thumbnail processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		ext := "png"
		if payload.Format == "jpeg" || payload.Format == "jpg" {
			ext = "jpg"
		}
		variantKey := buildVariantKey(payload.FileID, "pdf_preview", fmt.Sprintf("preview.%s", ext))
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantTypePdfPreview,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			return fmt.Errorf("failed to update file status: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height)
		return nil
	}
}

func MetadataHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "metadata")
		log.Info("job started")
		start := time.Now()

		var payload MetadataPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "error", err)
			return fmt.Errorf("failed to download file: %w", err)
		}
		defer closeSafely(reader, "file reader")

		proc := deps.Registry.MustGet("metadata")
		result, err := proc.Process(ctx, nil, reader)
		if err != nil {
			log.Error("failed to extract metadata", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to extract metadata: %w", err))
		}

		variantKey := buildVariantKey(payload.FileID, "metadata", "metadata.json")
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload metadata", "error", err)
			return fmt.Errorf("failed to upload metadata: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds())
		return nil
	}
}

func OptimizeHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "optimize")
		log.Info("job started")
		start := time.Now()

		var payload OptimizePayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "error", err)
			return fmt.Errorf("failed to download file: %w", err)
		}
		defer closeSafely(reader, "file reader")

		proc := deps.Registry.MustGet("optimize")
		opts := &processor.Options{Quality: payload.Quality}
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to optimize file", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to optimize: %w", err))
		}

		variantKey := buildVariantKey(payload.FileID, "optimized", "optimized.jpg")
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload optimized file", "error", err)
			return fmt.Errorf("failed to upload: %w", err)
		}

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantTypeOptimized,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			return fmt.Errorf("failed to save variant: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			return fmt.Errorf("failed to update file status: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds())
		return nil
	}
}

func ConvertHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "convert")
		log.Info("job started")
		start := time.Now()

		var payload ConvertPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("file_id", payload.FileID.String(), "format", payload.Format)

		fileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "error", err)
			return fmt.Errorf("failed to download file: %w", err)
		}
		defer closeSafely(reader, "file reader")

		proc := deps.Registry.MustGet("convert")
		opts := &processor.Options{
			Format:  payload.Format,
			Quality: payload.Quality,
		}

		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to convert file", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to convert: %w", err))
		}

		variantKey := buildVariantKey(payload.FileID, "converted", fmt.Sprintf("converted.%s", payload.Format))
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload converted file", "error", err)
			return fmt.Errorf("failed to upload: %w", err)
		}

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		variantType := db.VariantType(payload.Format)
		if payload.Format == "webp" {
			variantType = db.VariantTypeWebp
		}
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: variantType,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			return fmt.Errorf("failed to save variant: %w", err)
		}

		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds())
		return nil
	}
}

func buildVariantKey(fileID uuid.UUID, variantType, filename string) string {
	return fmt.Sprintf("processed/%s/%s/%s", fileID, variantType, filename)
}

func closeSafely(c io.Closer, name string) {
	if c != nil {
		if err := c.Close(); err != nil {
			logger.Default().Warn("error closing resource", "name", name, "error", err)
		}
	}
}
