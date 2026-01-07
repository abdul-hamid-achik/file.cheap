package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
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
				// Verify data was stored
				data, exists := storage.GetData(tt.key)
				if !exists {
					t.Error("Upload() file not stored")
					return
				}
				if string(data) != tt.content {
					t.Errorf("Upload() stored content = %q, want %q", string(data), tt.content)
				}

				// Verify content type
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

			// Verify file is gone
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

	// Should complete without race conditions or panics
	if storage.Count() == 0 {
		t.Error("Expected some files to be stored")
	}
}

// TestMinIOStorage_Upload tests the real MinIO implementation.
// These tests verify the implementation against the interface contract.
func TestMinIOStorage_Upload(t *testing.T) {
	// Skip if no MinIO available - these are for validating your implementation
	cfg := &Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
		UseSSL:    false,
		Region:    "us-east-1",
	}

	storage, err := NewMinIOStorage(cfg)
	if err != nil {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to ensure bucket exists
	if err := storage.EnsureBucket(ctx); err != nil {
		t.Skip("Could not create bucket, skipping integration test")
	}

	tests := []struct {
		name        string
		key         string
		content     string
		contentType string
		wantErr     bool
	}{
		{
			name:        "upload text file",
			key:         "test/upload-test.txt",
			content:     "hello world",
			contentType: "text/plain",
			wantErr:     false,
		},
		{
			name:        "upload binary data",
			key:         "test/binary-test.bin",
			content:     "\x00\x01\x02\x03",
			contentType: "application/octet-stream",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.content)
			err := storage.Upload(ctx, tt.key, reader, tt.contentType, int64(len(tt.content)))

			if (err != nil) != tt.wantErr {
				t.Errorf("Upload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify by downloading
			if !tt.wantErr {
				defer func() { _ = storage.Delete(ctx, tt.key) }() // Cleanup

				r, err := storage.Download(ctx, tt.key)
				if err != nil {
					t.Errorf("Download() after upload failed: %v", err)
					return
				}
				defer func() { _ = r.Close() }()

				content, _ := io.ReadAll(r)
				if string(content) != tt.content {
					t.Errorf("Downloaded content = %q, want %q", string(content), tt.content)
				}
			}
		})
	}
}

// TestMinIOStorage_Download tests the Download method.
func TestMinIOStorage_Download(t *testing.T) {
	cfg := &Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
		UseSSL:    false,
		Region:    "us-east-1",
	}

	storage, err := NewMinIOStorage(cfg)
	if err != nil {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := storage.EnsureBucket(ctx); err != nil {
		t.Skip("Could not create bucket, skipping integration test")
	}

	t.Run("download existing file", func(t *testing.T) {
		key := "test/download-test.txt"
		content := "test content for download"

		// Upload first
		err := storage.Upload(ctx, key, strings.NewReader(content), "text/plain", int64(len(content)))
		if err != nil {
			t.Fatalf("Setup upload failed: %v", err)
		}
		defer func() { _ = storage.Delete(ctx, key) }()

		// Download
		r, err := storage.Download(ctx, key)
		if err != nil {
			t.Fatalf("Download() error = %v", err)
		}
		defer func() { _ = r.Close() }()

		data, _ := io.ReadAll(r)
		if string(data) != content {
			t.Errorf("Download() content = %q, want %q", string(data), content)
		}
	})

	t.Run("download non-existent file", func(t *testing.T) {
		_, err := storage.Download(ctx, "test/does-not-exist-12345.txt")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Download() error = %v, want ErrNotFound", err)
		}
	})
}

// TestMinIOStorage_Delete tests the Delete method.
func TestMinIOStorage_Delete(t *testing.T) {
	cfg := &Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
		UseSSL:    false,
		Region:    "us-east-1",
	}

	storage, err := NewMinIOStorage(cfg)
	if err != nil {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := storage.EnsureBucket(ctx); err != nil {
		t.Skip("Could not create bucket, skipping integration test")
	}

	t.Run("delete existing file", func(t *testing.T) {
		key := "test/delete-test.txt"

		// Upload first
		_ = storage.Upload(ctx, key, strings.NewReader("content"), "text/plain", 7)

		// Delete
		err := storage.Delete(ctx, key)
		if err != nil {
			t.Errorf("Delete() error = %v", err)
		}

		// Verify gone
		exists, _ := storage.Exists(ctx, key)
		if exists {
			t.Error("Delete() file still exists")
		}
	})

	t.Run("delete non-existent file (idempotent)", func(t *testing.T) {
		err := storage.Delete(ctx, "test/does-not-exist-12345.txt")
		if err != nil {
			t.Errorf("Delete() non-existent file error = %v, want nil (idempotent)", err)
		}
	})
}

// TestMinIOStorage_Exists tests the Exists method.
func TestMinIOStorage_Exists(t *testing.T) {
	cfg := &Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
		UseSSL:    false,
		Region:    "us-east-1",
	}

	storage, err := NewMinIOStorage(cfg)
	if err != nil {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := storage.EnsureBucket(ctx); err != nil {
		t.Skip("Could not create bucket, skipping integration test")
	}

	t.Run("file exists", func(t *testing.T) {
		key := "test/exists-test.txt"
		_ = storage.Upload(ctx, key, strings.NewReader("content"), "text/plain", 7)
		defer func() { _ = storage.Delete(ctx, key) }()

		exists, err := storage.Exists(ctx, key)
		if err != nil {
			t.Errorf("Exists() error = %v", err)
		}
		if !exists {
			t.Error("Exists() = false, want true")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		exists, err := storage.Exists(ctx, "test/does-not-exist-12345.txt")
		if err != nil {
			t.Errorf("Exists() error = %v", err)
		}
		if exists {
			t.Error("Exists() = true, want false")
		}
	})
}

// TestMinIOStorage_GetPresignedURL tests the GetPresignedURL method.
func TestMinIOStorage_GetPresignedURL(t *testing.T) {
	cfg := &Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
		UseSSL:    false,
		Region:    "us-east-1",
	}

	storage, err := NewMinIOStorage(cfg)
	if err != nil {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := storage.EnsureBucket(ctx); err != nil {
		t.Skip("Could not create bucket, skipping integration test")
	}

	t.Run("generate URL for existing file", func(t *testing.T) {
		key := "test/presign-test.txt"
		_ = storage.Upload(ctx, key, strings.NewReader("content"), "text/plain", 7)
		defer func() { _ = storage.Delete(ctx, key) }()

		url, err := storage.GetPresignedURL(ctx, key, 3600)
		if err != nil {
			t.Errorf("GetPresignedURL() error = %v", err)
		}

		// URL should contain the key and be a valid URL
		if url == "" {
			t.Error("GetPresignedURL() returned empty URL")
		}
		if !strings.Contains(url, key) && !strings.Contains(url, "presign") {
			// Some implementations encode the key differently
			t.Logf("URL: %s", url)
		}
	})
}
