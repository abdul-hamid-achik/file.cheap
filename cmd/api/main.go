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

	"github.com/abdul-hamid-achik/file.cheap/internal/analytics"
	"github.com/abdul-hamid-achik/file.cheap/internal/api"
	"github.com/abdul-hamid-achik/file.cheap/internal/auth"
	"github.com/abdul-hamid-achik/file.cheap/internal/billing"
	"github.com/abdul-hamid-achik/file.cheap/internal/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/email"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/metrics"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	imgproc "github.com/abdul-hamid-achik/file.cheap/internal/processor/image"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	"github.com/abdul-hamid-achik/file.cheap/internal/tracing"
	"github.com/abdul-hamid-achik/file.cheap/internal/web"
	"github.com/abdul-hamid-achik/job-queue/pkg/broker"
	"github.com/abdul-hamid-achik/job-queue/pkg/job"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

type brokerAdapter struct {
	broker *broker.RedisStreamsBroker
}

func (a *brokerAdapter) Enqueue(jobType string, payload interface{}) (string, error) {
	j, err := job.New(jobType, payload)
	if err != nil {
		return "", fmt.Errorf("failed to create job: %w", err)
	}
	if err := a.broker.Enqueue(context.Background(), j); err != nil {
		return "", err
	}
	return j.ID, nil
}

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

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	logger.Init(cfg.LogLevel)
	log := logger.Default()

	log.Info("configuration loaded")

	ctx := context.Background()

	if cfg.TracingEnabled {
		shutdownTracing, err := tracing.Init(ctx, &tracing.Config{
			ServiceName:    "api",
			ServiceVersion: "1.0.0",
			Environment:    cfg.Environment,
			OTLPEndpoint:   cfg.OTLPEndpoint,
			Enabled:        true,
			SampleRate:     cfg.TraceSampleRate,
		})
		if err != nil {
			return fmt.Errorf("failed to init tracing: %w", err)
		}
		defer func() { _ = shutdownTracing(ctx) }()
		log.Info("tracing enabled", "endpoint", cfg.OTLPEndpoint, "sample_rate", cfg.TraceSampleRate)
	}

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
	if err := store.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("failed to ensure bucket: %w", err)
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

	b := broker.NewRedisStreamsBroker(redisClient)
	log.Info("broker initialized")

	queries := db.New(pool)

	log.Info("setting up auth services")
	authService := auth.NewService(queries)
	sessionManager := auth.NewSessionManager(queries, cfg.Secure)

	var oauthService *auth.OAuthService
	if cfg.GoogleClientID != "" || cfg.GitHubClientID != "" {
		oauthService = auth.NewOAuthService(queries, auth.OAuthConfig{
			GoogleClientID:     cfg.GoogleClientID,
			GoogleClientSecret: cfg.GoogleClientSecret,
			GitHubClientID:     cfg.GitHubClientID,
			GitHubClientSecret: cfg.GitHubClientSecret,
			BaseURL:            cfg.BaseURL,
		})
		log.Info("oauth services configured")
	}

	log.Info("setting up routes")

	metrics.SetAppInfo("1.0.0", cfg.Environment, "api")

	instrumentedStore := metrics.NewInstrumentedStorage(store)

	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())

	registry := processor.NewRegistry()
	registry.Register("resize", imgproc.NewResizeProcessor(nil))
	registry.Register("thumbnail", imgproc.NewThumbnailProcessor(nil))
	registry.Register("webp", imgproc.NewWebPProcessor(nil))
	registry.Register("watermark", imgproc.NewWatermarkProcessor(nil))
	registry.Register("metadata", imgproc.NewMetadataProcessor(nil))
	registry.Register("optimize", imgproc.NewOptimizeProcessor(nil))
	registry.Register("convert", imgproc.NewConvertProcessor(nil))

	apiCfg := &api.Config{
		Storage:       instrumentedStore,
		Broker:        &brokerAdapter{broker: b},
		Queries:       queries,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
		BaseURL:       cfg.BaseURL,
		Registry:      registry,
		Pool:          pool,
		RedisClient:   redisClient,
	}
	apiRouter := api.NewRouter(apiCfg)
	mux.Handle("/v1/", apiRouter)
	mux.Handle("/health", apiRouter)
	mux.Handle("/health/", apiRouter)
	mux.Handle("/cdn/", apiRouter)

	webCfg := &web.Config{
		Storage: instrumentedStore,
		Queries: queries,
		Broker:  &brokerAdapter{broker: b},
		BaseURL: cfg.BaseURL,
		Secure:  cfg.Secure,
	}

	var billingHandlers *web.BillingHandlers
	if cfg.StripeSecretKey != "" {
		stripeClient := billing.NewClient(
			cfg.StripeSecretKey,
			cfg.StripePublishableKey,
			cfg.StripeWebhookSecret,
			cfg.StripePriceIDPro,
		)
		billingService := billing.NewService(stripeClient, queries, cfg.BaseURL)
		webhookHandler := billing.NewWebhookHandler(billingService, cfg.StripeWebhookSecret, log)
		billingHandlers = web.NewBillingHandlers(billingService, webhookHandler)
		log.Info("stripe billing configured")
	}

	poolStats := analytics.NewPoolStatsFunc(func() analytics.PoolStats { return pool.Stat() })
	analyticsService := web.NewAnalyticsService(webCfg, redisClient, poolStats)
	analyticsHandlers := web.NewAnalyticsHandlers(analyticsService)
	adminHandlers := web.NewAdminHandlers(analyticsService)
	log.Info("analytics services configured")

	emailService := email.NewService(email.Config{
		SMTPHost:     cfg.SMTPHost,
		SMTPPort:     cfg.SMTPPort,
		SMTPUsername: cfg.SMTPUsername,
		SMTPPassword: cfg.SMTPPassword,
		FromAddress:  cfg.SMTPFromAddress,
		FromName:     cfg.SMTPFromName,
		BaseURL:      cfg.BaseURL,
	})
	log.Info("email service configured")

	webRouter := web.NewRouter(webCfg, sessionManager, authService, oauthService, emailService, billingHandlers, analyticsHandlers, adminHandlers)
	mux.Handle("/", webRouter)

	handler := api.SecurityHeaders(metrics.HTTPMetricsMiddleware(web.Recovery(web.RequestID(web.RequestLogger(mux)))))
	if cfg.TracingEnabled {
		handler = tracing.HTTPMiddleware("api")(handler)
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		redisSetFunc := func(ctx context.Context, key string, value interface{}, exp time.Duration) error {
			return redisClient.Set(ctx, key, value, exp).Err()
		}
		for {
			select {
			case <-ticker.C:
				metrics.UpdateLatencyMetrics(context.Background(), redisSetFunc)
			case <-shutdown:
				return
			}
		}
	}()

	// Start chunked upload session cleanup goroutine
	uploadSessionStore := api.GetSessionStore()
	go uploadSessionStore.StartCleanup(ctx, store)

	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sessionManager.CleanupExpiredSessions(context.Background()); err != nil {
					log.Error("session cleanup failed", "error", err)
				}
			case <-shutdown:
				return
			}
		}
	}()

	go func() {
		log.Info("server starting", "port", cfg.Port, "url", fmt.Sprintf("http://localhost:%d", cfg.Port))
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
	case sig := <-shutdown:
		log.Info("shutdown signal received", "signal", sig)

		// Stop background cleanup goroutines
		uploadSessionStore.StopCleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
			return fmt.Errorf("forced shutdown: %w", err)
		}
	}

	log.Info("server stopped gracefully")
	return nil
}
