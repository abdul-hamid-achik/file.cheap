package web

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/auth"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MockQuerier implements a mock database querier for testing
type MockQuerier struct {
	mu sync.RWMutex

	files    map[pgtype.UUID]db.File
	variants map[pgtype.UUID]db.FileVariant

	GetFileErr    error
	ListFilesErr  error
	CreateFileErr error
}

func NewMockQuerier() *MockQuerier {
	return &MockQuerier{
		files:    make(map[pgtype.UUID]db.File),
		variants: make(map[pgtype.UUID]db.FileVariant),
	}
}

func (m *MockQuerier) AddFile(f db.File) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[f.ID] = f
}

func (m *MockQuerier) AddVariant(v db.FileVariant) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.variants[v.ID] = v
}

func (m *MockQuerier) GetFile(ctx context.Context, id pgtype.UUID) (db.File, error) {
	if m.GetFileErr != nil {
		return db.File{}, m.GetFileErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[id]
	if !ok {
		return db.File{}, errors.New("file not found")
	}
	return f, nil
}

func (m *MockQuerier) GetFileByStorageKey(ctx context.Context, storageKey string) (db.File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, f := range m.files {
		if f.StorageKey == storageKey {
			return f, nil
		}
	}
	return db.File{}, errors.New("file not found")
}

func (m *MockQuerier) ListFilesByUser(ctx context.Context, arg db.ListFilesByUserParams) ([]db.File, error) {
	if m.ListFilesErr != nil {
		return nil, m.ListFilesErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.File
	for _, f := range m.files {
		if f.UserID == arg.UserID && !f.DeletedAt.Valid {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *MockQuerier) ListVariantsByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.FileVariant
	for _, v := range m.variants {
		if v.FileID == fileID {
			result = append(result, v)
		}
	}
	return result, nil
}

// MockStorage implements storage.Storage for testing
type MockStorage struct {
	mu          sync.RWMutex
	files       map[string][]byte
	UploadErr   error
	DownloadErr error
	DeleteErr   error
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		files: make(map[string][]byte),
	}
}

func (m *MockStorage) Upload(ctx context.Context, key string, r io.Reader, contentType string, size int64) error {
	if m.UploadErr != nil {
		return m.UploadErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	data, _ := io.ReadAll(r)
	m.files[key] = data
	return nil
}

func (m *MockStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.DownloadErr != nil {
		return nil, m.DownloadErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.files[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
	return nil
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[key]
	return ok, nil
}

func (m *MockStorage) GetPresignedURL(ctx context.Context, key string, expirySeconds int) (string, error) {
	return "http://localhost:9000/" + key, nil
}

func (m *MockStorage) HealthCheck(ctx context.Context) error {
	return nil
}

// Test helper to create handlers with mocks
func createTestHandlers() (*Handlers, *MockStorage, *MockQuerier) {
	stor := NewMockStorage()
	queries := NewMockQuerier()

	cfg := &Config{
		Storage: stor,
		Queries: nil, // Using mock querier
		BaseURL: "http://localhost:8080",
		Secure:  false,
	}

	h := NewHandlers(cfg, nil, nil, nil, nil)
	return h, stor, queries
}

// Test helper to create handlers with a mock session user
func createAuthenticatedRequest(r *http.Request, user *auth.SessionUser) *http.Request {
	ctx := context.WithValue(r.Context(), auth.UserContextKey, user)
	return r.WithContext(ctx)
}

// createMockUser creates a mock SessionUser for testing
func createMockUser() *auth.SessionUser {
	return &auth.SessionUser{
		ID:        uuid.New(),
		Email:     "test@example.com",
		Name:      "Test User",
		AvatarURL: nil,
		Role:      db.UserRoleUser,
		SessionID: uuid.New(),
	}
}

func TestHomeHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("renders_landing_page", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "<html") {
			t.Error("response should contain HTML")
		}
		if !strings.Contains(body, "file.cheap") {
			t.Error("response should contain 'file.cheap' title")
		}
	})

	t.Run("includes_navigation", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		body := rec.Body.String()
		// Should have sign in and get started buttons for unauthenticated users
		if !strings.Contains(body, "Sign in") {
			t.Error("unauthenticated home should have 'Sign in' link")
		}
	})

	t.Run("shows_user_menu_when_authenticated", func(t *testing.T) {
		user := createMockUser()
		req := httptest.NewRequest("GET", "/", nil)
		req = createAuthenticatedRequest(req, user)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, user.Name) {
			t.Errorf("authenticated home should show user name '%s'", user.Name)
		}
	})
}

func TestLoginHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("renders_login_form", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/login", nil)
		rec := httptest.NewRecorder()

		h.Login(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "<form") {
			t.Error("login page should contain a form")
		}
		if !strings.Contains(body, "email") {
			t.Error("login page should have email input")
		}
		if !strings.Contains(body, "password") {
			t.Error("login page should have password input")
		}
	})

	t.Run("includes_return_url", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/login?return=/dashboard", nil)
		rec := httptest.NewRecorder()

		h.Login(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "/dashboard") {
			t.Error("login page should preserve return URL")
		}
	})

	t.Run("includes_register_link", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/login", nil)
		rec := httptest.NewRecorder()

		h.Login(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "register") || !strings.Contains(body, "Sign up") {
			t.Error("login page should have link to register")
		}
	})
}

func TestRegisterHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("renders_register_form", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/register", nil)
		rec := httptest.NewRecorder()

		h.Register(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "<form") {
			t.Error("register page should contain a form")
		}
		if !strings.Contains(body, "name") {
			t.Error("register page should have name input")
		}
		if !strings.Contains(body, "email") {
			t.Error("register page should have email input")
		}
		if !strings.Contains(body, "password") {
			t.Error("register page should have password input")
		}
	})

	t.Run("includes_login_link", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/register", nil)
		rec := httptest.NewRecorder()

		h.Register(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "login") || !strings.Contains(body, "Sign in") {
			t.Error("register page should have link to login")
		}
	})
}

func TestForgotPasswordHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("renders_forgot_password_form", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/forgot-password", nil)
		rec := httptest.NewRecorder()

		h.ForgotPassword(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "<form") {
			t.Error("forgot password page should contain a form")
		}
		if !strings.Contains(body, "email") {
			t.Error("forgot password page should have email input")
		}
	})

	t.Run("includes_back_to_login_link", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/forgot-password", nil)
		rec := httptest.NewRecorder()

		h.ForgotPassword(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "login") || !strings.Contains(body, "sign in") {
			t.Error("forgot password page should have link back to login")
		}
	})
}

func TestDashboardHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("redirects_when_not_authenticated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/dashboard", nil)
		rec := httptest.NewRecorder()

		h.Dashboard(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want 302", rec.Code)
		}

		location := rec.Header().Get("Location")
		if !strings.Contains(location, "/login") {
			t.Errorf("should redirect to login, got: %s", location)
		}
	})

	t.Run("renders_dashboard_when_authenticated", func(t *testing.T) {
		user := createMockUser()
		req := httptest.NewRequest("GET", "/dashboard", nil)
		req = createAuthenticatedRequest(req, user)
		rec := httptest.NewRecorder()

		h.Dashboard(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, user.Name) {
			t.Error("dashboard should show user name")
		}
		if !strings.Contains(body, "Dashboard") {
			t.Error("dashboard should have Dashboard title")
		}
	})
}

func TestUploadPageHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("redirects_when_not_authenticated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/upload", nil)
		rec := httptest.NewRecorder()

		h.UploadPage(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want 302", rec.Code)
		}
	})

	t.Run("renders_upload_page_when_authenticated", func(t *testing.T) {
		user := createMockUser()
		req := httptest.NewRequest("GET", "/upload", nil)
		req = createAuthenticatedRequest(req, user)
		rec := httptest.NewRecorder()

		h.UploadPage(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "Upload") {
			t.Error("upload page should have Upload title")
		}
		// Check for file input in the template
		if !strings.Contains(body, "file") {
			t.Error("upload page should have file input")
		}
	})
}

func TestLogoutHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("redirects_to_home", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/logout", nil)
		rec := httptest.NewRecorder()

		h.Logout(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want 302", rec.Code)
		}

		location := rec.Header().Get("Location")
		if location != "/" {
			t.Errorf("should redirect to /, got: %s", location)
		}
	})
}

func TestFileListHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("redirects_when_not_authenticated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files", nil)
		rec := httptest.NewRecorder()

		h.FileList(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want 302", rec.Code)
		}
	})
}

func TestFileDetailHandler(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("redirects_when_not_authenticated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files/"+uuid.New().String(), nil)
		rec := httptest.NewRecorder()

		h.FileDetail(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want 302", rec.Code)
		}
	})
}

func TestHandlersWithTemplRendering(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("home_page_renders_valid_html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		body := rec.Body.String()
		bodyLower := strings.ToLower(body)

		// Check for basic HTML structure (case-insensitive for DOCTYPE)
		if !strings.Contains(bodyLower, "<!doctype html>") {
			t.Error("response should have DOCTYPE")
		}
		if !strings.Contains(body, "<html") {
			t.Error("response should have html tag")
		}
		if !strings.Contains(body, "</html>") {
			t.Error("response should close html tag")
		}
		if !strings.Contains(body, "<head>") {
			t.Error("response should have head tag")
		}
		if !strings.Contains(body, "<body") {
			t.Error("response should have body tag")
		}
	})

	t.Run("pages_include_tailwind", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "output.css") {
			t.Error("pages should include Tailwind CSS")
		}
	})

	t.Run("pages_include_htmx", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "htmx") {
			t.Error("pages should include HTMX")
		}
	})

	t.Run("pages_include_alpine", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "alpine") {
			t.Error("pages should include Alpine.js")
		}
	})
}

func TestNordThemeColors(t *testing.T) {
	h, _, _ := createTestHandlers()

	t.Run("pages_use_nord_colors", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		h.Home(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "nord") {
			t.Error("pages should use Nord color palette")
		}
	})
}

func TestVideoEmbedHandler(t *testing.T) {
	t.Run("invalid_file_id_shows_error", func(t *testing.T) {
		h, _, _ := createTestHandlers()
		req := httptest.NewRequest("GET", "/embed/invalid-uuid", nil)
		req.SetPathValue("id", "invalid-uuid")
		rec := httptest.NewRecorder()

		h.VideoEmbed(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Invalid video ID") {
			t.Error("should show invalid ID error")
		}
	})

	t.Run("no_queries_shows_service_unavailable", func(t *testing.T) {
		h, _, _ := createTestHandlers()
		fileID := uuid.New()
		req := httptest.NewRequest("GET", "/embed/"+fileID.String(), nil)
		req.SetPathValue("id", fileID.String())
		rec := httptest.NewRecorder()

		h.VideoEmbed(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Service unavailable") {
			t.Error("should show service unavailable error")
		}
	})

	t.Run("embed_page_renders_html", func(t *testing.T) {
		h, _, _ := createTestHandlers()
		req := httptest.NewRequest("GET", "/embed/invalid", nil)
		req.SetPathValue("id", "invalid")
		rec := httptest.NewRecorder()

		h.VideoEmbed(rec, req)

		body := rec.Body.String()
		bodyLower := strings.ToLower(body)
		if !strings.Contains(bodyLower, "<!doctype html>") {
			t.Error("should render HTML document")
		}
		if !strings.Contains(body, "output.css") {
			t.Error("should include Tailwind CSS")
		}
		if !strings.Contains(body, "plyr") {
			t.Error("should include Plyr player scripts")
		}
		if !strings.Contains(body, "hls") {
			t.Error("should include HLS.js")
		}
	})

	t.Run("embed_page_includes_video_player_script", func(t *testing.T) {
		h, _, _ := createTestHandlers()
		req := httptest.NewRequest("GET", "/embed/test", nil)
		req.SetPathValue("id", uuid.New().String())
		rec := httptest.NewRecorder()

		h.VideoEmbed(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "videoPlayer") {
			t.Error("should include videoPlayer Alpine component")
		}
	})
}
