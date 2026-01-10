package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	"github.com/abdul-hamid-achik/file.cheap/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		slog.Error("cleanup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Init(cfg.LogLevel)
	log := logger.Default()

	log.Info("starting cleanup job")
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	log.Info("connecting to database")
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	log.Info("database connected")

	log.Info("connecting to object storage")
	storageCfg := &storage.Config{
		Endpoint:  cfg.MinIOEndpoint,
		AccessKey: cfg.MinIOAccessKey,
		SecretKey: cfg.MinIOSecretKey,
		Bucket:    cfg.MinIOBucket,
		UseSSL:    cfg.MinIOUseSSL,
		Region:    cfg.MinIORegion,
	}
	store, err := storage.NewMinIOStorage(storageCfg)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}
	log.Info("object storage connected")

	queries := db.New(pool)

	deps := &worker.CleanupDependencies{
		Storage: store,
		Queries: queries,
	}

	stats, err := worker.RunCleanup(logger.WithLogger(ctx, log), deps)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	log.Info("cleanup completed",
		"duration_ms", time.Since(start).Milliseconds(),
		"soft_deleted_cleaned", stats.SoftDeletedCleaned,
		"retention_expired", stats.RetentionExpired,
		"storage_errors", stats.StorageDeleteErrors,
		"database_errors", stats.DatabaseDeleteErrors,
	)

	return nil
}
