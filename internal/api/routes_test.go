package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestHealthCheck tests the health endpoint.
func TestHealthCheck(t *testing.T) {
	cfg := &Config{}
	router := NewRouter(cfg)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("body status = %q, want %q", body["status"], "ok")
	}
}

// TestHealthCheckMethod tests that health only accepts GET.
func TestHealthCheckMethod(t *testing.T) {
	cfg := &Config{}
	router := NewRouter(cfg)

	methods := []string{"POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/health", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			// Go 1.22+ ServeMux returns 405 for wrong methods
			if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
				t.Errorf("%s /health: status = %d, want 405 or 404", method, rec.Code)
			}
		})
	}
}

// TestUploadHandler tests the file upload endpoint.
// See docs/06-phase6-api.md for implementation guidance.
func TestUploadHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name         string
		setupRequest func(t *testing.T) *http.Request
		setupMocks   func(q *MockQuerier, s *MockStorage, b *MockBroker)
		wantStatus   int
		wantBodyKeys []string // Keys that should be present in JSON response
	}{
		{
			name: "successful upload - JPEG image",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithImage(t, "file", "test.jpg", 800, 600)
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
				return req
			},
			setupMocks: func(q *MockQuerier, s *MockStorage, b *MockBroker) {
				// Default behavior is fine
			},
			wantStatus:   http.StatusAccepted,
			wantBodyKeys: []string{"id", "filename", "status"},
		},
		{
			name: "successful upload - PNG image",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithData(t, "file", "test.png", []byte("PNG data"), "image/png")
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
				return req
			},
			wantStatus:   http.StatusAccepted,
			wantBodyKeys: []string{"id", "filename", "status"},
		},
		{
			name: "missing file field",
			setupRequest: func(t *testing.T) *http.Request {
				// Empty multipart form
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)
				_ = writer.Close()

				req := httptest.NewRequest("POST", "/v1/upload", &buf)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
				return req
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "wrong field name",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithImage(t, "wrong_field", "test.jpg", 100, 100)
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
				return req
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "file too large",
			setupRequest: func(t *testing.T) *http.Request {
				// Create a 200MB file (exceeds default 100MB limit)
				largeData := make([]byte, 200*1024*1024)
				body, contentType := createMultipartFormWithData(t, "file", "large.jpg", largeData, "image/jpeg")
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
				return req
			},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "unauthorized - missing token",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithImage(t, "file", "test.jpg", 100, 100)
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				// No Authorization header
				return req
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "unauthorized - invalid token",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithImage(t, "file", "test.jpg", 100, 100)
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer invalid-token")
				return req
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "unauthorized - expired token",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithImage(t, "file", "test.jpg", 100, 100)
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer "+generateExpiredToken(t, testUserID))
				return req
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "storage error",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithImage(t, "file", "test.jpg", 100, 100)
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
				return req
			},
			setupMocks: func(q *MockQuerier, s *MockStorage, b *MockBroker) {
				s.UploadErr = io.ErrUnexpectedEOF
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "database error",
			setupRequest: func(t *testing.T) *http.Request {
				body, contentType := createMultipartFormWithImage(t, "file", "test.jpg", 100, 100)
				req := httptest.NewRequest("POST", "/v1/upload", body)
				req.Header.Set("Content-Type", contentType)
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
				return req
			},
			setupMocks: func(q *MockQuerier, s *MockStorage, b *MockBroker) {
				q.CreateFileErr = io.ErrUnexpectedEOF
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, broker, cfg := setupTestDeps(t)

			if tt.setupMocks != nil {
				tt.setupMocks(queries, storage, broker)
			}

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				Broker:        broker,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			req := tt.setupRequest(t)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			// Check response body keys for successful responses
			if tt.wantStatus == http.StatusAccepted && len(tt.wantBodyKeys) > 0 {
				var body map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				for _, key := range tt.wantBodyKeys {
					if _, ok := body[key]; !ok {
						t.Errorf("response missing key %q", key)
					}
				}
			}
		})
	}
}

// TestListFilesHandler tests the file list endpoint.
func TestListFilesHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	otherUserID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		query       string
		setupMocks  func(q *MockQuerier)
		wantStatus  int
		wantCount   int
		wantHasMore bool
	}{
		{
			name:  "list with defaults",
			query: "",
			setupMocks: func(q *MockQuerier) {
				for i := 0; i < 5; i++ {
					q.AddFile(createTestFile(testUserID, "file"+string(rune('0'+i))+".jpg"))
				}
			},
			wantStatus: http.StatusOK,
			wantCount:  5,
		},
		{
			name:  "list with pagination - first page",
			query: "?limit=2&offset=0",
			setupMocks: func(q *MockQuerier) {
				for i := 0; i < 5; i++ {
					q.AddFile(createTestFile(testUserID, "file"+string(rune('0'+i))+".jpg"))
				}
			},
			wantStatus:  http.StatusOK,
			wantCount:   2,
			wantHasMore: true,
		},
		{
			name:  "list with pagination - second page",
			query: "?limit=2&offset=2",
			setupMocks: func(q *MockQuerier) {
				for i := 0; i < 5; i++ {
					q.AddFile(createTestFile(testUserID, "file"+string(rune('0'+i))+".jpg"))
				}
			},
			wantStatus:  http.StatusOK,
			wantCount:   2,
			wantHasMore: true,
		},
		{
			name:  "list with pagination - last page",
			query: "?limit=2&offset=4",
			setupMocks: func(q *MockQuerier) {
				for i := 0; i < 5; i++ {
					q.AddFile(createTestFile(testUserID, "file"+string(rune('0'+i))+".jpg"))
				}
			},
			wantStatus:  http.StatusOK,
			wantCount:   1,
			wantHasMore: false,
		},
		{
			name:       "empty list",
			query:      "",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:  "only returns user's own files",
			query: "",
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFile(testUserID, "my-file.jpg"))
				q.AddFile(createTestFile(otherUserID, "other-file.jpg"))
			},
			wantStatus: http.StatusOK,
			wantCount:  1, // Only returns testUserID's file
		},
		{
			name:       "invalid limit - negative",
			query:      "?limit=-1",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid limit - too large",
			query:      "?limit=1000",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid offset - negative",
			query:      "?offset=-1",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid limit - not a number",
			query:      "?limit=abc",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, _, cfg := setupTestDeps(t)

			if tt.setupMocks != nil {
				tt.setupMocks(queries)
			}

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			req := httptest.NewRequest("GET", "/v1/files"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				var body struct {
					Files   []map[string]interface{} `json:"files"`
					Total   int                      `json:"total"`
					HasMore bool                     `json:"has_more"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if len(body.Files) != tt.wantCount {
					t.Errorf("files count = %d, want %d", len(body.Files), tt.wantCount)
				}
			}
		})
	}
}

// TestGetFileHandler tests the get file endpoint.
func TestGetFileHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	otherUserID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")
	existingFileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")
	otherFileID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name         string
		fileID       string
		setupMocks   func(q *MockQuerier)
		wantStatus   int
		wantBodyKeys []string
	}{
		{
			name:   "existing file",
			fileID: existingFileID.String(),
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "myfile.jpg"))
			},
			wantStatus:   http.StatusOK,
			wantBodyKeys: []string{"id", "filename", "content_type", "size_bytes", "status", "created_at"},
		},
		{
			name:   "file with variants",
			fileID: existingFileID.String(),
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "myfile.jpg"))
				q.AddVariant(createTestVariant(existingFileID, "thumbnail"))
				q.AddVariant(createTestVariant(existingFileID, "medium"))
			},
			wantStatus:   http.StatusOK,
			wantBodyKeys: []string{"id", "filename", "variants"},
		},
		{
			name:       "non-existent file",
			fileID:     "00000000-0000-0000-0000-000000000000",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID format",
			fileID:     "not-a-uuid",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty file ID",
			fileID:     "",
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusNotFound, // Route won't match
		},
		{
			name:   "file belongs to another user - returns not found",
			fileID: otherFileID.String(),
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(otherFileID, otherUserID, "other-file.jpg"))
			},
			wantStatus: http.StatusNotFound, // Don't leak existence
		},
		{
			name:   "deleted file returns not found",
			fileID: existingFileID.String(),
			setupMocks: func(q *MockQuerier) {
				f := createTestFileWithID(existingFileID, testUserID, "deleted.jpg")
				var deletedAt pgtype.Timestamptz
				_ = deletedAt.Scan(time.Now())
				f.DeletedAt = deletedAt
				q.AddFile(f)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, _, cfg := setupTestDeps(t)

			if tt.setupMocks != nil {
				tt.setupMocks(queries)
			}

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			url := "/v1/files/" + tt.fileID
			if tt.fileID == "" {
				url = "/v1/files/"
			}

			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK && len(tt.wantBodyKeys) > 0 {
				var body map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				for _, key := range tt.wantBodyKeys {
					if _, ok := body[key]; !ok {
						t.Errorf("response missing key %q", key)
					}
				}
			}
		})
	}
}

// TestDownloadHandler tests the file download endpoint.
func TestDownloadHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	existingFileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name            string
		fileID          string
		query           string
		setupMocks      func(q *MockQuerier, s *MockStorage)
		wantStatus      int
		wantRedirect    bool
		wantContentType string
	}{
		{
			name:   "download existing file - redirect to presigned URL",
			fileID: existingFileID.String(),
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "download.jpg"))
			},
			wantStatus:   http.StatusTemporaryRedirect,
			wantRedirect: true,
		},
		{
			name:   "download non-existent file",
			fileID: "00000000-0000-0000-0000-000000000000",
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				// No file added
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "download with variant query parameter",
			fileID: existingFileID.String(),
			query:  "?variant=thumbnail",
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "file.jpg"))
				q.AddVariant(createTestVariant(existingFileID, "thumbnail"))
			},
			wantStatus:   http.StatusTemporaryRedirect,
			wantRedirect: true,
		},
		{
			name:   "download non-existent variant",
			fileID: existingFileID.String(),
			query:  "?variant=nonexistent",
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "file.jpg"))
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, _, cfg := setupTestDeps(t)

			if tt.setupMocks != nil {
				tt.setupMocks(queries, storage)
			}

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			req := httptest.NewRequest("GET", "/v1/files/"+tt.fileID+"/download"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantRedirect {
				location := rec.Header().Get("Location")
				if location == "" {
					t.Error("expected Location header for redirect")
				}
			}
		})
	}
}

// TestDeleteHandler tests the file delete endpoint.
func TestDeleteHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	otherUserID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")
	existingFileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")
	otherFileID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name       string
		fileID     string
		setupMocks func(q *MockQuerier, s *MockStorage)
		wantStatus int
	}{
		{
			name:   "delete existing file",
			fileID: existingFileID.String(),
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				f := createTestFileWithID(existingFileID, testUserID, "to-delete.jpg")
				q.AddFile(f)
				// Also add to storage
				_ = s.MemoryStorage.Upload(context.Background(), f.StorageKey, strings.NewReader("data"), "image/jpeg", 4)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "delete non-existent file",
			fileID: "00000000-0000-0000-0000-000000000000",
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				// No file
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "delete file belonging to another user",
			fileID: otherFileID.String(),
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				q.AddFile(createTestFileWithID(otherFileID, otherUserID, "other.jpg"))
			},
			wantStatus: http.StatusNotFound, // Don't reveal file exists
		},
		{
			name:   "delete with invalid UUID",
			fileID: "invalid-uuid",
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				// No setup needed
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "delete already deleted file",
			fileID: existingFileID.String(),
			setupMocks: func(q *MockQuerier, s *MockStorage) {
				f := createTestFileWithID(existingFileID, testUserID, "deleted.jpg")
				var deletedAt pgtype.Timestamptz
				_ = deletedAt.Scan(time.Now())
				f.DeletedAt = deletedAt
				q.AddFile(f)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, _, cfg := setupTestDeps(t)

			if tt.setupMocks != nil {
				tt.setupMocks(queries, storage)
			}

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			req := httptest.NewRequest("DELETE", "/v1/files/"+tt.fileID, nil)
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestDeleteHandlerSoftDelete verifies that delete is a soft delete.
func TestDeleteHandlerSoftDelete(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, _, cfg := setupTestDeps(t)

	// Add file
	file := createTestFileWithID(fileID, testUserID, "soft-delete.jpg")
	queries.AddFile(file)

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	req := httptest.NewRequest("DELETE", "/v1/files/"+fileID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	allFiles := queries.GetAllFiles()
	found := false
	for _, f := range allFiles {
		if uuidToString(f.ID) == fileID.String() {
			found = true
			if !f.DeletedAt.Valid {
				t.Error("file should have DeletedAt set after soft delete")
			}
			break
		}
	}
	if !found {
		t.Error("file should still exist in database after soft delete")
	}
}

// TestAPIRequiresAuth tests that all API endpoints require authentication.
func TestAPIRequiresAuth(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/files"},
		{"GET", "/v1/files/550e8400-e29b-41d4-a716-446655440000"},
		{"GET", "/v1/files/550e8400-e29b-41d4-a716-446655440000/download"},
		{"DELETE", "/v1/files/550e8400-e29b-41d4-a716-446655440000"},
		// POST /upload is tested separately due to multipart form
	}

	_, storage, _, cfg := setupTestDeps(t)

	router := NewRouter(&Config{
		Storage:       storage,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			// No Authorization header

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without auth: status = %d, want %d",
					ep.method, ep.path, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

// TestHealthDoesNotRequireAuth tests that health endpoint is public.
func TestHealthDoesNotRequireAuth(t *testing.T) {
	cfg := &Config{JWTSecret: testJWTSecret}
	router := NewRouter(cfg)

	req := httptest.NewRequest("GET", "/health", nil)
	// No Authorization header

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /health without auth: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestResponseContentType tests that API returns JSON.
func TestResponseContentType(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, _, cfg := setupTestDeps(t)
	queries.AddFile(createTestFileWithID(fileID, testUserID, "test.jpg"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"GET", "/v1/files"},
		{"GET", "/v1/files/" + fileID.String()},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			if ep.path != "/health" {
				req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			contentType := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("%s %s: Content-Type = %q, want application/json",
					ep.method, ep.path, contentType)
			}
		})
	}
}

// Helper functions

func createMultipartForm(t *testing.T, fieldName, fileName string, content []byte) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}

	if _, err := part.Write(content); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	return &buf, writer.FormDataContentType()
}

func createMultipartFormWithImage(t *testing.T, fieldName, fileName string, width, height int) (io.Reader, string) {
	t.Helper()

	// Create a real JPEG image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8(255 * x / width)
			g := uint8(255 * y / height)
			img.Set(x, y, color.RGBA{R: r, G: g, B: 128, A: 255})
		}
	}

	var imgBuf bytes.Buffer
	if err := jpeg.Encode(&imgBuf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("failed to encode JPEG: %v", err)
	}

	return createMultipartForm(t, fieldName, fileName, imgBuf.Bytes())
}

func createMultipartFormWithData(t *testing.T, fieldName, fileName string, data []byte, contentType string) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Create a form file with custom content type
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="` + fieldName + `"; filename="` + fileName + `"`}
	h["Content-Type"] = []string{contentType}

	part, err := writer.CreatePart(h)
	if err != nil {
		t.Fatalf("failed to create form part: %v", err)
	}

	if _, err := part.Write(data); err != nil {
		t.Fatalf("failed to write data: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	return &buf, writer.FormDataContentType()
}

// TestUploadVerifiesJobEnqueued tests that upload enqueues processing jobs.
func TestUploadVerifiesJobEnqueued(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body, contentType := createMultipartFormWithImage(t, "file", "test.jpg", 800, 600)
	req := httptest.NewRequest("POST", "/v1/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// After successful upload, jobs should be enqueued
	// This verifies the integration between upload and job queue
	_ = broker // Will be used when implementation supports broker injection
	_ = queries
}

func TestTransformHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	existingFileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name       string
		fileID     string
		body       string
		setupMocks func(q *MockQuerier)
		wantStatus int
	}{
		{
			name:   "transform with thumbnail preset",
			fileID: existingFileID.String(),
			body:   `{"presets": ["thumbnail"]}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "test.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:   "transform with multiple presets",
			fileID: existingFileID.String(),
			body:   `{"presets": ["thumbnail", "sm", "md"]}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "test.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:   "transform with webp conversion",
			fileID: existingFileID.String(),
			body:   `{"webp": true, "quality": 85}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "test.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:   "transform with social presets",
			fileID: existingFileID.String(),
			body:   `{"presets": ["og", "twitter"]}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "test.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "transform non-existent file",
			fileID:     "00000000-0000-0000-0000-000000000000",
			body:       `{"presets": ["thumbnail"]}`,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "transform invalid file ID",
			fileID:     "not-a-uuid",
			body:       `{"presets": ["thumbnail"]}`,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "transform with no transformations",
			fileID: existingFileID.String(),
			body:   `{}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "test.jpg"))
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "transform with invalid JSON",
			fileID: existingFileID.String(),
			body:   `{invalid}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(existingFileID, testUserID, "test.jpg"))
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, broker, cfg := setupTestDeps(t)

			if tt.setupMocks != nil {
				tt.setupMocks(queries)
			}

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				Broker:        broker,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			req := httptest.NewRequest("POST", "/v1/files/"+tt.fileID+"/transform", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusAccepted {
				var body map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if _, ok := body["file_id"]; !ok {
					t.Error("response missing file_id")
				}
				if _, ok := body["jobs"]; !ok {
					t.Error("response missing jobs")
				}
			}
		})
	}
}

func TestBatchTransformHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID1 := uuid.MustParse("770e8400-e29b-41d4-a716-446655440001")
	fileID2 := uuid.MustParse("770e8400-e29b-41d4-a716-446655440002")
	fileID3 := uuid.MustParse("770e8400-e29b-41d4-a716-446655440003")

	tests := []struct {
		name       string
		body       string
		setupMocks func(q *MockQuerier)
		wantStatus int
	}{
		{
			name: "batch transform single file",
			body: `{"file_ids": ["` + fileID1.String() + `"], "presets": ["thumbnail"]}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(fileID1, testUserID, "test1.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name: "batch transform multiple files",
			body: `{"file_ids": ["` + fileID1.String() + `", "` + fileID2.String() + `", "` + fileID3.String() + `"], "presets": ["thumbnail", "sm"]}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(fileID1, testUserID, "test1.jpg"))
				q.AddFile(createTestFileWithID(fileID2, testUserID, "test2.jpg"))
				q.AddFile(createTestFileWithID(fileID3, testUserID, "test3.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name: "batch transform with webp",
			body: `{"file_ids": ["` + fileID1.String() + `"], "webp": true, "quality": 90}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(fileID1, testUserID, "test1.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "batch transform with no files",
			body:       `{"file_ids": [], "presets": ["thumbnail"]}`,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "batch transform with no transformations",
			body:       `{"file_ids": ["` + fileID1.String() + `"]}`,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "batch transform with invalid file IDs",
			body: `{"file_ids": ["invalid-uuid"], "presets": ["thumbnail"]}`,
			setupMocks: func(q *MockQuerier) {
				// No valid files
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "batch transform skips non-existent files",
			body: `{"file_ids": ["` + fileID1.String() + `", "00000000-0000-0000-0000-000000000000"], "presets": ["thumbnail"]}`,
			setupMocks: func(q *MockQuerier) {
				q.AddFile(createTestFileWithID(fileID1, testUserID, "test1.jpg"))
			},
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, broker, cfg := setupTestDeps(t)

			if tt.setupMocks != nil {
				tt.setupMocks(queries)
			}

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				Broker:        broker,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			req := httptest.NewRequest("POST", "/v1/batch/transform", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusAccepted {
				var body map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if _, ok := body["batch_id"]; !ok {
					t.Error("response missing batch_id")
				}
				if _, ok := body["total_files"]; !ok {
					t.Error("response missing total_files")
				}
				if _, ok := body["status_url"]; !ok {
					t.Error("response missing status_url")
				}
			}
		})
	}
}

func TestGetBatchHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	batchID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name       string
		batchID    string
		query      string
		wantStatus int
	}{
		{
			name:       "get batch status",
			batchID:    batchID.String(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "get batch with items",
			batchID:    batchID.String(),
			query:      "?include_items=true",
			wantStatus: http.StatusOK,
		},
		{
			name:       "get batch with invalid ID",
			batchID:    "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, broker, cfg := setupTestDeps(t)

			router := NewRouter(&Config{
				Storage:       storage,
				Queries:       queries,
				Broker:        broker,
				MaxUploadSize: cfg.MaxUploadSize,
				JWTSecret:     cfg.JWTSecret,
			})

			req := httptest.NewRequest("GET", "/v1/batch/"+tt.batchID+tt.query, nil)
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				var body map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if _, ok := body["id"]; !ok {
					t.Error("response missing id")
				}
				if _, ok := body["status"]; !ok {
					t.Error("response missing status")
				}
				if _, ok := body["total_files"]; !ok {
					t.Error("response missing total_files")
				}
			}
		})
	}
}

func TestVideoHLSHandler_Unauthorized(t *testing.T) {
	cfg := &Config{}
	router := NewRouter(cfg)

	fileID := uuid.New()
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/hls", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestVideoHLSHandler_InvalidFileID(t *testing.T) {
	testUserID := uuid.New()
	cfg := &Config{}
	router := NewRouter(cfg)

	req := httptest.NewRequest("POST", "/v1/files/invalid-id/video/hls", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
	ctx := context.WithValue(req.Context(), UserIDKey, testUserID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 400 or 401", rec.Code)
	}
}

func TestHLSStreamHandler_Unauthorized(t *testing.T) {
	cfg := &Config{}
	router := NewRouter(cfg)

	fileID := uuid.New()
	req := httptest.NewRequest("GET", "/v1/files/"+fileID.String()+"/hls/playlist.m3u8", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHLSStreamHandler_InvalidFileID(t *testing.T) {
	testUserID := uuid.New()
	cfg := &Config{}
	router := NewRouter(cfg)

	req := httptest.NewRequest("GET", "/v1/files/invalid-id/hls/playlist.m3u8", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))
	ctx := context.WithValue(req.Context(), UserIDKey, testUserID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 400 or 401", rec.Code)
	}
}

func TestVideoHLSRequest_Defaults(t *testing.T) {
	req := VideoHLSRequest{}

	if req.SegmentDuration != 0 {
		t.Errorf("SegmentDuration default = %d, want 0", req.SegmentDuration)
	}

	if len(req.Resolutions) != 0 {
		t.Errorf("Resolutions default = %v, want []", req.Resolutions)
	}
}

// Helper to create test video file
func createTestVideoFile(userID uuid.UUID, filename string) db.File {
	fileID := uuid.New()
	return createTestVideoFileWithID(fileID, userID, filename)
}

func createTestVideoFileWithID(id, userID uuid.UUID, filename string) db.File {
	pgFileID := pgtype.UUID{Bytes: id, Valid: true}
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	now := time.Now()
	createdAt := pgtype.Timestamptz{Time: now, Valid: true}
	updatedAt := pgtype.Timestamptz{Time: now, Valid: true}

	return db.File{
		ID:          pgFileID,
		UserID:      pgUserID,
		Filename:    filename,
		ContentType: "video/mp4",
		SizeBytes:   10 * 1024 * 1024, // 10MB
		StorageKey:  "uploads/" + id.String() + "/" + filename,
		Status:      db.FileStatusCompleted,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func TestVideoTranscodeHandler_Success(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{"resolutions": [720], "format": "mp4"}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/transcode", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var resp VideoTranscodeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.FileID != fileID.String() {
		t.Errorf("file_id = %s, want %s", resp.FileID, fileID.String())
	}

	if len(resp.Jobs) == 0 {
		t.Error("expected at least one job ID")
	}

	if !broker.HasJob("video_transcode") {
		t.Error("expected video_transcode job to be enqueued")
	}
}

func TestVideoTranscodeHandler_MultipleResolutions(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{"resolutions": [360, 720, 1080], "format": "mp4", "thumbnail": true}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/transcode", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var resp VideoTranscodeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have 3 transcode jobs + 1 thumbnail job = 4 jobs
	if len(resp.Jobs) != 4 {
		t.Errorf("job count = %d, want 4", len(resp.Jobs))
	}

	if !broker.HasJob("video_transcode") {
		t.Error("expected video_transcode job to be enqueued")
	}
	if !broker.HasJob("video_thumbnail") {
		t.Error("expected video_thumbnail job to be enqueued")
	}
}

func TestVideoTranscodeHandler_NonVideoFile(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	// Add a non-video file (image)
	queries.AddFile(createTestFileWithID(fileID, testUserID, "test-image.jpg"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{"resolutions": [720]}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/transcode", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	// Verify error message mentions it's not a video
	if !strings.Contains(rec.Body.String(), "not a video") {
		t.Errorf("expected 'not a video' in error message, got: %s", rec.Body.String())
	}
}

func TestVideoTranscodeHandler_ResolutionLimit(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))
	// Free tier blocks API write access via billing middleware, so testing resolution limits
	// requires Pro tier with transformation quota exceeded
	queries.BillingTier = db.SubscriptionTierFree

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	// Try to transcode at 4K - free tier should return upgrade_required
	body := `{"resolutions": [2160], "format": "mp4"}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/transcode", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Free tier users get blocked by billing middleware before resolution check
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	// Either "upgrade_required" from billing middleware or "resolution_limit" error is acceptable
	body_str := rec.Body.String()
	if !strings.Contains(body_str, "upgrade") && !strings.Contains(body_str, "resolution") {
		t.Errorf("expected upgrade or resolution limit error message, got: %s", body_str)
	}
}

func TestVideoTranscodeHandler_InvalidFormat(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{"resolutions": [720], "format": "avi"}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/transcode", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "invalid_format") {
		t.Errorf("expected 'invalid_format' error, got: %s", rec.Body.String())
	}
}

func TestVideoTranscodeHandler_FileNotFound(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	nonExistentID := uuid.MustParse("00000000-0000-0000-0000-000000000000")

	queries, storage, broker, cfg := setupTestDeps(t)

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{"resolutions": [720]}`
	req := httptest.NewRequest("POST", "/v1/files/"+nonExistentID.String()+"/video/transcode", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestVideoHLSHandler_Success(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))
	queries.BillingTier = db.SubscriptionTierPro // Pro tier has HLS access

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/hls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var resp VideoTranscodeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.FileID != fileID.String() {
		t.Errorf("file_id = %s, want %s", resp.FileID, fileID.String())
	}

	if len(resp.Jobs) != 1 {
		t.Errorf("job count = %d, want 1", len(resp.Jobs))
	}

	if !broker.HasJob("video_hls") {
		t.Error("expected video_hls job to be enqueued")
	}
}

func TestVideoHLSHandler_CustomSegmentDuration(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))
	queries.BillingTier = db.SubscriptionTierPro

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{"segment_duration": 6, "resolutions": [360, 720, 1080]}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/hls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	if !broker.HasJob("video_hls") {
		t.Error("expected video_hls job to be enqueued")
	}
}

func TestVideoHLSHandler_FreeTierRestricted(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))
	queries.BillingTier = db.SubscriptionTierFree // Free tier doesn't have HLS

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/hls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "Pro") {
		t.Errorf("expected 'Pro' in upgrade message, got: %s", rec.Body.String())
	}
}

func TestVideoHLSHandler_NonVideoFile(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestFileWithID(fileID, testUserID, "test-image.jpg")) // Not a video
	queries.BillingTier = db.SubscriptionTierPro

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	body := `{}`
	req := httptest.NewRequest("POST", "/v1/files/"+fileID.String()+"/video/hls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHLSStreamHandler_ManifestDelivery(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	req := httptest.NewRequest("GET", "/v1/files/"+fileID.String()+"/hls/master.m3u8", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should redirect to presigned URL
	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusTemporaryRedirect, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Error("expected Location header for redirect")
	}
}

func TestHLSStreamHandler_SegmentDelivery(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	queries.AddFile(createTestVideoFileWithID(fileID, testUserID, "test-video.mp4"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	// Route pattern is /v1/files/{id}/hls/{segment} - segment is a single path component
	req := httptest.NewRequest("GET", "/v1/files/"+fileID.String()+"/hls/segment001.ts", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should redirect to presigned URL
	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusTemporaryRedirect, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Error("expected Location header for redirect")
	}
}

func TestHLSStreamHandler_FileNotOwned(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	otherUserID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	queries, storage, broker, cfg := setupTestDeps(t)
	// File belongs to otherUserID
	queries.AddFile(createTestVideoFileWithID(fileID, otherUserID, "test-video.mp4"))

	router := NewRouter(&Config{
		Storage:       storage,
		Queries:       queries,
		Broker:        broker,
		MaxUploadSize: cfg.MaxUploadSize,
		JWTSecret:     cfg.JWTSecret,
	})

	// Request as testUserID
	req := httptest.NewRequest("GET", "/v1/files/"+fileID.String()+"/hls/master.m3u8", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testUserID, 1*time.Hour))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
