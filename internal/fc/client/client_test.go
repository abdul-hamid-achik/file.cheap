package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := New("https://file.cheap", "fp_test123")
	if c.baseURL != "https://file.cheap" {
		t.Errorf("baseURL = %s, want https://file.cheap", c.baseURL)
	}
	if c.apiKey != "fp_test123" {
		t.Errorf("apiKey = %s, want fp_test123", c.apiKey)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := New("https://file.cheap/", "fp_test123")
	if c.baseURL != "https://file.cheap" {
		t.Errorf("baseURL = %s, want https://file.cheap (without trailing slash)", c.baseURL)
	}
}

func TestClient_ListFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer fp_test123" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		json.NewEncoder(w).Encode(ListFilesResponse{
			Files: []File{
				{ID: "abc123", Filename: "test.jpg", Status: "completed"},
			},
			Total:   1,
			HasMore: false,
		})
	}))
	defer server.Close()

	c := New(server.URL, "fp_test123")
	resp, err := c.ListFiles(context.Background(), 20, 0, "", "")
	if err != nil {
		t.Fatalf("ListFiles error = %v", err)
	}

	if len(resp.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(resp.Files))
	}
	if resp.Files[0].ID != "abc123" {
		t.Errorf("Files[0].ID = %s, want abc123", resp.Files[0].ID)
	}
}

func TestClient_GetFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files/abc123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(File{
			ID:       "abc123",
			Filename: "test.jpg",
			Status:   "completed",
			Variants: []Variant{
				{ID: "v1", VariantType: "thumbnail", Width: 300, Height: 300},
			},
		})
	}))
	defer server.Close()

	c := New(server.URL, "fp_test123")
	file, err := c.GetFile(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetFile error = %v", err)
	}

	if file.ID != "abc123" {
		t.Errorf("ID = %s, want abc123", file.ID)
	}
	if len(file.Variants) != 1 {
		t.Errorf("len(Variants) = %d, want 1", len(file.Variants))
	}
}

func TestClient_DeleteFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/files/abc123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "fp_test123")
	err := c.DeleteFile(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("DeleteFile error = %v", err)
	}
}

func TestClient_Transform(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/files/abc123/transform" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req TransformRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Presets) == 0 || req.Presets[0] != "thumbnail" {
			t.Errorf("unexpected presets: %v", req.Presets)
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(TransformResponse{
			FileID: "abc123",
			Jobs:   []string{"job1", "job2"},
		})
	}))
	defer server.Close()

	c := New(server.URL, "fp_test123")
	resp, err := c.Transform(context.Background(), "abc123", &TransformRequest{
		Presets: []string{"thumbnail", "webp"},
	})
	if err != nil {
		t.Fatalf("Transform error = %v", err)
	}

	if resp.FileID != "abc123" {
		t.Errorf("FileID = %s, want abc123", resp.FileID)
	}
	if len(resp.Jobs) != 2 {
		t.Errorf("len(Jobs) = %d, want 2", len(resp.Jobs))
	}
}

func TestClient_BatchTransform(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/batch/transform" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(BatchTransformResponse{
			BatchID:    "batch123",
			TotalFiles: 3,
			TotalJobs:  9,
			Status:     "pending",
		})
	}))
	defer server.Close()

	c := New(server.URL, "fp_test123")
	resp, err := c.BatchTransform(context.Background(), &BatchTransformRequest{
		FileIDs: []string{"a", "b", "c"},
		Presets: []string{"thumbnail"},
	})
	if err != nil {
		t.Fatalf("BatchTransform error = %v", err)
	}

	if resp.BatchID != "batch123" {
		t.Errorf("BatchID = %s, want batch123", resp.BatchID)
	}
}

func TestClient_Upload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/upload" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("failed to get file: %v", err)
		}
		defer file.Close()

		if header.Filename != "test.txt" {
			t.Errorf("filename = %s, want test.txt", header.Filename)
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(UploadResponse{
			ID:       "uploaded123",
			Filename: "test.txt",
			Status:   "pending",
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	c := New(server.URL, "fp_test123")
	resp, err := c.Upload(context.Background(), testFile, nil, false)
	if err != nil {
		t.Fatalf("Upload error = %v", err)
	}

	if resp.ID != "uploaded123" {
		t.Errorf("ID = %s, want uploaded123", resp.ID)
	}
}

func TestClient_DeviceAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/device" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(DeviceAuthResponse{
			DeviceCode:      "dev123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://file.cheap/auth/device",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer server.Close()

	c := New(server.URL, "")
	resp, err := c.DeviceAuth(context.Background())
	if err != nil {
		t.Fatalf("DeviceAuth error = %v", err)
	}

	if resp.UserCode != "ABCD-1234" {
		t.Errorf("UserCode = %s, want ABCD-1234", resp.UserCode)
	}
}

func TestClient_WaitForFile(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		status := "processing"
		if calls >= 3 {
			status = "completed"
		}
		json.NewEncoder(w).Encode(File{
			ID:     "abc123",
			Status: status,
		})
	}))
	defer server.Close()

	c := New(server.URL, "fp_test123")
	file, err := c.WaitForFile(context.Background(), "abc123", 10*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForFile error = %v", err)
	}

	if file.Status != "completed" {
		t.Errorf("Status = %s, want completed", file.Status)
	}
	if calls < 3 {
		t.Errorf("Expected at least 3 API calls, got %d", calls)
	}
}

func TestClient_ErrorParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: APIError{
				Code:    "not_found",
				Message: "File not found",
			},
		})
	}))
	defer server.Close()

	c := New(server.URL, "fp_test123")
	_, err := c.GetFile(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}
	if err.Error() != "not_found: File not found" {
		t.Errorf("Error = %q, want 'not_found: File not found'", err.Error())
	}
}
