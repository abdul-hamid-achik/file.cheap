package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          int
	MaxUploadSize int64
	BaseURL       string
	Secure        bool

	Environment string
	LogLevel    string
	LogFormat   string

	DatabaseURL string
	RedisURL    string

	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool
	MinIORegion    string

	WorkerConcurrency int
	JobTimeout        time.Duration
	MaxRetries        int

	JWTSecret string

	// OAuth configuration
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string

	// Email configuration
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFromAddress string
	SMTPFromName    string
}

func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	cfg.Port = getEnvInt("PORT", 8080)
	cfg.MaxUploadSize = getEnvInt64("MAX_UPLOAD_SIZE", 100*1024*1024)

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	cfg.RedisURL = os.Getenv("REDIS_URL")
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}

	cfg.MinIOEndpoint = os.Getenv("MINIO_ENDPOINT")
	if cfg.MinIOEndpoint == "" {
		return nil, fmt.Errorf("MINIO_ENDPOINT is required")
	}

	cfg.MinIOAccessKey = os.Getenv("MINIO_ACCESS_KEY")
	if cfg.MinIOAccessKey == "" {
		return nil, fmt.Errorf("MINIO_ACCESS_KEY is required")
	}

	cfg.MinIOSecretKey = os.Getenv("MINIO_SECRET_KEY")
	if cfg.MinIOSecretKey == "" {
		return nil, fmt.Errorf("MINIO_SECRET_KEY is required")
	}

	cfg.MinIOBucket = getEnvString("MINIO_BUCKET", "files")
	cfg.MinIOUseSSL = getEnvBool("MINIO_USE_SSL", false)
	cfg.MinIORegion = getEnvString("MINIO_REGION", "us-east-1")

	cfg.WorkerConcurrency = getEnvInt("WORKER_CONCURRENCY", 4)
	cfg.JobTimeout, err = getEnvDuration("JOB_TIMEOUT", "5m")
	if err != nil {
		return nil, fmt.Errorf("invalid JOB_TIMEOUT: %w", err)
	}
	cfg.MaxRetries = getEnvInt("MAX_RETRIES", 3)

	cfg.JWTSecret = getEnvString("JWT_SECRET", "change-me-in-production")

	// Base URL and security
	cfg.BaseURL = getEnvString("BASE_URL", "http://localhost:8080")
	cfg.Secure = getEnvBool("SECURE_COOKIES", false)

	// OAuth (optional)
	cfg.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	cfg.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	cfg.GitHubClientID = os.Getenv("GITHUB_CLIENT_ID")
	cfg.GitHubClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")

	// Email (optional for dev, Mailhog defaults)
	cfg.SMTPHost = getEnvString("SMTP_HOST", "localhost")
	cfg.SMTPPort = getEnvInt("SMTP_PORT", 1025)
	cfg.SMTPUsername = os.Getenv("SMTP_USERNAME")
	cfg.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	cfg.SMTPFromAddress = getEnvString("SMTP_FROM_ADDRESS", "noreply@fileprocessor.local")
	cfg.SMTPFromName = getEnvString("SMTP_FROM_NAME", "File Processor")

	cfg.Environment = getEnvString("ENVIRONMENT", "development")
	cfg.LogLevel = getEnvString("LOG_LEVEL", "info")
	cfg.LogFormat = os.Getenv("LOG_FORMAT")

	return cfg, nil
}

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key, defaultValue string) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return time.ParseDuration(value)
}

func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}

	if c.MaxUploadSize < 1 {
		return fmt.Errorf("invalid max upload size: %d", c.MaxUploadSize)
	}

	if c.WorkerConcurrency < 1 {
		return fmt.Errorf("invalid worker concurrency: %d", c.WorkerConcurrency)
	}

	return nil
}
