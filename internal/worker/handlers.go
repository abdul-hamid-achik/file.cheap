package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/video"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	"github.com/abdul-hamid-achik/file.cheap/internal/webhook"
	"github.com/abdul-hamid-achik/job-queue/pkg/job"
	"github.com/abdul-hamid-achik/job-queue/pkg/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Dependencies struct {
	Storage           storage.Storage
	Registry          *processor.Registry
	Queries           *db.Queries
	WebhookDispatcher *webhook.Dispatcher
}

func (d *Dependencies) markJobRunning(ctx context.Context, jobID pgtype.UUID) {
	if jobID.Valid {
		if err := d.Queries.MarkJobRunning(ctx, jobID); err != nil {
			logger.FromContext(ctx).Warn("failed to mark job running", "job_id", jobID, "error", err)
		}
	}
}

func (d *Dependencies) markJobCompleted(ctx context.Context, jobID pgtype.UUID) {
	if jobID.Valid {
		if err := d.Queries.MarkJobCompleted(ctx, jobID); err != nil {
			logger.FromContext(ctx).Warn("failed to mark job completed", "job_id", jobID, "error", err)
		}
	}
}

func (d *Dependencies) markJobFailed(ctx context.Context, jobID pgtype.UUID, errMsg string) {
	if jobID.Valid {
		if err := d.Queries.MarkJobFailed(ctx, db.MarkJobFailedParams{
			ID:           jobID,
			ErrorMessage: &errMsg,
		}); err != nil {
			logger.FromContext(ctx).Warn("failed to mark job failed", "job_id", jobID, "error", err)
		}
	}
}

func (d *Dependencies) dispatchProcessingCompleted(ctx context.Context, userID pgtype.UUID, fileID, jobID, jobType, variantKey, contentType string, sizeBytes, durationMs int64) {
	if d.WebhookDispatcher == nil || !userID.Valid {
		return
	}
	event, err := webhook.NewProcessingCompletedEvent(fileID, jobID, jobType, variantKey, contentType, sizeBytes, durationMs)
	if err != nil {
		return
	}
	uid := uuid.UUID(userID.Bytes)
	go func() {
		if err := d.WebhookDispatcher.Dispatch(context.Background(), uid, event); err != nil {
			logger.FromContext(ctx).Debug("webhook dispatch failed", "error", err)
		}
	}()
}

func (d *Dependencies) dispatchProcessingFailed(ctx context.Context, userID pgtype.UUID, fileID, jobID, jobType, errMsg string) {
	if d.WebhookDispatcher == nil || !userID.Valid {
		return
	}
	event, err := webhook.NewProcessingFailedEvent(fileID, jobID, jobType, errMsg)
	if err != nil {
		return
	}
	uid := uuid.UUID(userID.Bytes)
	go func() {
		if err := d.WebhookDispatcher.Dispatch(context.Background(), uid, event); err != nil {
			logger.FromContext(ctx).Debug("webhook dispatch failed", "error", err)
		}
	}()
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "thumbnail", err.Error())
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original file reader")
		log.Debug("file downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("thumbnail")

		opts := &processor.Options{
			Width:    payload.Width,
			Height:   payload.Height,
			Quality:  payload.Quality,
			Position: payload.Position,
		}

		log.Debug("processing thumbnail", "width", payload.Width, "height", payload.Height, "position", payload.Position)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to process thumbnail", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "thumbnail", err.Error())
			return middleware.Permanent(fmt.Errorf("failed to process thumbnail: %w", err))
		}
		log.Debug("thumbnail processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		variantKey := buildVariantKey(payload.FileID, "thumbnail", "thumb.jpg")
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "thumbnail", err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "thumbnail", err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "thumbnail", err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
		durationMs := time.Since(start).Milliseconds()
		deps.dispatchProcessingCompleted(ctx, file.UserID, payload.FileID.String(), j.ID, "thumbnail", variantKey, result.ContentType, result.Size, durationMs)
		log.Info("job completed", "duration_ms", durationMs, "output_width", width, "output_height", height)
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String(), "variant_type", payload.VariantType)

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "resize", err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "resize", err.Error())
			return middleware.Permanent(fmt.Errorf("failed to process resize: %w", err))
		}
		log.Debug("resize processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		filename := fmt.Sprintf("%s.jpg", payload.VariantType)
		variantKey := buildVariantKey(payload.FileID, payload.VariantType, filename)
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "resize", err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "resize", err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			deps.dispatchProcessingFailed(ctx, file.UserID, payload.FileID.String(), j.ID, "resize", err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
		durationMs := time.Since(start).Milliseconds()
		deps.dispatchProcessingCompleted(ctx, file.UserID, payload.FileID.String(), j.ID, "resize", variantKey, result.ContentType, result.Size, durationMs)
		log.Info("job completed", "duration_ms", durationMs, "output_width", width, "output_height", height)
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to process webp: %w", err))
		}
		log.Debug("webp processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		variantKey := buildVariantKey(payload.FileID, "webp", "converted.webp")
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to process watermark: %w", err))
		}
		log.Debug("watermark processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		variantKey := buildVariantKey(payload.FileID, "watermarked", "watermarked.jpg")
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String(), "page", payload.Page)

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		if file.ContentType != "application/pdf" {
			log.Error("file is not a PDF", "content_type", file.ContentType)
			deps.markJobFailed(ctx, payload.JobID, "file is not a PDF: "+file.ContentType)
			return middleware.Permanent(fmt.Errorf("file is not a PDF: %s", file.ContentType))
		}

		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to download file: %w", err)
		}
		defer closeSafely(reader, "file reader")

		proc := deps.Registry.MustGet("metadata")
		result, err := proc.Process(ctx, nil, reader)
		if err != nil {
			log.Error("failed to extract metadata", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to extract metadata: %w", err))
		}

		variantKey := buildVariantKey(payload.FileID, "metadata", "metadata.json")
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload metadata", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to upload metadata: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to download file: %w", err)
		}
		defer closeSafely(reader, "file reader")

		proc := deps.Registry.MustGet("optimize")
		opts := &processor.Options{Quality: payload.Quality}
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to optimize file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to optimize: %w", err))
		}

		variantKey := buildVariantKey(payload.FileID, "optimized", "optimized.jpg")
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload optimized file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
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

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String(), "format", payload.Format)

		fileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to convert: %w", err))
		}

		variantKey := buildVariantKey(payload.FileID, "converted", fmt.Sprintf("converted.%s", payload.Format))
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload converted file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
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
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
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

// Video handlers

func VideoThumbnailHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "video_thumbnail")
		log.Info("job started")
		start := time.Now()

		var payload VideoThumbnailPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading video from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original video reader")
		log.Debug("video downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("video_thumbnail")

		// Use Page field to pass percentage (1-100)
		pagePercent := int(payload.AtPercent * 100)
		if pagePercent <= 0 {
			pagePercent = 10 // default 10%
		}

		opts := &processor.Options{
			Width:   payload.Width,
			Height:  payload.Height,
			Quality: payload.Quality,
			Format:  payload.Format,
			Page:    pagePercent,
		}

		log.Debug("extracting video thumbnail", "width", payload.Width, "height", payload.Height, "at_percent", payload.AtPercent)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to extract video thumbnail", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to extract video thumbnail: %w", err))
		}
		log.Debug("video thumbnail extracted", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		ext := "jpg"
		if payload.Format == "png" {
			ext = "png"
		}
		variantKey := buildVariantKey(payload.FileID, "video_thumbnail", fmt.Sprintf("thumbnail.%s", ext))
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantType("video_thumbnail"),
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height)
		return nil
	}
}

func VideoTranscodeHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "video_transcode")
		log.Info("job started")
		start := time.Now()

		var payload VideoTranscodePayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String(), "variant_type", payload.VariantType)

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading video from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original video reader")
		log.Debug("video downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("video_transcode")

		// Map CRF to quality (CRF 0-51 inverted to quality 0-100)
		quality := 100 - (payload.CRF * 100 / 51)

		opts := &processor.Options{
			Quality:     quality,
			Format:      payload.OutputFormat,
			Height:      payload.MaxResolution,
			VariantType: payload.VariantType,
		}

		log.Debug("transcoding video", "output_format", payload.OutputFormat, "max_resolution", payload.MaxResolution, "preset", payload.Preset)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to transcode video", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to transcode video: %w", err))
		}
		log.Debug("video transcoded", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		ext := payload.OutputFormat
		if ext == "" {
			ext = "mp4"
		}
		filename := fmt.Sprintf("video.%s", ext)
		variantKey := buildVariantKey(payload.FileID, payload.VariantType, filename)
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		resolution := fmt.Sprintf("%dx%d", result.Metadata.Width, result.Metadata.Height)
		_, err = deps.Queries.CreateVideoVariant(ctx, db.CreateVideoVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantType(payload.VariantType),
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
			DurationSeconds: pgtype.Numeric{
				Int:   big.NewInt(int64(result.Metadata.Duration * 100)),
				Exp:   -2,
				Valid: true,
			},
			Resolution: &resolution,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		// Track video processing minutes
		videoDuration := int32(result.Metadata.Duration)
		if videoDuration > 0 {
			if err := deps.Queries.EnsureMonthlyUsageRecord(ctx, file.UserID); err != nil {
				log.Warn("failed to ensure monthly usage record", "error", err)
			}
			if err := deps.Queries.IncrementVideoSecondsProcessed(ctx, db.IncrementVideoSecondsProcessedParams{
				UserID:                file.UserID,
				VideoSecondsProcessed: videoDuration,
			}); err != nil {
				log.Warn("failed to track video processing duration", "error", err)
			}
		}

		// Auto-delete original if user setting is enabled
		userSettings, err := deps.Queries.GetUserSettings(ctx, file.UserID)
		if err == nil && userSettings.AutoDeleteOriginals && file.StorageKey != "" {
			if err := deps.Storage.Delete(ctx, file.StorageKey); err != nil {
				log.Warn("failed to delete original file", "error", err)
			} else {
				if err := deps.Queries.MarkOriginalDeleted(ctx, file.ID); err != nil {
					log.Warn("failed to mark original as deleted", "error", err)
				}
				log.Debug("original file deleted (auto_delete_originals enabled)")
			}
		}

		deps.markJobCompleted(ctx, payload.JobID)
		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height, "duration_seconds", result.Metadata.Duration)
		return nil
	}
}

func VideoHLSHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "video_hls")
		log.Info("job started")
		start := time.Now()

		var payload VideoHLSPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading video from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original video reader")
		log.Debug("video downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("video_transcode")
		ffmpegProc, ok := proc.(*video.FFmpegProcessor)
		if !ok {
			log.Error("video_transcode processor is not FFmpegProcessor")
			deps.markJobFailed(ctx, payload.JobID, "invalid processor type")
			return middleware.Permanent(fmt.Errorf("invalid processor type"))
		}

		segmentDuration := payload.SegmentDuration
		if segmentDuration <= 0 {
			segmentDuration = 10
		}

		opts := &video.VideoOptions{
			Preset:             "medium",
			CRF:                23,
			HLSSegmentDuration: segmentDuration,
		}

		log.Debug("generating HLS", "segment_duration", segmentDuration, "resolutions", payload.Resolutions)
		processStart := time.Now()
		hlsResult, err := ffmpegProc.GenerateHLS(ctx, opts, reader)
		if err != nil {
			log.Error("failed to generate HLS", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to generate HLS: %w", err))
		}
		defer func() { _ = os.RemoveAll(filepath.Dir(hlsResult.ManifestPath)) }()
		log.Debug("HLS generated", "duration_ms", time.Since(processStart).Milliseconds(), "segments", hlsResult.SegmentCount)

		manifestData, err := os.ReadFile(hlsResult.ManifestPath)
		if err != nil {
			log.Error("failed to read manifest", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to read manifest: %w", err)
		}

		manifestKey := buildVariantKey(payload.FileID, "hls_master", "playlist.m3u8")
		log.Debug("uploading manifest", "storage_key", manifestKey)
		if err := deps.Storage.Upload(ctx, manifestKey, bytes.NewReader(manifestData), "application/x-mpegURL", int64(len(manifestData))); err != nil {
			log.Error("failed to upload manifest", "storage_key", manifestKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to upload manifest: %w", err)
		}

		for _, segPath := range hlsResult.SegmentPaths {
			segData, err := os.ReadFile(segPath)
			if err != nil {
				log.Error("failed to read segment", "path", segPath, "error", err)
				deps.markJobFailed(ctx, payload.JobID, err.Error())
				return fmt.Errorf("failed to read segment: %w", err)
			}
			segName := filepath.Base(segPath)
			segKey := buildVariantKey(payload.FileID, "hls_master", segName)
			if err := deps.Storage.Upload(ctx, segKey, bytes.NewReader(segData), "video/mp2t", int64(len(segData))); err != nil {
				log.Error("failed to upload segment", "storage_key", segKey, "error", err)
				deps.markJobFailed(ctx, payload.JobID, err.Error())
				return fmt.Errorf("failed to upload segment: %w", err)
			}
		}

		_, err = deps.Queries.CreateVideoVariant(ctx, db.CreateVideoVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantType("hls_master"),
			ContentType: "application/x-mpegURL",
			SizeBytes:   int64(len(manifestData)),
			StorageKey:  manifestKey,
			DurationSeconds: pgtype.Numeric{
				Int:   big.NewInt(int64(hlsResult.TotalDuration * 100)),
				Exp:   -2,
				Valid: true,
			},
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "segments", hlsResult.SegmentCount, "duration_seconds", hlsResult.TotalDuration)
		return nil
	}
}

func VideoWatermarkHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "video_watermark")
		log.Info("job started")
		start := time.Now()

		var payload VideoWatermarkPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		deps.markJobRunning(ctx, payload.JobID)
		log = log.With("file_id", payload.FileID.String())

		fileID := pgtype.UUID{
			Bytes: payload.FileID,
			Valid: true,
		}

		file, err := deps.Queries.GetFile(ctx, fileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		log.Debug("downloading video from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original video reader")
		log.Debug("video downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())

		proc := deps.Registry.MustGet("video_transcode")
		ffmpegProc, ok := proc.(*video.FFmpegProcessor)
		if !ok {
			log.Error("video_transcode processor is not FFmpegProcessor")
			deps.markJobFailed(ctx, payload.JobID, "invalid processor type")
			return middleware.Permanent(fmt.Errorf("invalid processor type"))
		}

		text := payload.Text
		if text == "" {
			text = "file.cheap"
		}
		position := payload.Position
		if position == "" {
			position = "bottom-right"
		}
		opacity := payload.Opacity
		if opacity <= 0 || opacity > 1 {
			opacity = 0.5
		}

		log.Debug("adding watermark", "text", text, "position", position, "opacity", opacity)
		processStart := time.Now()
		result, err := ffmpegProc.AddWatermark(ctx, reader, text, position, opacity)
		if err != nil {
			log.Error("failed to add watermark", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return middleware.Permanent(fmt.Errorf("failed to add watermark: %w", err))
		}
		log.Debug("watermark added", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		variantKey := buildVariantKey(payload.FileID, "video_watermarked", "video.mp4")
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantType("video_watermarked"),
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		if err := deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			log.Error("failed to update file status", "error", err)
			deps.markJobFailed(ctx, payload.JobID, err.Error())
			return fmt.Errorf("failed to update file status: %w", err)
		}

		deps.markJobCompleted(ctx, payload.JobID)
		log.Info("job completed", "duration_ms", time.Since(start).Milliseconds(), "output_width", width, "output_height", height)
		return nil
	}
}

// ZipDownloadHandler creates a ZIP archive of multiple files for bulk download
func ZipDownloadHandler(deps *Dependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "zip_download")
		log.Info("job started")
		start := time.Now()

		var payload ZipDownloadPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		log = log.With("zip_download_id", payload.ZipDownloadID.String(), "file_count", len(payload.FileIDs))

		// Mark as running
		zipID := pgtype.UUID{Bytes: payload.ZipDownloadID, Valid: true}
		if err := deps.Queries.UpdateZipDownloadRunning(ctx, zipID); err != nil {
			log.Warn("failed to mark zip download running", "error", err)
		}

		// Convert file IDs to pgtype.UUID
		pgFileIDs := make([]pgtype.UUID, len(payload.FileIDs))
		for i, id := range payload.FileIDs {
			pgFileIDs[i] = pgtype.UUID{Bytes: id, Valid: true}
		}

		// Fetch all files
		files, err := deps.Queries.GetFilesByIDs(ctx, pgFileIDs)
		if err != nil {
			errMsg := fmt.Sprintf("failed to fetch files: %v", err)
			log.Error("failed to fetch files", "error", err)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return fmt.Errorf("failed to fetch files: %w", err)
		}

		// Verify ownership
		userID := pgtype.UUID{Bytes: payload.UserID, Valid: true}
		validFiles := make([]db.File, 0, len(files))
		for _, f := range files {
			if f.UserID == userID && !f.DeletedAt.Valid {
				validFiles = append(validFiles, f)
			}
		}

		if len(validFiles) == 0 {
			errMsg := "no valid files found"
			log.Error(errMsg)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return middleware.Permanent(fmt.Errorf("%s", errMsg))
		}

		// Create temporary ZIP file
		tmpDir := os.TempDir()
		zipPath := filepath.Join(tmpDir, fmt.Sprintf("download_%s.zip", payload.ZipDownloadID.String()))
		zipFile, err := os.Create(zipPath)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create temp file: %v", err)
			log.Error("failed to create temp file", "error", err)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer func() {
			_ = zipFile.Close()
			_ = os.Remove(zipPath)
		}()

		// Create ZIP archive
		zipWriter := NewZipWriter(zipFile)
		defer func() { _ = zipWriter.Close() }()

		// Track filenames to handle duplicates
		filenameCounts := make(map[string]int)

		for _, file := range validFiles {
			log.Debug("adding file to zip", "filename", file.Filename, "storage_key", file.StorageKey)

			reader, err := deps.Storage.Download(ctx, file.StorageKey)
			if err != nil {
				log.Warn("failed to download file, skipping", "filename", file.Filename, "error", err)
				continue
			}

			// Handle duplicate filenames
			filename := file.Filename
			if count, exists := filenameCounts[filename]; exists {
				ext := filepath.Ext(filename)
				base := filename[:len(filename)-len(ext)]
				filename = fmt.Sprintf("%s_%d%s", base, count+1, ext)
			}
			filenameCounts[file.Filename]++

			if err := zipWriter.AddFile(filename, reader); err != nil {
				closeSafely(reader, "file reader")
				log.Warn("failed to add file to zip, skipping", "filename", file.Filename, "error", err)
				continue
			}
			closeSafely(reader, "file reader")
		}

		if err := zipWriter.Close(); err != nil {
			errMsg := fmt.Sprintf("failed to finalize zip: %v", err)
			log.Error("failed to finalize zip", "error", err)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return fmt.Errorf("failed to finalize zip: %w", err)
		}

		// Get file info
		zipInfo, err := zipFile.Stat()
		if err != nil {
			errMsg := fmt.Sprintf("failed to stat zip: %v", err)
			log.Error("failed to stat zip", "error", err)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return fmt.Errorf("failed to stat zip: %w", err)
		}

		// Upload to storage
		storageKey := fmt.Sprintf("downloads/%s/%s.zip", payload.UserID.String(), payload.ZipDownloadID.String())

		// Reopen file for reading
		_ = zipFile.Close()
		uploadFile, err := os.Open(zipPath)
		if err != nil {
			errMsg := fmt.Sprintf("failed to reopen zip: %v", err)
			log.Error("failed to reopen zip", "error", err)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return fmt.Errorf("failed to reopen zip: %w", err)
		}
		defer func() { _ = uploadFile.Close() }()

		if err := deps.Storage.Upload(ctx, storageKey, uploadFile, "application/zip", zipInfo.Size()); err != nil {
			errMsg := fmt.Sprintf("failed to upload zip: %v", err)
			log.Error("failed to upload zip", "error", err)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return fmt.Errorf("failed to upload zip: %w", err)
		}

		// Generate presigned URL (valid for 24 hours)
		expiresAt := time.Now().Add(24 * time.Hour)
		downloadURL, err := deps.Storage.GetPresignedURL(ctx, storageKey, 86400)
		if err != nil {
			errMsg := fmt.Sprintf("failed to generate download URL: %v", err)
			log.Error("failed to generate download URL", "error", err)
			_ = deps.Queries.UpdateZipDownloadFailed(ctx, db.UpdateZipDownloadFailedParams{
				ID:           zipID,
				ErrorMessage: &errMsg,
			})
			return fmt.Errorf("failed to generate download URL: %w", err)
		}

		// Update database
		zipSize := zipInfo.Size()
		err = deps.Queries.UpdateZipDownloadCompleted(ctx, db.UpdateZipDownloadCompletedParams{
			ID:          zipID,
			StorageKey:  &storageKey,
			SizeBytes:   &zipSize,
			DownloadUrl: &downloadURL,
			ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		})
		if err != nil {
			log.Error("failed to update zip download record", "error", err)
			return fmt.Errorf("failed to update zip download record: %w", err)
		}

		log.Info("job completed",
			"duration_ms", time.Since(start).Milliseconds(),
			"file_count", len(validFiles),
			"size_bytes", zipInfo.Size())
		return nil
	}
}

// ZipWriter wraps archive/zip for easier file addition
type ZipWriter struct {
	file   *os.File
	writer *zip.Writer
}

func NewZipWriter(f *os.File) *ZipWriter {
	return &ZipWriter{
		file:   f,
		writer: zip.NewWriter(f),
	}
}

func (z *ZipWriter) AddFile(name string, r io.Reader) error {
	w, err := z.writer.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

func (z *ZipWriter) Close() error {
	return z.writer.Close()
}
