package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var _ Storage = (*LocalStorage)(nil)

// LocalStorage is a filesystem-based implementation of Storage.
// Files are stored under rootDir using the key as a relative path.
type LocalStorage struct {
	rootDir string
}

// NewLocalStorage creates a new local filesystem storage rooted at rootDir.
// The directory is created if it does not exist.
func NewLocalStorage(rootDir string) (*LocalStorage, error) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve root dir: %w", err)
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create root dir: %w", err)
	}

	return &LocalStorage{rootDir: abs}, nil
}

func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fullPath, err := s.resolve(key)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("create parent dirs for %s: %w", key, err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", key, err)
	}
	defer f.Close() //nolint:errcheck // write errors caught by io.Copy

	if _, err := io.Copy(f, reader); err != nil {
		_ = os.Remove(fullPath)
		return fmt.Errorf("write file %s: %w", key, err)
	}

	return nil
}

func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	fullPath, err := s.resolve(key)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("open file %s: %w", key, err)
	}

	return f, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fullPath, err := s.resolve(key)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("delete file %s: %w", key, err)
	}

	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	fullPath, err := s.resolve(key)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat file %s: %w", key, err)
	}

	return true, nil
}

func (s *LocalStorage) GetPresignedURL(ctx context.Context, key string, expirySeconds int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	fullPath, err := s.resolve(key)
	if err != nil {
		return "", err
	}

	return "file://" + fullPath, nil
}

func (s *LocalStorage) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	info, err := os.Stat(s.rootDir)
	if err != nil {
		return fmt.Errorf("root dir not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("root path is not a directory: %s", s.rootDir)
	}

	// Verify writable by creating and removing a temp file.
	tmp := filepath.Join(s.rootDir, ".healthcheck")
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("root dir not writable: %w", err)
	}
	_ = f.Close()
	_ = os.Remove(tmp)

	return nil
}

// resolve validates the key and returns the absolute file path.
// It rejects empty keys, absolute paths, and path traversal attempts.
func (s *LocalStorage) resolve(key string) (string, error) {
	if key == "" {
		return "", ErrInvalidKey
	}

	if filepath.IsAbs(key) {
		return "", ErrInvalidKey
	}

	if strings.Contains(key, "..") {
		return "", ErrInvalidKey
	}

	return filepath.Join(s.rootDir, filepath.FromSlash(key)), nil
}
