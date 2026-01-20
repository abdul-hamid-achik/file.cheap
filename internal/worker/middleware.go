package worker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/job-queue/pkg/job"
	"github.com/abdul-hamid-achik/job-queue/pkg/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"log/slog"
)

// FileJobPayload extends JobPayload with GetJobID for the middleware
type FileJobPayload interface {
	JobPayload
	GetJobID() pgtype.UUID
}

// JobContext holds common data during job execution
type JobContext struct {
	Context   context.Context
	Job       *job.Job
	Log       *slog.Logger
	StartTime time.Time
	File      db.File
	Reader    io.ReadCloser
	Deps      *Dependencies
}

// FileJobConfig configures a standard file processing handler
type FileJobConfig struct {
	// JobType is used for logging and webhook events (e.g., "thumbnail", "resize")
	JobType string

	// ProcessorName is the name of the processor in the registry
	ProcessorName string

	// VariantType for the database record
	VariantType db.VariantType

	// BuildFilename returns the output filename given the payload
	BuildFilename func(payload any) string

	// BuildOptions creates processor options from the payload
	BuildOptions func(payload any) *processor.Options

	// BuildVariantKey returns the storage key for the variant
	// If nil, defaults to buildVariantKey(fileID, variantType, filename)
	BuildVariantKey func(fileID uuid.UUID, payload any, filename string) string

	// Validate performs payload-specific validation before processing
	// Return an error to abort with permanent failure
	Validate func(ctx *JobContext, payload any) error

	// UpdateFileStatus controls whether to update file status to completed after success
	UpdateFileStatus bool

	// EnableWebhooks controls whether to dispatch processing.completed/failed webhooks
	EnableWebhooks bool
}

// FileJobBuilder creates file processing handlers with reduced boilerplate
type FileJobBuilder[P FileJobPayload] struct {
	deps   *Dependencies
	config FileJobConfig
}

// NewFileJobBuilder creates a new builder for file processing handlers
func NewFileJobBuilder[P FileJobPayload](deps *Dependencies, config FileJobConfig) *FileJobBuilder[P] {
	return &FileJobBuilder[P]{
		deps:   deps,
		config: config,
	}
}

// Build returns a job handler function with the standard processing flow
func (b *FileJobBuilder[P]) Build() func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", b.config.JobType)
		log.Info("job started")
		start := time.Now()

		// Parse payload
		var payload P
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		// Extract common fields using interface methods
		jobID := payload.GetJobID()
		pgFileID := payload.GetFileID()
		fileID := uuid.UUID(pgFileID.Bytes)

		b.deps.markJobRunning(ctx, jobID)
		log = log.With("file_id", fileID.String())

		// Get file from database
		file, err := b.deps.Queries.GetFile(ctx, pgFileID)
		if err != nil {
			log.Error("failed to retrieve file", "error", err)
			b.deps.markJobFailed(ctx, jobID, err.Error())
			return fmt.Errorf("failed to retrieve file: %w", err)
		}

		// Create job context for validation
		jc := &JobContext{
			Context:   ctx,
			Job:       j,
			Log:       log,
			StartTime: start,
			File:      file,
			Deps:      b.deps,
		}

		// Run payload-specific validation
		if b.config.Validate != nil {
			if err := b.config.Validate(jc, any(payload)); err != nil {
				log.Error("validation failed", "error", err)
				b.deps.markJobFailed(ctx, jobID, err.Error())
				return middleware.Permanent(err)
			}
		}

		// Download file from storage
		log.Debug("downloading file from storage", "storage_key", file.StorageKey)
		downloadStart := time.Now()
		reader, err := b.deps.Storage.Download(ctx, file.StorageKey)
		if err != nil {
			log.Error("failed to download file", "storage_key", file.StorageKey, "error", err)
			b.deps.markJobFailed(ctx, jobID, err.Error())
			if b.config.EnableWebhooks {
				b.deps.dispatchProcessingFailed(ctx, file.UserID, fileID.String(), j.ID, b.config.JobType, err.Error())
			}
			return fmt.Errorf("failed to download file %s: %w", file.StorageKey, err)
		}
		defer closeSafely(reader, "original file reader")
		log.Debug("file downloaded", "duration_ms", time.Since(downloadStart).Milliseconds())
		jc.Reader = reader

		// Get processor
		proc, err := b.deps.Registry.GetOrError(b.config.ProcessorName)
		if err != nil {
			log.Error("processor not found", "processor", b.config.ProcessorName, "error", err)
			b.deps.markJobFailed(ctx, jobID, err.Error())
			if b.config.EnableWebhooks {
				b.deps.dispatchProcessingFailed(ctx, file.UserID, fileID.String(), j.ID, b.config.JobType, err.Error())
			}
			return middleware.Permanent(err)
		}

		// Build options and process
		opts := b.config.BuildOptions(any(payload))
		log.Debug("processing", "options", opts)
		processStart := time.Now()
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			log.Error("failed to process", "error", err)
			b.deps.markJobFailed(ctx, jobID, err.Error())
			if b.config.EnableWebhooks {
				b.deps.dispatchProcessingFailed(ctx, file.UserID, fileID.String(), j.ID, b.config.JobType, err.Error())
			}
			return middleware.Permanent(fmt.Errorf("failed to process %s: %w", b.config.JobType, err))
		}
		log.Debug("processed", "duration_ms", time.Since(processStart).Milliseconds(), "output_size", result.Size)

		// Build variant key
		filename := b.config.BuildFilename(any(payload))
		var variantKey string
		if b.config.BuildVariantKey != nil {
			variantKey = b.config.BuildVariantKey(fileID, any(payload), filename)
		} else {
			variantKey = buildVariantKey(fileID, string(b.config.VariantType), filename)
		}

		// Upload to storage
		log.Debug("uploading variant", "storage_key", variantKey)
		uploadStart := time.Now()
		if err := b.deps.Storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			log.Error("failed to upload variant", "storage_key", variantKey, "error", err)
			b.deps.markJobFailed(ctx, jobID, err.Error())
			if b.config.EnableWebhooks {
				b.deps.dispatchProcessingFailed(ctx, file.UserID, fileID.String(), j.ID, b.config.JobType, err.Error())
			}
			return fmt.Errorf("failed to upload variant: %w", err)
		}
		log.Debug("variant uploaded", "duration_ms", time.Since(uploadStart).Milliseconds())

		// Create variant record
		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = b.deps.Queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: b.config.VariantType,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			log.Error("failed to save variant record", "error", err)
			b.deps.markJobFailed(ctx, jobID, err.Error())
			if b.config.EnableWebhooks {
				b.deps.dispatchProcessingFailed(ctx, file.UserID, fileID.String(), j.ID, b.config.JobType, err.Error())
			}
			return fmt.Errorf("failed to save variant record: %w", err)
		}

		// Update file status if configured
		if b.config.UpdateFileStatus {
			if err := b.deps.Queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
				ID:     file.ID,
				Status: db.FileStatusCompleted,
			}); err != nil {
				log.Error("failed to update file status", "error", err)
				b.deps.markJobFailed(ctx, jobID, err.Error())
				if b.config.EnableWebhooks {
					b.deps.dispatchProcessingFailed(ctx, file.UserID, fileID.String(), j.ID, b.config.JobType, err.Error())
				}
				return fmt.Errorf("failed to update file status: %w", err)
			}
		}

		// Success
		b.deps.markJobCompleted(ctx, jobID)
		durationMs := time.Since(start).Milliseconds()
		if b.config.EnableWebhooks {
			b.deps.dispatchProcessingCompleted(ctx, file.UserID, fileID.String(), j.ID, b.config.JobType, variantKey, result.ContentType, result.Size, durationMs)
		}
		log.Info("job completed", "duration_ms", durationMs, "output_width", width, "output_height", height)
		return nil
	}
}

// DefaultBuildVariantKey returns the standard variant key format
func DefaultBuildVariantKey(fileID uuid.UUID, variantType, filename string) string {
	return buildVariantKey(fileID, variantType, filename)
}
