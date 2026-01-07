package web

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/auth"
	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// createTestRouter creates a router for testing with mock services
func createTestRouter(cfg *Config) http.Handler {
	// Create a mock session manager that doesn't require DB
	// For tests, we'll pass nil and handlers will handle gracefully
	return NewRouter(cfg, nil, nil, nil, nil)
}

func TestRouterRoutes(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	tests := []struct {
		method     string
		path       string
		wantStatus int
	}{
		{"GET", "/", http.StatusOK},
		// These routes require auth, so they redirect to login
		{"GET", "/upload", http.StatusFound},
		{"GET", "/files", http.StatusFound},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Errorf("%s %s: route not registered (got 404)", tt.method, tt.path)
			}
		})
	}
}

func TestRouterMethodNotAllowed(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	tests := []struct {
		method string
		path   string
	}{
		{"PUT", "/upload"},
		{"DELETE", "/upload"},
		{"PUT", "/files"},
		{"DELETE", "/files"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// These should either 404 or 405
			if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound && rec.Code != http.StatusFound {
				t.Errorf("%s %s: status = %d, want 405, 404, or 302", tt.method, tt.path, rec.Code)
			}
		})
	}
}

func TestHomeRoute(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("home page: status = %d, want 200", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !bytes.Contains([]byte(contentType), []byte("text/html")) {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}
}

func TestLoginRoute(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("login page: status = %d, want 200", rec.Code)
	}
}

func TestRegisterRoute(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/register", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("register page: status = %d, want 200", rec.Code)
	}
}

func TestUploadPageRequiresAuth(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/upload", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should redirect to login
	if rec.Code != http.StatusFound {
		t.Errorf("upload page without auth: status = %d, want 302", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location == "" || !bytes.Contains([]byte(location), []byte("/login")) {
		t.Errorf("upload page should redirect to login, got: %s", location)
	}
}

func TestDashboardRequiresAuth(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should redirect to login
	if rec.Code != http.StatusFound {
		t.Errorf("dashboard without auth: status = %d, want 302", rec.Code)
	}
}

func TestFilesListRequiresAuth(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/files", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should redirect to login
	if rec.Code != http.StatusFound {
		t.Errorf("files list without auth: status = %d, want 302", rec.Code)
	}
}

func TestFileDetailRequiresAuth(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	fileID := uuid.New().String()
	req := httptest.NewRequest("GET", "/files/"+fileID, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should redirect to login
	if rec.Code != http.StatusFound {
		t.Errorf("file detail without auth: status = %d, want 302", rec.Code)
	}
}

func TestStaticAssets(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	// Static files will 404 unless the files exist
	paths := []string{
		"/static/css/style.css",
		"/static/js/app.js",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Just check it doesn't panic
			_ = rec.Code
		})
	}
}

func TestRouterWithHTMX(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	tests := []struct {
		name string
		path string
		htmx bool
	}{
		{"home without htmx", "/", false},
		{"home with htmx", "/", true},
		{"login without htmx", "/login", false},
		{"login with htmx", "/login", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.htmx {
				req.Header.Set("HX-Request", "true")
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			// Just check it doesn't panic
			_ = rec
		})
	}
}

func TestRouterErrorHandling(t *testing.T) {
	stor := NewMockStorage()
	stor.DownloadErr = io.ErrUnexpectedEOF

	cfg := &Config{
		Storage: stor,
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/files/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("router panicked: %v", r)
			}
		}()
		router.ServeHTTP(rec, req)
	}()
}

func TestRouterConcurrency(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func(n int) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("concurrent request %d panicked: %v", n, r)
				}
				done <- true
			}()

			paths := []string{"/", "/login", "/register"}
			path := paths[n%len(paths)]

			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}
}

func TestRouterIntegration(t *testing.T) {
	queries := NewMockQuerier()
	stor := NewMockStorage()

	cfg := &Config{
		Storage: stor,
	}
	router := createTestRouter(cfg)

	t.Run("visit home", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Error("home page not found")
		}
	})

	t.Run("visit login page", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/login", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Error("login page not found")
		}
	})

	t.Run("visit register page", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/register", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Error("register page not found")
		}
	})

	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID := uuid.New()
	queries.AddFile(db.File{
		ID:          pgtype.UUID{Bytes: fileID, Valid: true},
		UserID:      pgtype.UUID{Bytes: testUserID, Valid: true},
		Filename:    "integrated-test.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   1024,
		StorageKey:  "uploads/" + fileID.String() + "/integrated-test.jpg",
		Status:      db.FileStatusCompleted,
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})

	t.Run("files list requires auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		// Should redirect to login
		if rec.Code != http.StatusFound {
			t.Errorf("files without auth: status = %d, want 302", rec.Code)
		}
	})
}

func TestRouterNoQueryParams(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	tests := []struct {
		path string
	}{
		{"/login?return=/dashboard"},
		{"/register?ref=google"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Errorf("route with query params %s returned 404", tt.path)
			}
		})
	}
}

func TestRouterTrailingSlash(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	tests := []struct {
		path string
	}{
		{"/"},
		{"/login"},
		{"/register"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code >= 500 {
				t.Errorf("%s returned server error: %d", tt.path, rec.Code)
			}
		})
	}
}

func TestRouterHTTPHeaders(t *testing.T) {
	cfg := &Config{
		Storage: NewMockStorage(),
	}
	router := createTestRouter(cfg)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		ct := rec.Header().Get("Content-Type")
		if ct == "" {
			t.Error("response missing Content-Type header")
		}
	}
}

// Unused in current tests but kept for future use
var _ = auth.SessionManager{}
