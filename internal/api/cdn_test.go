package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MockProcessor implements processor.Processor for testing
type MockProcessor struct {
	name        string
	types       []string
	result      *processor.Result
	processErr  error
	processFunc func(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error)
}

func (m *MockProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, opts, input)
	}
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.result, nil
}

func (m *MockProcessor) SupportedTypes() []string { return m.types }
func (m *MockProcessor) Name() string             { return m.name }

func setupCDNTestDeps(t *testing.T) (*MockQuerier, *MockStorage, *processor.Registry) {
	t.Helper()
	return NewMockQuerier(), NewMockStorage(), processor.NewRegistry()
}

func createTestShareByToken(fileID, userID uuid.UUID, token, storageKey, contentType, filename string, expiresAt *time.Time, allowedTransforms []string) db.GetFileShareByTokenRow {
	pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	shareID := uuid.New()
	pgShareID := pgtype.UUID{Bytes: shareID, Valid: true}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	var expires pgtype.Timestamptz
	if expiresAt != nil {
		expires = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}

	return db.GetFileShareByTokenRow{
		ID:                pgShareID,
		FileID:            pgFileID,
		Token:             token,
		ExpiresAt:         expires,
		AllowedTransforms: allowedTransforms,
		AccessCount:       0,
		CreatedAt:         now,
		StorageKey:        storageKey,
		ContentType:       contentType,
		UserID:            pgUserID,
		Filename:          filename,
	}
}

func TestCDNHandler_TokenValidation(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		transforms string
		filename   string
		setupMocks func(q *MockQuerier, s *MockStorage, r *processor.Registry)
		wantStatus int
		wantBody   string
	}{
		{
			name:       "missing_share_token",
			token:      "",
			transforms: "_",
			filename:   "test.jpg",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {},
			wantStatus: http.StatusBadRequest,
			wantBody:   "missing share token",
		},
		{
			name:       "invalid_share_token",
			token:      "nonexistent-token",
			transforms: "_",
			filename:   "test.jpg",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {},
			wantStatus: http.StatusNotFound,
			wantBody:   "share not found or expired",
		},
		{
			name:       "expired_share_token",
			token:      "expired-token",
			transforms: "_",
			filename:   "test.jpg",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				expiredTime := time.Now().Add(-1 * time.Hour)
				share := createTestShareByToken(
					uuid.New(), uuid.New(), "expired-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg",
					&expiredTime, nil,
				)
				q.AddShareByToken("expired-token", share)
			},
			wantStatus: http.StatusNotFound,
			wantBody:   "share not found or expired",
		},
		{
			name:       "valid_share_token_original",
			token:      "valid-token",
			transforms: "_",
			filename:   "test.jpg",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(
					uuid.New(), uuid.New(), "valid-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg",
					nil, nil,
				)
				q.AddShareByToken("valid-token", share)
				s.PresignedURLFn = func(key string, expiry int) (string, error) {
					return "https://cdn.example.com/" + key, nil
				}
			},
			wantStatus: http.StatusTemporaryRedirect,
		},
		{
			name:       "valid_share_token_original_keyword",
			token:      "valid-token2",
			transforms: "original",
			filename:   "test.jpg",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(
					uuid.New(), uuid.New(), "valid-token2",
					"uploads/test.jpg", "image/jpeg", "test.jpg",
					nil, nil,
				)
				q.AddShareByToken("valid-token2", share)
				s.PresignedURLFn = func(key string, expiry int) (string, error) {
					return "https://cdn.example.com/" + key, nil
				}
			},
			wantStatus: http.StatusTemporaryRedirect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, registry := setupCDNTestDeps(t)
			tt.setupMocks(queries, storage, registry)

			cfg := &CDNConfig{
				Storage:  storage,
				Queries:  queries,
				Registry: registry,
			}

			handler := CDNHandler(cfg)

			req := httptest.NewRequest("GET", "/cdn/"+tt.token+"/"+tt.transforms+"/"+tt.filename, nil)
			req.SetPathValue("token", tt.token)
			req.SetPathValue("transforms", tt.transforms)
			req.SetPathValue("filename", tt.filename)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestCDNHandler_TransformParsing(t *testing.T) {
	fileID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name       string
		transforms string
		setupMocks func(q *MockQuerier, s *MockStorage, r *processor.Registry)
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid_width_transform",
			transforms: "w_800",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "transform-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("transform-token", share)

				// Add mock processor
				mockProc := &MockProcessor{
					name:  "resize",
					types: []string{"image/jpeg"},
					result: &processor.Result{
						Data:        io.NopCloser(bytes.NewReader([]byte("processed"))),
						ContentType: "image/jpeg",
						Filename:    "test.jpg",
					},
				}
				r.Register("resize", mockProc)

				// Setup storage download
				_ = s.MemoryStorage.Upload(context.Background(), "uploads/test.jpg",
					bytes.NewReader([]byte("original image data")), "image/jpeg", 19)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid_transform_format",
			transforms: "w800",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "transform-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("transform-token", share)
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid transform format",
		},
		{
			name:       "unknown_transform_key",
			transforms: "x_123",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "transform-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("transform-token", share)
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "unknown transform key",
		},
		{
			name:       "width_out_of_range",
			transforms: "w_20000",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "transform-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("transform-token", share)
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "width must be between",
		},
		{
			name:       "invalid_quality",
			transforms: "q_200",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "transform-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("transform-token", share)
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "quality must be between",
		},
		{
			name:       "unsupported_format",
			transforms: "f_bmp",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "transform-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("transform-token", share)
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "unsupported format",
		},
		{
			name:       "valid_full_transform",
			transforms: "w_800,h_600,q_85",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "transform-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("transform-token", share)

				mockProc := &MockProcessor{
					name:  "resize",
					types: []string{"image/jpeg"},
					result: &processor.Result{
						Data:        io.NopCloser(bytes.NewReader([]byte("processed"))),
						ContentType: "image/jpeg",
						Filename:    "test.jpg",
					},
				}
				r.Register("resize", mockProc)

				_ = s.MemoryStorage.Upload(context.Background(), "uploads/test.jpg",
					bytes.NewReader([]byte("original image data")), "image/jpeg", 19)
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, registry := setupCDNTestDeps(t)
			tt.setupMocks(queries, storage, registry)

			cfg := &CDNConfig{
				Storage:  storage,
				Queries:  queries,
				Registry: registry,
			}

			handler := CDNHandler(cfg)

			req := httptest.NewRequest("GET", "/cdn/transform-token/"+tt.transforms+"/test.jpg", nil)
			req.SetPathValue("token", "transform-token")
			req.SetPathValue("transforms", tt.transforms)
			req.SetPathValue("filename", "test.jpg")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestCDNHandler_AllowedTransforms(t *testing.T) {
	fileID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name       string
		transforms string
		allowed    []string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "transform_allowed_wildcard",
			transforms: "w_800",
			allowed:    []string{"*"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "transform_allowed_specific",
			transforms: "w_800",
			allowed:    []string{"w_800"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "transform_not_in_allowed",
			transforms: "w_1200",
			allowed:    []string{"w_800"},
			wantStatus: http.StatusForbidden,
			wantBody:   "transform not allowed",
		},
		{
			name:       "original_always_allowed",
			transforms: "_",
			allowed:    []string{"w_800"},
			wantStatus: http.StatusTemporaryRedirect,
		},
		{
			name:       "empty_allowed_list_permits_all",
			transforms: "w_800",
			allowed:    nil,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, registry := setupCDNTestDeps(t)

			share := createTestShareByToken(fileID, userID, "allowed-token",
				"uploads/test.jpg", "image/jpeg", "test.jpg", nil, tt.allowed)
			queries.AddShareByToken("allowed-token", share)

			storage.PresignedURLFn = func(key string, expiry int) (string, error) {
				return "https://cdn.example.com/" + key, nil
			}

			if tt.transforms != "_" && tt.transforms != "original" && tt.wantStatus == http.StatusOK {
				mockProc := &MockProcessor{
					name:  "resize",
					types: []string{"image/jpeg"},
					result: &processor.Result{
						Data:        io.NopCloser(bytes.NewReader([]byte("processed"))),
						ContentType: "image/jpeg",
						Filename:    "test.jpg",
					},
				}
				registry.Register("resize", mockProc)

				_ = storage.MemoryStorage.Upload(context.Background(), "uploads/test.jpg",
					bytes.NewReader([]byte("original image data")), "image/jpeg", 19)
			}

			cfg := &CDNConfig{
				Storage:  storage,
				Queries:  queries,
				Registry: registry,
			}

			handler := CDNHandler(cfg)

			req := httptest.NewRequest("GET", "/cdn/allowed-token/"+tt.transforms+"/test.jpg", nil)
			req.SetPathValue("token", "allowed-token")
			req.SetPathValue("transforms", tt.transforms)
			req.SetPathValue("filename", "test.jpg")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestCDNHandler_CacheBehavior(t *testing.T) {
	fileID := uuid.New()
	userID := uuid.New()
	pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}

	tests := []struct {
		name         string
		setupMocks   func(q *MockQuerier, s *MockStorage, r *processor.Registry)
		wantStatus   int
		wantRedirect bool
	}{
		{
			name: "cache_hit_redirects",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "cache-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("cache-token", share)

				// Add existing cache entry
				cacheKey := (&TransformOptions{Width: 800}).CacheKey()
				cache := db.TransformCache{
					ID:          pgtype.UUID{Bytes: uuid.New(), Valid: true},
					FileID:      pgFileID,
					CacheKey:    cacheKey,
					StorageKey:  "cache/test/cached.jpg",
					ContentType: "image/jpeg",
				}
				q.AddTransformCache(cache)

				s.PresignedURLFn = func(key string, expiry int) (string, error) {
					return "https://cdn.example.com/" + key, nil
				}
			},
			wantStatus:   http.StatusTemporaryRedirect,
			wantRedirect: true,
		},
		{
			name: "cache_miss_processes",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "cache-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("cache-token", share)

				// No cache entry - will process
				mockProc := &MockProcessor{
					name:  "resize",
					types: []string{"image/jpeg"},
					result: &processor.Result{
						Data:        io.NopCloser(bytes.NewReader([]byte("processed"))),
						ContentType: "image/jpeg",
						Filename:    "test.jpg",
					},
				}
				r.Register("resize", mockProc)

				_ = s.MemoryStorage.Upload(context.Background(), "uploads/test.jpg",
					bytes.NewReader([]byte("original")), "image/jpeg", 8)
			},
			wantStatus:   http.StatusOK,
			wantRedirect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, registry := setupCDNTestDeps(t)
			tt.setupMocks(queries, storage, registry)

			cfg := &CDNConfig{
				Storage:  storage,
				Queries:  queries,
				Registry: registry,
			}

			handler := CDNHandler(cfg)

			req := httptest.NewRequest("GET", "/cdn/cache-token/w_800/test.jpg", nil)
			req.SetPathValue("token", "cache-token")
			req.SetPathValue("transforms", "w_800")
			req.SetPathValue("filename", "test.jpg")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantRedirect {
				location := rec.Header().Get("Location")
				if location == "" {
					t.Error("expected redirect location header")
				}
			}
		})
	}
}

func TestCDNHandler_ProcessingErrors(t *testing.T) {
	fileID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name       string
		setupMocks func(q *MockQuerier, s *MockStorage, r *processor.Registry)
		wantStatus int
		wantBody   string
	}{
		{
			name: "processor_not_found",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "proc-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("proc-token", share)

				// No processor registered
				_ = s.MemoryStorage.Upload(context.Background(), "uploads/test.jpg",
					bytes.NewReader([]byte("original")), "image/jpeg", 8)
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to process",
		},
		{
			name: "processing_error",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "proc-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("proc-token", share)

				mockProc := &MockProcessor{
					name:       "resize",
					types:      []string{"image/jpeg"},
					processErr: errors.New("processing failed"),
				}
				r.Register("resize", mockProc)

				_ = s.MemoryStorage.Upload(context.Background(), "uploads/test.jpg",
					bytes.NewReader([]byte("original")), "image/jpeg", 8)
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to process",
		},
		{
			name: "storage_download_error",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "proc-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("proc-token", share)

				mockProc := &MockProcessor{
					name:  "resize",
					types: []string{"image/jpeg"},
				}
				r.Register("resize", mockProc)

				// Don't upload the file - download will fail
				s.DownloadErr = errors.New("file not found in storage")
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to process",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, registry := setupCDNTestDeps(t)
			tt.setupMocks(queries, storage, registry)

			cfg := &CDNConfig{
				Storage:  storage,
				Queries:  queries,
				Registry: registry,
			}

			handler := CDNHandler(cfg)

			req := httptest.NewRequest("GET", "/cdn/proc-token/w_800/test.jpg", nil)
			req.SetPathValue("token", "proc-token")
			req.SetPathValue("transforms", "w_800")
			req.SetPathValue("filename", "test.jpg")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestCDNHandler_CacheHeaders(t *testing.T) {
	fileID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name            string
		transforms      string
		setupMocks      func(q *MockQuerier, s *MockStorage, r *processor.Registry)
		wantCacheHeader string
	}{
		{
			name:       "original_sets_public_cache",
			transforms: "_",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "header-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("header-token", share)
				s.PresignedURLFn = func(key string, expiry int) (string, error) {
					return "https://cdn.example.com/" + key, nil
				}
			},
			wantCacheHeader: "public, max-age=3000",
		},
		{
			name:       "processed_sets_short_cache",
			transforms: "w_800",
			setupMocks: func(q *MockQuerier, s *MockStorage, r *processor.Registry) {
				share := createTestShareByToken(fileID, userID, "header-token",
					"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
				q.AddShareByToken("header-token", share)

				mockProc := &MockProcessor{
					name:  "resize",
					types: []string{"image/jpeg"},
					result: &processor.Result{
						Data:        io.NopCloser(bytes.NewReader([]byte("processed"))),
						ContentType: "image/jpeg",
						Filename:    "test.jpg",
					},
				}
				r.Register("resize", mockProc)

				_ = s.MemoryStorage.Upload(context.Background(), "uploads/test.jpg",
					bytes.NewReader([]byte("original")), "image/jpeg", 8)
			},
			wantCacheHeader: "public, max-age=3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, storage, registry := setupCDNTestDeps(t)
			tt.setupMocks(queries, storage, registry)

			cfg := &CDNConfig{
				Storage:  storage,
				Queries:  queries,
				Registry: registry,
			}

			handler := CDNHandler(cfg)

			req := httptest.NewRequest("GET", "/cdn/header-token/"+tt.transforms+"/test.jpg", nil)
			req.SetPathValue("token", "header-token")
			req.SetPathValue("transforms", tt.transforms)
			req.SetPathValue("filename", "test.jpg")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			cacheControl := rec.Header().Get("Cache-Control")
			if cacheControl != tt.wantCacheHeader {
				t.Errorf("Cache-Control = %q, want %q", cacheControl, tt.wantCacheHeader)
			}
		})
	}
}

func TestCDNHandler_ContentDisposition(t *testing.T) {
	fileID := uuid.New()
	userID := uuid.New()

	queries, storage, registry := setupCDNTestDeps(t)

	share := createTestShareByToken(fileID, userID, "disp-token",
		"uploads/test.jpg", "image/jpeg", "test.jpg", nil, nil)
	queries.AddShareByToken("disp-token", share)

	storage.PresignedURLFn = func(key string, expiry int) (string, error) {
		return "https://cdn.example.com/" + key, nil
	}

	cfg := &CDNConfig{
		Storage:  storage,
		Queries:  queries,
		Registry: registry,
	}

	handler := CDNHandler(cfg)

	req := httptest.NewRequest("GET", "/cdn/disp-token/_/my-image.jpg", nil)
	req.SetPathValue("token", "disp-token")
	req.SetPathValue("transforms", "_")
	req.SetPathValue("filename", "my-image.jpg")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	contentDisp := rec.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisp, "my-image.jpg") {
		t.Errorf("Content-Disposition = %q, want to contain filename", contentDisp)
	}
}

func TestGenerateShareToken(t *testing.T) {
	token1, err := GenerateShareToken()
	if err != nil {
		t.Fatalf("GenerateShareToken() error = %v", err)
	}

	if len(token1) == 0 {
		t.Error("token should not be empty")
	}

	if len(token1) > 43 {
		t.Errorf("token length = %d, want <= 43", len(token1))
	}

	// Test uniqueness
	token2, _ := GenerateShareToken()
	if token1 == token2 {
		t.Error("tokens should be unique")
	}
}

func TestCreateShareHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	otherUserID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")
	testFileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name       string
		fileID     string
		query      string
		userID     *uuid.UUID
		setupMocks func(q *MockQuerier)
		wantStatus int
		wantBody   string
	}{
		{
			name:   "create_share_success",
			fileID: testFileID.String(),
			userID: &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, testUserID, "test.jpg")
				q.AddFile(file)
			},
			wantStatus: http.StatusCreated,
			wantBody:   "share_url",
		},
		{
			name:   "create_share_with_expiration",
			fileID: testFileID.String(),
			query:  "?expires=24h",
			userID: &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, testUserID, "test.jpg")
				q.AddFile(file)
			},
			wantStatus: http.StatusCreated,
			wantBody:   "expires_at",
		},
		{
			name:       "create_share_file_not_found",
			fileID:     uuid.New().String(),
			userID:     &testUserID,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusNotFound,
			wantBody:   "file not found",
		},
		{
			name:   "create_share_wrong_owner",
			fileID: testFileID.String(),
			userID: &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, otherUserID, "test.jpg")
				q.AddFile(file)
			},
			wantStatus: http.StatusNotFound,
			wantBody:   "file not found",
		},
		{
			name:       "create_share_invalid_file_id",
			fileID:     "not-a-uuid",
			userID:     &testUserID,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid file ID",
		},
		{
			name:       "create_share_unauthorized",
			fileID:     testFileID.String(),
			userID:     nil,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusUnauthorized,
			wantBody:   "authentication required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries := NewMockQuerier()
			storage := NewMockStorage()
			tt.setupMocks(queries)

			cfg := &CDNConfig{
				Storage: storage,
				Queries: queries,
			}

			handler := CreateShareHandler(cfg, "https://cdn.example.com")

			req := httptest.NewRequest("POST", "/v1/files/"+tt.fileID+"/share"+tt.query, nil)
			req.SetPathValue("id", tt.fileID)

			if tt.userID != nil {
				ctx := context.WithValue(req.Context(), UserIDKey, *tt.userID)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestListSharesHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	otherUserID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")
	testFileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name       string
		fileID     string
		userID     *uuid.UUID
		setupMocks func(q *MockQuerier)
		wantStatus int
		wantBody   string
	}{
		{
			name:   "list_shares_success",
			fileID: testFileID.String(),
			userID: &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, testUserID, "test.jpg")
				q.AddFile(file)
				// Add some shares
				pgFileID := pgtype.UUID{Bytes: testFileID, Valid: true}
				share := db.FileShare{
					ID:          pgtype.UUID{Bytes: uuid.New(), Valid: true},
					FileID:      pgFileID,
					Token:       "test-token",
					AccessCount: 5,
					CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}
				q.AddShare(share)
			},
			wantStatus: http.StatusOK,
			wantBody:   `"shares":[`,
		},
		{
			name:   "list_shares_empty",
			fileID: testFileID.String(),
			userID: &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, testUserID, "test.jpg")
				q.AddFile(file)
			},
			wantStatus: http.StatusOK,
			wantBody:   `"shares":[]`,
		},
		{
			name:       "list_shares_file_not_found",
			fileID:     uuid.New().String(),
			userID:     &testUserID,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusNotFound,
			wantBody:   "file not found",
		},
		{
			name:   "list_shares_wrong_owner",
			fileID: testFileID.String(),
			userID: &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, otherUserID, "test.jpg")
				q.AddFile(file)
			},
			wantStatus: http.StatusNotFound,
			wantBody:   "file not found",
		},
		{
			name:       "list_shares_invalid_file_id",
			fileID:     "not-a-uuid",
			userID:     &testUserID,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid file ID",
		},
		{
			name:       "list_shares_unauthorized",
			fileID:     testFileID.String(),
			userID:     nil,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusUnauthorized,
			wantBody:   "authentication required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries := NewMockQuerier()
			storage := NewMockStorage()
			tt.setupMocks(queries)

			cfg := &CDNConfig{
				Storage: storage,
				Queries: queries,
			}

			handler := ListSharesHandler(cfg)

			req := httptest.NewRequest("GET", "/v1/files/"+tt.fileID+"/shares", nil)
			req.SetPathValue("id", tt.fileID)

			if tt.userID != nil {
				ctx := context.WithValue(req.Context(), UserIDKey, *tt.userID)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestDeleteShareHandler(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	otherUserID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")
	testFileID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440000")
	testShareID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name       string
		shareID    string
		userID     *uuid.UUID
		setupMocks func(q *MockQuerier)
		wantStatus int
		wantBody   string
	}{
		{
			name:    "delete_share_success",
			shareID: testShareID.String(),
			userID:  &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, testUserID, "test.jpg")
				q.AddFile(file)
				pgFileID := pgtype.UUID{Bytes: testFileID, Valid: true}
				pgShareID := pgtype.UUID{Bytes: testShareID, Valid: true}
				share := db.FileShare{
					ID:        pgShareID,
					FileID:    pgFileID,
					Token:     "test-token",
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}
				q.AddShare(share)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete_share_not_found",
			shareID:    uuid.New().String(),
			userID:     &testUserID,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusNotFound,
			wantBody:   "share not found",
		},
		{
			name:    "delete_share_wrong_owner",
			shareID: testShareID.String(),
			userID:  &testUserID,
			setupMocks: func(q *MockQuerier) {
				file := createTestFileWithID(testFileID, otherUserID, "test.jpg")
				q.AddFile(file)
				pgFileID := pgtype.UUID{Bytes: testFileID, Valid: true}
				pgShareID := pgtype.UUID{Bytes: testShareID, Valid: true}
				share := db.FileShare{
					ID:        pgShareID,
					FileID:    pgFileID,
					Token:     "test-token",
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}
				q.AddShare(share)
			},
			wantStatus: http.StatusNotFound,
			wantBody:   "share not found",
		},
		{
			name:       "delete_share_invalid_id",
			shareID:    "not-a-uuid",
			userID:     &testUserID,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid share ID",
		},
		{
			name:       "delete_share_unauthorized",
			shareID:    testShareID.String(),
			userID:     nil,
			setupMocks: func(q *MockQuerier) {},
			wantStatus: http.StatusUnauthorized,
			wantBody:   "authentication required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries := NewMockQuerier()
			storage := NewMockStorage()
			tt.setupMocks(queries)

			cfg := &CDNConfig{
				Storage: storage,
				Queries: queries,
			}

			handler := DeleteShareHandler(cfg)

			req := httptest.NewRequest("DELETE", "/v1/shares/"+tt.shareID, nil)
			req.SetPathValue("shareId", tt.shareID)

			if tt.userID != nil {
				ctx := context.WithValue(req.Context(), UserIDKey, *tt.userID)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
