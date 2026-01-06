package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/logger"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var _ Storage = (*MinIOStorage)(nil)

type MinIOStorage struct {
	client *minio.Client
	bucket string
	config *Config
}

func NewMinIOStorage(cfg *Config) (*MinIOStorage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	return &MinIOStorage{
		client: client,
		bucket: cfg.Bucket,
		config: cfg,
	}, nil
}

func (s *MinIOStorage) EnsureBucket(ctx context.Context) error {
	log := logger.FromContext(ctx)

	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		log.Info("creating bucket", "bucket", s.bucket, "region", s.config.Region)
		err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{
			Region: s.config.Region,
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		log.Info("bucket created", "bucket", s.bucket)
	}

	return nil
}

func (s *MinIOStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error {
	log := logger.FromContext(ctx)
	start := time.Now()

	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		log.Error("storage upload failed", "key", key, "size", size, "error", err)
		return fmt.Errorf("upload to %s: %w", key, err)
	}

	log.Debug("storage upload completed", "key", key, "size", size, "content_type", contentType, "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (s *MinIOStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	log := logger.FromContext(ctx)
	start := time.Now()

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		log.Error("storage download failed", "key", key, "error", err)
		return nil, fmt.Errorf("download %s: %w", key, err)
	}

	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		if isNotFoundError(err) {
			log.Warn("storage object not found", "key", key)
			return nil, ErrNotFound
		}
		log.Error("storage stat failed", "key", key, "error", err)
		return nil, fmt.Errorf("stat %s: %w", key, err)
	}

	log.Debug("storage download started", "key", key, "size", info.Size, "duration_ms", time.Since(start).Milliseconds())
	return obj, nil
}

func (s *MinIOStorage) Delete(ctx context.Context, key string) error {
	log := logger.FromContext(ctx)

	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		log.Error("storage delete failed", "key", key, "error", err)
		return fmt.Errorf("delete %s: %w", key, err)
	}

	log.Debug("storage object deleted", "key", key)
	return nil
}

func (s *MinIOStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("check exists %s: %w", key, err)
	}
	return true, nil
}

func (s *MinIOStorage) GetPresignedURL(ctx context.Context, key string, expirySeconds int) (string, error) {
	log := logger.FromContext(ctx)

	url, err := s.client.PresignedGetObject(ctx, s.bucket, key, presignDuration(expirySeconds), nil)
	if err != nil {
		log.Error("storage presign failed", "key", key, "error", err)
		return "", fmt.Errorf("presign %s: %w", key, err)
	}

	log.Debug("storage presigned url generated", "key", key, "expiry_seconds", expirySeconds)
	return url.String(), nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errResp := minio.ToErrorResponse(err)
	return errResp.Code == "NoSuchKey"
}

func presignDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
