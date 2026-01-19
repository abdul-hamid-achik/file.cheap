package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/metrics"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
)

type CleanupDependencies struct {
	Storage storage.Storage
	Queries *db.Queries
}

type CleanupStats struct {
	SoftDeletedCleaned   int
	RetentionExpired     int
	StorageDeleteErrors  int
	DatabaseDeleteErrors int
}

func RunCleanup(ctx context.Context, deps *CleanupDependencies) (*CleanupStats, error) {
	log := logger.FromContext(ctx)
	log.Info("starting cleanup job")
	start := time.Now()

	stats := &CleanupStats{}

	if err := cleanupSoftDeletedFiles(ctx, deps, stats); err != nil {
		log.Error("failed to cleanup soft-deleted files", "error", err)
	}

	if err := cleanupRetentionExpiredFiles(ctx, deps, stats); err != nil {
		log.Error("failed to cleanup retention-expired files", "error", err)
	}

	log.Info("cleanup job completed",
		"duration_ms", time.Since(start).Milliseconds(),
		"soft_deleted_cleaned", stats.SoftDeletedCleaned,
		"retention_expired", stats.RetentionExpired,
		"storage_errors", stats.StorageDeleteErrors,
		"database_errors", stats.DatabaseDeleteErrors,
	)

	return stats, nil
}

func cleanupSoftDeletedFiles(ctx context.Context, deps *CleanupDependencies, stats *CleanupStats) error {
	log := logger.FromContext(ctx)

	batchSize := int32(100)
	for {
		files, err := deps.Queries.ListExpiredSoftDeletedFiles(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("failed to list expired soft-deleted files: %w", err)
		}

		if len(files) == 0 {
			break
		}

		for _, file := range files {
			if file.StorageKey != "" {
				if err := deps.Storage.Delete(ctx, file.StorageKey); err != nil {
					log.Warn("failed to delete file from storage",
						"file_id", file.ID.Bytes,
						"storage_key", file.StorageKey,
						"error", err,
					)
					stats.StorageDeleteErrors++
				}
			}

			if err := deps.Queries.HardDeleteFile(ctx, file.ID); err != nil {
				log.Warn("failed to hard-delete file from database",
					"file_id", file.ID.Bytes,
					"error", err,
				)
				stats.DatabaseDeleteErrors++
				metrics.RecordFileDeletion("error")
				continue
			}

			metrics.RecordFileDeletion("success")
			stats.SoftDeletedCleaned++
		}

		if int32(len(files)) < batchSize {
			break
		}
	}

	return nil
}

func cleanupRetentionExpiredFiles(ctx context.Context, deps *CleanupDependencies, stats *CleanupStats) error {
	log := logger.FromContext(ctx)

	batchSize := int32(100)
	for {
		files, err := deps.Queries.ListRetentionExpiredFiles(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("failed to list retention-expired files: %w", err)
		}

		if len(files) == 0 {
			break
		}

		for _, file := range files {
			if file.StorageKey != "" {
				if err := deps.Storage.Delete(ctx, file.StorageKey); err != nil {
					log.Warn("failed to delete file from storage",
						"file_id", file.ID.Bytes,
						"storage_key", file.StorageKey,
						"error", err,
					)
					stats.StorageDeleteErrors++
				}
			}

			if err := deps.Queries.SoftDeleteFile(ctx, file.ID); err != nil {
				log.Warn("failed to soft-delete expired file",
					"file_id", file.ID.Bytes,
					"error", err,
				)
				stats.DatabaseDeleteErrors++
				metrics.RecordFileDeletion("error")
				continue
			}

			metrics.RecordFileDeletion("success")
			stats.RetentionExpired++
		}

		if int32(len(files)) < batchSize {
			break
		}
	}

	return nil
}
