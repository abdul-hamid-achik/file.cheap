package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestLocalStorage(t *testing.T) *LocalStorage {
	t.Helper()
	dir := t.TempDir()
	s, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("NewLocalStorage(%q): %v", dir, err)
	}
	return s
}

func TestLocalStorage_UploadDownloadRoundTrip(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()
	content := []byte("hello, file.cheap!")

	err := s.Upload(ctx, "test/greeting.txt", bytes.NewReader(content), "text/plain", int64(len(content)))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	rc, err := s.Download(ctx, "test/greeting.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestLocalStorage_UploadOverwrite(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	err := s.Upload(ctx, "file.txt", strings.NewReader("v1"), "text/plain", 2)
	if err != nil {
		t.Fatalf("Upload v1: %v", err)
	}

	err = s.Upload(ctx, "file.txt", strings.NewReader("v2"), "text/plain", 2)
	if err != nil {
		t.Fatalf("Upload v2: %v", err)
	}

	rc, err := s.Download(ctx, "file.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()

	got, _ := io.ReadAll(rc)
	if string(got) != "v2" {
		t.Errorf("expected v2, got %q", got)
	}
}

func TestLocalStorage_Delete(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	err := s.Upload(ctx, "to-delete.txt", strings.NewReader("bye"), "text/plain", 3)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	err = s.Delete(ctx, "to-delete.txt")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, err := s.Exists(ctx, "to-delete.txt")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("file still exists after Delete")
	}
}

func TestLocalStorage_DeleteNotFound(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	err := s.Delete(ctx, "nonexistent.txt")
	if err != ErrNotFound {
		t.Errorf("Delete nonexistent: got %v, want ErrNotFound", err)
	}
}

func TestLocalStorage_DownloadNotFound(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	_, err := s.Download(ctx, "nonexistent.txt")
	if err != ErrNotFound {
		t.Errorf("Download nonexistent: got %v, want ErrNotFound", err)
	}
}

func TestLocalStorage_Exists(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	exists, err := s.Exists(ctx, "nope.txt")
	if err != nil {
		t.Fatalf("Exists before upload: %v", err)
	}
	if exists {
		t.Error("Exists returned true for missing file")
	}

	err = s.Upload(ctx, "yep.txt", strings.NewReader("here"), "text/plain", 4)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	exists, err = s.Exists(ctx, "yep.txt")
	if err != nil {
		t.Fatalf("Exists after upload: %v", err)
	}
	if !exists {
		t.Error("Exists returned false for existing file")
	}
}

func TestLocalStorage_GetPresignedURL(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	url, err := s.GetPresignedURL(ctx, "some/file.txt", 3600)
	if err != nil {
		t.Fatalf("GetPresignedURL: %v", err)
	}

	expected := "file://" + filepath.Join(s.rootDir, "some", "file.txt")
	if url != expected {
		t.Errorf("URL = %q, want %q", url, expected)
	}
}

func TestLocalStorage_PathTraversal(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	traversalKeys := []string{
		"../etc/passwd",
		"foo/../../etc/passwd",
		"foo/../bar/../../../etc/shadow",
		"..\\windows\\system32",
	}

	for _, key := range traversalKeys {
		t.Run("Upload_"+key, func(t *testing.T) {
			err := s.Upload(ctx, key, strings.NewReader("x"), "text/plain", 1)
			if err != ErrInvalidKey {
				t.Errorf("Upload(%q): got %v, want ErrInvalidKey", key, err)
			}
		})

		t.Run("Download_"+key, func(t *testing.T) {
			_, err := s.Download(ctx, key)
			if err != ErrInvalidKey {
				t.Errorf("Download(%q): got %v, want ErrInvalidKey", key, err)
			}
		})

		t.Run("Delete_"+key, func(t *testing.T) {
			err := s.Delete(ctx, key)
			if err != ErrInvalidKey {
				t.Errorf("Delete(%q): got %v, want ErrInvalidKey", key, err)
			}
		})

		t.Run("Exists_"+key, func(t *testing.T) {
			_, err := s.Exists(ctx, key)
			if err != ErrInvalidKey {
				t.Errorf("Exists(%q): got %v, want ErrInvalidKey", key, err)
			}
		})
	}
}

func TestLocalStorage_AbsolutePathRejected(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	err := s.Upload(ctx, "/etc/passwd", strings.NewReader("x"), "text/plain", 1)
	if err != ErrInvalidKey {
		t.Errorf("Upload absolute path: got %v, want ErrInvalidKey", err)
	}
}

func TestLocalStorage_EmptyKeyRejected(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	err := s.Upload(ctx, "", strings.NewReader("x"), "text/plain", 1)
	if err != ErrInvalidKey {
		t.Errorf("Upload empty key: got %v, want ErrInvalidKey", err)
	}
}

func TestLocalStorage_HealthCheck_ValidDir(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	err := s.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck on valid dir: %v", err)
	}
}

func TestLocalStorage_HealthCheck_InvalidDir(t *testing.T) {
	s := &LocalStorage{rootDir: "/nonexistent/path/that/does/not/exist"}
	ctx := context.Background()

	err := s.HealthCheck(ctx)
	if err == nil {
		t.Error("HealthCheck on invalid dir: expected error, got nil")
	}
}

func TestLocalStorage_HealthCheck_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &LocalStorage{rootDir: filePath}
	ctx := context.Background()

	err := s.HealthCheck(ctx)
	if err == nil {
		t.Error("HealthCheck on file: expected error, got nil")
	}
}

func TestLocalStorage_NestedDirectoryUpload(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx := context.Background()

	key := "a/b/c/d/deep.txt"
	err := s.Upload(ctx, key, strings.NewReader("deep"), "text/plain", 4)
	if err != nil {
		t.Fatalf("Upload nested: %v", err)
	}

	exists, err := s.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("nested file not found after upload")
	}
}

func TestLocalStorage_CancelledContext(t *testing.T) {
	s := newTestLocalStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Upload(ctx, "file.txt", strings.NewReader("x"), "text/plain", 1)
	if err == nil {
		t.Error("Upload with cancelled context: expected error")
	}

	_, err = s.Download(ctx, "file.txt")
	if err == nil {
		t.Error("Download with cancelled context: expected error")
	}

	err = s.Delete(ctx, "file.txt")
	if err == nil {
		t.Error("Delete with cancelled context: expected error")
	}

	_, err = s.Exists(ctx, "file.txt")
	if err == nil {
		t.Error("Exists with cancelled context: expected error")
	}

	_, err = s.GetPresignedURL(ctx, "file.txt", 60)
	if err == nil {
		t.Error("GetPresignedURL with cancelled context: expected error")
	}

	err = s.HealthCheck(ctx)
	if err == nil {
		t.Error("HealthCheck with cancelled context: expected error")
	}
}

func TestNewLocalStorage_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "nested", "dir")
	s, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	info, err := os.Stat(s.rootDir)
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !info.IsDir() {
		t.Error("root is not a directory")
	}
}
