package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// TestMemoryStorage_Upload tests the Upload method.
func TestMemoryStorage_Upload(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		content     string
		contentType string
		wantErr     error
	}{
		{
			name:        "upload text file",
			key:         "test/file.txt",
			content:     "hello world",
			contentType: "text/plain",
			wantErr:     nil,
		},
		{
			name:        "upload binary data",
			key:         "test/image.jpg",
			content:     "\xff\xd8\xff\xe0binary data",
			contentType: "image/jpeg",
			wantErr:     nil,
		},
		{
			name:        "upload with empty key",
			key:         "",
			content:     "content",
			contentType: "text/plain",
			wantErr:     ErrInvalidKey,
		},
		{
			name:        "upload empty content",
			key:         "test/empty.txt",
			content:     "",
			contentType: "text/plain",
			wantErr:     nil,
		},
		{
			name:        "upload with nested path",
			key:         "a/b/c/d/file.txt",
			content:     "nested",
			contentType: "text/plain",
			wantErr:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewMemoryStorage()
			ctx := context.Background()
			reader := strings.NewReader(tt.content)

			err := storage.Upload(ctx, tt.key, reader, tt.contentType, int64(len(tt.content)))

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Upload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr == nil {
				data, exists := storage.GetData(tt.key)
				if !exists {
					t.Error("Upload() file not stored")
					return
				}
				if string(data) != tt.content {
					t.Errorf("Upload() stored content = %q, want %q", string(data), tt.content)
				}

				ct, _ := storage.GetContentType(tt.key)
				if ct != tt.contentType {
					t.Errorf("Upload() content type = %q, want %q", ct, tt.contentType)
				}
			}
		})
	}
}

// TestMemoryStorage_Upload_ContextCanceled tests upload with canceled context.
func TestMemoryStorage_Upload_ContextCanceled(t *testing.T) {
	storage := NewMemoryStorage()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := storage.Upload(ctx, "test.txt", strings.NewReader("data"), "text/plain", 4)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Upload() with canceled context error = %v, want context.Canceled", err)
	}
}

// TestMemoryStorage_Download tests the Download method.
func TestMemoryStorage_Download(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(s *MemoryStorage)
		key         string
		wantContent string
		wantErr     error
	}{
		{
			name: "download existing file",
			setup: func(s *MemoryStorage) {
				_ = s.Upload(context.Background(), "test/file.txt", strings.NewReader("hello world"), "text/plain", 11)
			},
			key:         "test/file.txt",
			wantContent: "hello world",
			wantErr:     nil,
		},
		{
			name:        "download non-existent file",
			setup:       nil,
			key:         "test/missing.txt",
			wantContent: "",
			wantErr:     ErrNotFound,
		},
		{
			name: "download empty file",
			setup: func(s *MemoryStorage) {
				_ = s.Upload(context.Background(), "test/empty.txt", strings.NewReader(""), "text/plain", 0)
			},
			key:         "test/empty.txt",
			wantContent: "",
			wantErr:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewMemoryStorage()
			if tt.setup != nil {
				tt.setup(storage)
			}

			ctx := context.Background()
			reader, err := storage.Download(ctx, tt.key)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Download() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr == nil {
				defer func() { _ = reader.Close() }()
				content, _ := io.ReadAll(reader)
				if string(content) != tt.wantContent {
					t.Errorf("Download() content = %q, want %q", string(content), tt.wantContent)
				}
			}
		})
	}
}

// TestMemoryStorage_Download_ContextCanceled tests download with canceled context.
func TestMemoryStorage_Download_ContextCanceled(t *testing.T) {
	storage := NewMemoryStorage()
	_ = storage.Upload(context.Background(), "test.txt", strings.NewReader("data"), "text/plain", 4)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := storage.Download(ctx, "test.txt")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Download() with canceled context error = %v, want context.Canceled", err)
	}
}

// TestMemoryStorage_Delete tests the Delete method.
func TestMemoryStorage_Delete(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *MemoryStorage)
		key     string
		wantErr error
	}{
		{
			name: "delete existing file",
			setup: func(s *MemoryStorage) {
				_ = s.Upload(context.Background(), "test/file.txt", strings.NewReader("content"), "text/plain", 7)
			},
			key:     "test/file.txt",
			wantErr: nil,
		},
		{
			name:    "delete non-existent file (idempotent)",
			setup:   nil,
			key:     "test/missing.txt",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewMemoryStorage()
			if tt.setup != nil {
				tt.setup(storage)
			}

			ctx := context.Background()
			err := storage.Delete(ctx, tt.key)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			exists, _ := storage.Exists(ctx, tt.key)
			if exists {
				t.Error("Delete() file still exists")
			}
		})
	}
}

// TestMemoryStorage_Exists tests the Exists method.
func TestMemoryStorage_Exists(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(s *MemoryStorage)
		key        string
		wantExists bool
		wantErr    error
	}{
		{
			name: "file exists",
			setup: func(s *MemoryStorage) {
				_ = s.Upload(context.Background(), "test/file.txt", strings.NewReader("content"), "text/plain", 7)
			},
			key:        "test/file.txt",
			wantExists: true,
			wantErr:    nil,
		},
		{
			name:       "file does not exist",
			setup:      nil,
			key:        "test/missing.txt",
			wantExists: false,
			wantErr:    nil,
		},
		{
			name: "file deleted",
			setup: func(s *MemoryStorage) {
				_ = s.Upload(context.Background(), "test/file.txt", strings.NewReader("content"), "text/plain", 7)
				_ = s.Delete(context.Background(), "test/file.txt")
			},
			key:        "test/file.txt",
			wantExists: false,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewMemoryStorage()
			if tt.setup != nil {
				tt.setup(storage)
			}

			ctx := context.Background()
			exists, err := storage.Exists(ctx, tt.key)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Exists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if exists != tt.wantExists {
				t.Errorf("Exists() = %v, want %v", exists, tt.wantExists)
			}
		})
	}
}

// TestMemoryStorage_GetPresignedURL tests the GetPresignedURL method.
func TestMemoryStorage_GetPresignedURL(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(s *MemoryStorage)
		key           string
		expirySeconds int
		wantErr       error
		wantContains  string
	}{
		{
			name: "generate URL for existing file",
			setup: func(s *MemoryStorage) {
				_ = s.Upload(context.Background(), "test/file.txt", strings.NewReader("content"), "text/plain", 7)
			},
			key:           "test/file.txt",
			expirySeconds: 3600,
			wantErr:       nil,
			wantContains:  "test/file.txt",
		},
		{
			name:          "generate URL for non-existent file",
			setup:         nil,
			key:           "test/missing.txt",
			expirySeconds: 3600,
			wantErr:       ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewMemoryStorage()
			if tt.setup != nil {
				tt.setup(storage)
			}

			ctx := context.Background()
			url, err := storage.GetPresignedURL(ctx, tt.key, tt.expirySeconds)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("GetPresignedURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr == nil && !strings.Contains(url, tt.wantContains) {
				t.Errorf("GetPresignedURL() = %q, want to contain %q", url, tt.wantContains)
			}
		})
	}
}

// TestMemoryStorage_Concurrent tests concurrent access safety.
func TestMemoryStorage_Concurrent(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent uploads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a'+n%26)) + "/file.txt"
			content := strings.Repeat("x", n)
			_ = storage.Upload(ctx, key, strings.NewReader(content), "text/plain", int64(len(content)))
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a'+n%26)) + "/file.txt"
			_, _ = storage.Exists(ctx, key)
			if r, err := storage.Download(ctx, key); err == nil {
				_, _ = io.Copy(io.Discard, r)
				_ = r.Close()
			}
		}(i)
	}

	wg.Wait()

	if storage.Count() == 0 {
		t.Error("Expected some files to be stored")
	}
}
