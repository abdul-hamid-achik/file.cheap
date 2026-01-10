package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestUploadSessionStore(t *testing.T) {
	store := &uploadSessionStore{
		sessions: make(map[string]*uploadSession),
	}

	t.Run("Set and Get", func(t *testing.T) {
		session := &uploadSession{
			ID:           "test-session-1",
			UserID:       uuid.New(),
			Filename:     "test.mp4",
			ContentType:  "video/mp4",
			TotalSize:    1024 * 1024,
			ChunksTotal:  10,
			ChunksLoaded: make(map[int]bool),
		}

		store.Set(session)

		got, ok := store.Get("test-session-1")
		if !ok {
			t.Fatal("Get() returned false, want true")
		}
		if got.ID != session.ID {
			t.Errorf("Got session ID = %q, want %q", got.ID, session.ID)
		}
		if got.Filename != session.Filename {
			t.Errorf("Got Filename = %q, want %q", got.Filename, session.Filename)
		}
	})

	t.Run("Get non-existent", func(t *testing.T) {
		_, ok := store.Get("non-existent")
		if ok {
			t.Error("Get() returned true for non-existent session")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		session := &uploadSession{
			ID:           "test-session-2",
			UserID:       uuid.New(),
			ChunksLoaded: make(map[int]bool),
		}
		store.Set(session)

		store.Delete("test-session-2")

		_, ok := store.Get("test-session-2")
		if ok {
			t.Error("Get() returned true after Delete()")
		}
	})

	t.Run("Concurrent access", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				id := uuid.New().String()
				session := &uploadSession{
					ID:           id,
					UserID:       uuid.New(),
					ChunksLoaded: make(map[int]bool),
				}
				store.Set(session)
				store.Get(id)
				store.Delete(id)
			}(i)
		}

		wg.Wait()
	})
}

func TestInitUploadRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     InitUploadRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: InitUploadRequest{
				Filename:    "video.mp4",
				ContentType: "video/mp4",
				TotalSize:   1024 * 1024,
			},
			wantErr: false,
		},
		{
			name: "empty filename",
			req: InitUploadRequest{
				Filename:    "",
				ContentType: "video/mp4",
				TotalSize:   1024,
			},
			wantErr: true,
		},
		{
			name: "zero size",
			req: InitUploadRequest{
				Filename:    "video.mp4",
				ContentType: "video/mp4",
				TotalSize:   0,
			},
			wantErr: true,
		},
		{
			name: "negative size",
			req: InitUploadRequest{
				Filename:    "video.mp4",
				ContentType: "video/mp4",
				TotalSize:   -1024,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasErr := tt.req.Filename == "" || tt.req.TotalSize <= 0
			if hasErr != tt.wantErr {
				t.Errorf("validation = %v, wantErr = %v", hasErr, tt.wantErr)
			}
		})
	}
}

func TestInitUploadResponse_JSON(t *testing.T) {
	resp := InitUploadResponse{
		UploadID:    "upload-123",
		ChunkSize:   5 * 1024 * 1024,
		ChunksTotal: 10,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded InitUploadResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.UploadID != resp.UploadID {
		t.Errorf("UploadID = %q, want %q", decoded.UploadID, resp.UploadID)
	}
	if decoded.ChunkSize != resp.ChunkSize {
		t.Errorf("ChunkSize = %d, want %d", decoded.ChunkSize, resp.ChunkSize)
	}
	if decoded.ChunksTotal != resp.ChunksTotal {
		t.Errorf("ChunksTotal = %d, want %d", decoded.ChunksTotal, resp.ChunksTotal)
	}
}

func TestUploadChunkResponse_JSON(t *testing.T) {
	resp := UploadChunkResponse{
		UploadID:     "upload-123",
		ChunkIndex:   5,
		ChunksLoaded: 6,
		ChunksTotal:  10,
		Complete:     false,
		FileID:       "",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded UploadChunkResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.UploadID != resp.UploadID {
		t.Errorf("UploadID = %q, want %q", decoded.UploadID, resp.UploadID)
	}
	if decoded.ChunkIndex != resp.ChunkIndex {
		t.Errorf("ChunkIndex = %d, want %d", decoded.ChunkIndex, resp.ChunkIndex)
	}
	if decoded.Complete != resp.Complete {
		t.Errorf("Complete = %v, want %v", decoded.Complete, resp.Complete)
	}
}

func TestUploadChunkResponse_Complete(t *testing.T) {
	resp := UploadChunkResponse{
		UploadID:     "upload-123",
		ChunkIndex:   9,
		ChunksLoaded: 10,
		ChunksTotal:  10,
		Complete:     true,
		FileID:       "file-abc",
	}

	if !resp.Complete {
		t.Error("Complete = false, want true")
	}
	if resp.FileID == "" {
		t.Error("FileID is empty for complete upload")
	}
}

func TestChunkedUploadConfig(t *testing.T) {
	cfg := &ChunkedUploadConfig{
		MaxUploadSize: 100 * 1024 * 1024,
		ChunkSize:     5 * 1024 * 1024,
	}

	if cfg.MaxUploadSize != 100*1024*1024 {
		t.Errorf("MaxUploadSize = %d, want %d", cfg.MaxUploadSize, 100*1024*1024)
	}
	if cfg.ChunkSize != 5*1024*1024 {
		t.Errorf("ChunkSize = %d, want %d", cfg.ChunkSize, 5*1024*1024)
	}
}

func TestUploadSession_ChunkTracking(t *testing.T) {
	session := &uploadSession{
		ID:           "test-session",
		UserID:       uuid.New(),
		Filename:     "test.mp4",
		ContentType:  "video/mp4",
		TotalSize:    50 * 1024 * 1024,
		ChunksTotal:  10,
		ChunksLoaded: make(map[int]bool),
	}

	t.Run("Mark chunks as loaded", func(t *testing.T) {
		session.mu.Lock()
		session.ChunksLoaded[0] = true
		session.ChunksLoaded[1] = true
		session.ChunksLoaded[2] = true
		chunksLoaded := len(session.ChunksLoaded)
		session.mu.Unlock()

		if chunksLoaded != 3 {
			t.Errorf("ChunksLoaded = %d, want 3", chunksLoaded)
		}
	})

	t.Run("Check completion", func(t *testing.T) {
		session.mu.Lock()
		for i := 0; i < session.ChunksTotal; i++ {
			session.ChunksLoaded[i] = true
		}
		complete := len(session.ChunksLoaded) == session.ChunksTotal
		session.mu.Unlock()

		if !complete {
			t.Error("Upload should be complete")
		}
	})
}

func TestInitChunkedUploadHandler_Unauthorized(t *testing.T) {
	cfg := &ChunkedUploadConfig{
		ChunkSize: 5 * 1024 * 1024,
	}

	handler := InitChunkedUploadHandler(cfg)

	reqBody := `{"filename":"test.mp4","content_type":"video/mp4","total_size":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/upload/chunked", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestInitChunkedUploadHandler_InvalidBody(t *testing.T) {
	cfg := &ChunkedUploadConfig{
		ChunkSize: 5 * 1024 * 1024,
	}

	handler := InitChunkedUploadHandler(cfg)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	req := httptest.NewRequest(http.MethodPost, "/v1/upload/chunked", strings.NewReader("invalid json"))
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestInitChunkedUploadHandler_MissingFields(t *testing.T) {
	cfg := &ChunkedUploadConfig{
		ChunkSize: 5 * 1024 * 1024,
	}

	handler := InitChunkedUploadHandler(cfg)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	tests := []struct {
		name string
		body string
	}{
		{"missing filename", `{"content_type":"video/mp4","total_size":1024}`},
		{"missing total_size", `{"filename":"test.mp4","content_type":"video/mp4"}`},
		{"empty filename", `{"filename":"","content_type":"video/mp4","total_size":1024}`},
		{"zero total_size", `{"filename":"test.mp4","content_type":"video/mp4","total_size":0}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/upload/chunked", strings.NewReader(tt.body))
			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGetUploadStatusHandler_Unauthorized(t *testing.T) {
	cfg := &ChunkedUploadConfig{}

	handler := GetUploadStatusHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/upload/chunked/test-id", nil)
	req.SetPathValue("uploadId", "test-id")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestGetUploadStatusHandler_NotFound(t *testing.T) {
	cfg := &ChunkedUploadConfig{}

	handler := GetUploadStatusHandler(cfg)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	req := httptest.NewRequest(http.MethodGet, "/v1/upload/chunked/non-existent", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("uploadId", "non-existent")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestUploadChunkHandler_Unauthorized(t *testing.T) {
	cfg := &ChunkedUploadConfig{}

	handler := UploadChunkHandler(cfg)

	req := httptest.NewRequest(http.MethodPut, "/v1/upload/chunked/test-id?chunk=0", bytes.NewReader([]byte("chunk data")))
	req.SetPathValue("uploadId", "test-id")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUploadChunkHandler_InvalidChunkIndex(t *testing.T) {
	cfg := &ChunkedUploadConfig{}

	handler := UploadChunkHandler(cfg)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	tests := []struct {
		name       string
		chunkParam string
	}{
		{"non-numeric", "abc"},
		{"negative", "-1"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/v1/upload/chunked/test-id?chunk=" + tt.chunkParam
			req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader([]byte("chunk data")))
			req = req.WithContext(ctx)
			req.SetPathValue("uploadId", "test-id")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d for chunk=%q", rr.Code, http.StatusBadRequest, tt.chunkParam)
			}
		})
	}
}

func TestUploadChunkHandler_SessionNotFound(t *testing.T) {
	cfg := &ChunkedUploadConfig{}

	handler := UploadChunkHandler(cfg)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	req := httptest.NewRequest(http.MethodPut, "/v1/upload/chunked/non-existent?chunk=0", bytes.NewReader([]byte("chunk data")))
	req = req.WithContext(ctx)
	req.SetPathValue("uploadId", "non-existent")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestCancelUploadHandler_Unauthorized(t *testing.T) {
	cfg := &ChunkedUploadConfig{}

	handler := CancelUploadHandler(cfg)

	req := httptest.NewRequest(http.MethodDelete, "/v1/upload/chunked/test-id", nil)
	req.SetPathValue("uploadId", "test-id")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestCancelUploadHandler_NotFound(t *testing.T) {
	cfg := &ChunkedUploadConfig{}

	handler := CancelUploadHandler(cfg)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	req := httptest.NewRequest(http.MethodDelete, "/v1/upload/chunked/non-existent", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("uploadId", "non-existent")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestChunkSizeCalculation(t *testing.T) {
	tests := []struct {
		name        string
		totalSize   int64
		chunkSize   int64
		wantChunks  int
	}{
		{
			name:       "exact division",
			totalSize:  50 * 1024 * 1024,
			chunkSize:  5 * 1024 * 1024,
			wantChunks: 10,
		},
		{
			name:       "with remainder",
			totalSize:  52 * 1024 * 1024,
			chunkSize:  5 * 1024 * 1024,
			wantChunks: 11,
		},
		{
			name:       "small file",
			totalSize:  1024,
			chunkSize:  5 * 1024 * 1024,
			wantChunks: 1,
		},
		{
			name:       "exactly one chunk",
			totalSize:  5 * 1024 * 1024,
			chunkSize:  5 * 1024 * 1024,
			wantChunks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := int((tt.totalSize + tt.chunkSize - 1) / tt.chunkSize)
			if chunks != tt.wantChunks {
				t.Errorf("Chunks = %d, want %d", chunks, tt.wantChunks)
			}
		})
	}
}

// mockStorage is a minimal mock for testing
type mockChunkedStorage struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func newMockChunkedStorage() *mockChunkedStorage {
	return &mockChunkedStorage{
		data: make(map[string][]byte),
	}
}

func (m *mockChunkedStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.data[key] = data
	m.mu.Unlock()
	return nil
}

func (m *mockChunkedStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	data, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockChunkedStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}

func (m *mockChunkedStorage) GetURL(ctx context.Context, key string) (string, error) {
	return "https://example.com/" + key, nil
}

func (m *mockChunkedStorage) GetPresignedURL(ctx context.Context, key string, expires int) (string, error) {
	return "https://example.com/" + key + "?token=abc", nil
}

func (m *mockChunkedStorage) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	_, ok := m.data[key]
	m.mu.RUnlock()
	return ok, nil
}

func TestSessionOwnershipVerification(t *testing.T) {
	store := &uploadSessionStore{
		sessions: make(map[string]*uploadSession),
	}

	ownerID := uuid.New()
	otherID := uuid.New()

	session := &uploadSession{
		ID:           "test-session",
		UserID:       ownerID,
		Filename:     "test.mp4",
		ChunksLoaded: make(map[int]bool),
	}
	store.Set(session)

	t.Run("owner can access", func(t *testing.T) {
		got, ok := store.Get("test-session")
		if !ok {
			t.Fatal("Get() returned false")
		}
		if got.UserID != ownerID {
			t.Error("Wrong owner")
		}
	})

	t.Run("non-owner check", func(t *testing.T) {
		got, ok := store.Get("test-session")
		if !ok {
			t.Fatal("Get() returned false")
		}
		if got.UserID == otherID {
			t.Error("Session should not belong to other user")
		}
	})
}
