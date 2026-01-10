//go:build integration

package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/client"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/config"
)

// Integration tests require FC_API_KEY and optionally FC_BASE_URL to be set.
// Run with: go test ./internal/fc/cli -tags=integration -v

func skipIfNoAPIKey(t *testing.T) {
	if os.Getenv("FC_API_KEY") == "" {
		t.Skip("FC_API_KEY not set, skipping integration test")
	}
}

func getTestClient(t *testing.T) *client.Client {
	apiKey := os.Getenv("FC_API_KEY")
	baseURL := os.Getenv("FC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://file.cheap"
	}
	return client.New(baseURL, apiKey)
}

func TestIntegration_UploadDownloadDelete(t *testing.T) {
	skipIfNoAPIKey(t)
	c := getTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-image.jpg")

	// Create a minimal valid JPEG (smallest valid JPEG is about 125 bytes)
	jpegData := createMinimalJPEG()
	if err := os.WriteFile(testFile, jpegData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test Upload
	t.Run("Upload", func(t *testing.T) {
		result, err := c.Upload(ctx, testFile, nil, false)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		if result.ID == "" {
			t.Error("Expected file ID in response")
		}
		t.Logf("Uploaded file with ID: %s", result.ID)

		// Store file ID for subsequent tests
		t.Cleanup(func() {
			// Clean up: delete the uploaded file
			if err := c.DeleteFile(ctx, result.ID); err != nil {
				t.Logf("Warning: failed to delete test file: %v", err)
			}
		})

		// Test GetFile
		t.Run("GetFile", func(t *testing.T) {
			file, err := c.GetFile(ctx, result.ID)
			if err != nil {
				t.Fatalf("GetFile failed: %v", err)
			}
			if file.ID != result.ID {
				t.Errorf("Expected file ID %s, got %s", result.ID, file.ID)
			}
		})

		// Test Download
		t.Run("Download", func(t *testing.T) {
			reader, filename, err := c.Download(ctx, result.ID, "")
			if err != nil {
				t.Fatalf("Download failed: %v", err)
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("Failed to read download: %v", err)
			}
			if len(data) == 0 {
				t.Error("Downloaded file is empty")
			}
			t.Logf("Downloaded %d bytes, filename: %s", len(data), filename)
		})

		// Test ListFiles
		t.Run("ListFiles", func(t *testing.T) {
			list, err := c.ListFiles(ctx, 10, 0, "", "")
			if err != nil {
				t.Fatalf("ListFiles failed: %v", err)
			}
			if list.Total == 0 {
				t.Error("Expected at least one file in list")
			}
			t.Logf("Found %d files", list.Total)
		})
	})
}

func TestIntegration_UploadWithTransforms(t *testing.T) {
	skipIfNoAPIKey(t)
	c := getTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-image.jpg")

	jpegData := createMinimalJPEG()
	if err := os.WriteFile(testFile, jpegData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, err := c.Upload(ctx, testFile, []string{"thumbnail"}, false)
	if err != nil {
		t.Fatalf("Upload with transforms failed: %v", err)
	}
	if result.ID == "" {
		t.Error("Expected file ID in response")
	}
	t.Logf("Uploaded file with ID: %s, transforms queued", result.ID)

	// Clean up
	t.Cleanup(func() {
		if err := c.DeleteFile(ctx, result.ID); err != nil {
			t.Logf("Warning: failed to delete test file: %v", err)
		}
	})
}

func TestIntegration_UploadReader(t *testing.T) {
	skipIfNoAPIKey(t)
	c := getTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	jpegData := createMinimalJPEG()
	reader := bytes.NewReader(jpegData)

	result, err := c.UploadReader(ctx, reader, "stream-test.jpg", int64(len(jpegData)), nil, false)
	if err != nil {
		t.Fatalf("UploadReader failed: %v", err)
	}
	if result.ID == "" {
		t.Error("Expected file ID in response")
	}
	t.Logf("Uploaded via reader with ID: %s", result.ID)

	// Clean up
	t.Cleanup(func() {
		if err := c.DeleteFile(ctx, result.ID); err != nil {
			t.Logf("Warning: failed to delete test file: %v", err)
		}
	})
}

func TestIntegration_ConfigEnvOverride(t *testing.T) {
	// Test that environment variables override config file values
	originalKey := os.Getenv("FC_API_KEY")
	originalURL := os.Getenv("FC_BASE_URL")

	// Set test values
	os.Setenv("FC_API_KEY", "test_env_key_12345")
	os.Setenv("FC_BASE_URL", "https://test.example.com")
	defer func() {
		if originalKey != "" {
			os.Setenv("FC_API_KEY", originalKey)
		} else {
			os.Unsetenv("FC_API_KEY")
		}
		if originalURL != "" {
			os.Setenv("FC_BASE_URL", originalURL)
		} else {
			os.Unsetenv("FC_BASE_URL")
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.APIKey != "test_env_key_12345" {
		t.Errorf("Expected API key from env, got: %s", cfg.APIKey)
	}
	if cfg.BaseURL != "https://test.example.com" {
		t.Errorf("Expected base URL from env, got: %s", cfg.BaseURL)
	}
}

func TestIntegration_WaitForFile(t *testing.T) {
	skipIfNoAPIKey(t)
	c := getTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create and upload a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "wait-test.jpg")

	jpegData := createMinimalJPEG()
	if err := os.WriteFile(testFile, jpegData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, err := c.Upload(ctx, testFile, nil, false)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	t.Logf("Uploaded file with ID: %s, waiting for completion...", result.ID)

	// Clean up
	t.Cleanup(func() {
		if err := c.DeleteFile(ctx, result.ID); err != nil {
			t.Logf("Warning: failed to delete test file: %v", err)
		}
	})

	// Wait for file processing
	file, err := c.WaitForFile(ctx, result.ID, 2*time.Second, 2*time.Minute)
	if err != nil {
		t.Fatalf("WaitForFile failed: %v", err)
	}

	if file.Status != "completed" && file.Status != "failed" {
		t.Errorf("Expected completed or failed status, got: %s", file.Status)
	}
	t.Logf("File status: %s", file.Status)
}

// createMinimalJPEG creates the smallest valid JPEG image possible
func createMinimalJPEG() []byte {
	// This is a minimal 1x1 white JPEG image
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
		0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
		0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
		0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
		0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
		0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
		0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
		0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
		0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
		0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
		0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
		0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
		0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
		0x00, 0x00, 0x3F, 0x00, 0xFB, 0xD5, 0xDB, 0x20, 0xA8, 0xF1, 0x4F, 0xFF,
		0xD9,
	}
}
