package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/metrics"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/image"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/pdf"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	fpworker "github.com/abdul-hamid-achik/file.cheap/internal/worker"
	"github.com/abdul-hamid-achik/job-queue/pkg/broker"
	"github.com/abdul-hamid-achik/job-queue/pkg/middleware"
	"github.com/abdul-hamid-achik/job-queue/pkg/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
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

	log.Info("configuration loaded")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zerologger := zerolog.New(os.Stdout).With().Timestamp().Logger()

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

	log.Info("connecting to redis")
	redisOpt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("failed to parse redis url: %w", err)
	}
	redisClient := redis.NewClient(redisOpt)
	defer func() { _ = redisClient.Close() }()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	b := broker.NewRedisStreamsBroker(redisClient,
		broker.WithWorkerID(fmt.Sprintf("worker-%d", os.Getpid())),
	)
	log.Info("broker initialized")

	queries := db.New(pool)

	metrics.SetAppInfo("1.0.0", cfg.Environment, "worker")
	metrics.SetWorkerPoolSize(cfg.WorkerConcurrency)

	instrumentedStore := metrics.NewInstrumentedStorage(store)

	log.Info("registering processors")
	procRegistry := processor.NewRegistry()
	procRegistry.Register("thumbnail", image.NewThumbnailProcessor(processor.DefaultConfig()))
	procRegistry.Register("resize", image.NewResizeProcessor(processor.DefaultConfig()))
	procRegistry.Register("webp", image.NewWebPProcessor(processor.DefaultConfig()))
	procRegistry.Register("watermark", image.NewWatermarkProcessor(processor.DefaultConfig()))
	procRegistry.Register("pdf_thumbnail", pdf.NewThumbnailProcessor(processor.DefaultConfig()))
	procRegistry.Register("metadata", image.NewMetadataProcessor(processor.DefaultConfig()))
	procRegistry.Register("optimize", image.NewOptimizeProcessor(processor.DefaultConfig()))
	procRegistry.Register("convert", image.NewConvertProcessor(processor.DefaultConfig()))

	registerVideoProcessors(procRegistry, log)

	log.Info("processor registry ready", "count", len(procRegistry.List()))

	deps := &fpworker.Dependencies{
		Storage:  instrumentedStore,
		Registry: procRegistry,
		Queries:  queries,
	}

	log.Info("registering job handlers")
	registry := worker.NewRegistry()
	_ = registry.Register("thumbnail", fpworker.ThumbnailHandler(deps))
	_ = registry.Register("resize", fpworker.ResizeHandler(deps))
	_ = registry.Register("webp", fpworker.WebPHandler(deps))
	_ = registry.Register("watermark", fpworker.WatermarkHandler(deps))
	_ = registry.Register("pdf_thumbnail", fpworker.PDFThumbnailHandler(deps))
	_ = registry.Register("metadata", fpworker.MetadataHandler(deps))
	_ = registry.Register("optimize", fpworker.OptimizeHandler(deps))
	_ = registry.Register("convert", fpworker.ConvertHandler(deps))

	registerVideoHandlers(registry, deps)

	log.Info("handlers registered", "count", len(registry.Types()))

	registry.Use(
		middleware.RecoveryMiddleware(zerologger),
		middleware.LoggingMiddleware(zerologger),
		middleware.TimeoutMiddleware(cfg.JobTimeout),
		middleware.MetricsMiddleware(metrics.NewPrometheusCollector()),
	)

	log.Info("creating worker pool", "concurrency", cfg.WorkerConcurrency)

	workerPool := worker.NewPool(b, registry,
		worker.WithConcurrency(cfg.WorkerConcurrency),
		worker.WithPoolQueues([]string{"default"}),
		worker.WithPoolPollInterval(time.Second),
		worker.WithShutdownTimeout(30*time.Second),
		worker.WithPoolLogger(zerologger),
	)

	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "9090"
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	metricsServer := &http.Server{
		Addr:    ":" + metricsPort,
		Handler: metricsMux,
	}

	go func() {
		log.Info("metrics server starting", "port", metricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server error", "error", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	poolErr := make(chan error, 1)
	go func() {
		log.Info("starting worker pool")
		poolErr <- workerPool.Start(ctx)
	}()

	select {
	case err := <-poolErr:
		if err != nil && err != context.Canceled {
			return fmt.Errorf("worker pool error: %w", err)
		}
	case sig := <-shutdown:
		log.Info("shutdown signal received", "signal", sig)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := workerPool.Stop(shutdownCtx); err != nil {
			log.Error("error stopping pool", "error", err)
		}

		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error("error stopping metrics server", "error", err)
		}

		cancel()
	}

	log.Info("worker pool stopped gracefully")
	return nil
}
