package storage

import (
	"context"
	"errors"
	"io"
)

var (
	ErrNotFound      = errors.New("storage: file not found")
	ErrAlreadyExists = errors.New("storage: file already exists")
	ErrInvalidKey    = errors.New("storage: invalid key")
	ErrAccessDenied  = errors.New("storage: access denied")
)

type Storage interface {
	Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	GetPresignedURL(ctx context.Context, key string, expirySeconds int) (string, error)
	HealthCheck(ctx context.Context) error
}

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string
}
