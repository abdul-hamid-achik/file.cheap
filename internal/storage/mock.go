package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

// MemoryStorage is an in-memory implementation of Storage for testing.
// It stores files in a map and is safe for concurrent use.
type MemoryStorage struct {
	files map[string]memoryFile
	mu    sync.RWMutex
}

type memoryFile struct {
	data        []byte
	contentType string
}

// NewMemoryStorage creates a new in-memory storage instance.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		files: make(map[string]memoryFile),
	}
}

// Ensure MemoryStorage implements Storage
var _ Storage = (*MemoryStorage)(nil)

// Upload stores data at the given key.
func (s *MemoryStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if key == "" {
		return ErrInvalidKey
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read data: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.files[key] = memoryFile{
		data:        data,
		contentType: contentType,
	}

	return nil
}

// Download retrieves data from the given key.
func (s *MemoryStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[key]
	if !exists {
		return nil, ErrNotFound
	}

	return io.NopCloser(bytes.NewReader(file.data)), nil
}

// Delete removes the file at the given key.
func (s *MemoryStorage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.files, key)
	return nil
}

// Exists checks if a file exists at the given key.
func (s *MemoryStorage) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.files[key]
	return exists, nil
}

// GetPresignedURL returns a fake presigned URL for testing.
func (s *MemoryStorage) GetPresignedURL(ctx context.Context, key string, expirySeconds int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.files[key]; !exists {
		return "", ErrNotFound
	}

	return fmt.Sprintf("http://test-storage/%s?expires=%d", key, expirySeconds), nil
}

// GetData returns the raw data for a key (test helper).
func (s *MemoryStorage) GetData(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[key]
	if !exists {
		return nil, false
	}
	return file.data, true
}

// GetContentType returns the content type for a key (test helper).
func (s *MemoryStorage) GetContentType(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[key]
	if !exists {
		return "", false
	}
	return file.contentType, true
}

// Clear removes all files (test helper).
func (s *MemoryStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = make(map[string]memoryFile)
}

// Count returns the number of stored files (test helper).
func (s *MemoryStorage) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.files)
}
