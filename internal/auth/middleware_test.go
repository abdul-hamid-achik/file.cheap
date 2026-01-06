package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/google/uuid"
)

func TestGetUserFromContext(t *testing.T) {
	t.Run("returns nil when no user in context", func(t *testing.T) {
		ctx := context.Background()
		user := GetUserFromContext(ctx)
		if user != nil {
			t.Error("expected nil user for empty context")
		}
	})

	t.Run("returns user when present in context", func(t *testing.T) {
		expectedUser := &SessionUser{
			ID:    uuid.New(),
			Email: "test@example.com",
			Name:  "Test User",
			Role:  db.UserRoleUser,
		}
		ctx := context.WithValue(context.Background(), UserContextKey, expectedUser)

		user := GetUserFromContext(ctx)
		if user == nil {
			t.Fatal("expected user to be present")
		}
		if user.Email != expectedUser.Email {
			t.Errorf("email = %s, want %s", user.Email, expectedUser.Email)
		}
	})

	t.Run("returns nil for wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), UserContextKey, "not a user")
		user := GetUserFromContext(ctx)
		if user != nil {
			t.Error("expected nil for wrong type in context")
		}
	})
}

func TestRequireEmailVerified(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	t.Run("redirects when no user in context", func(t *testing.T) {
		nextCalled = false
		handler := RequireEmailVerified(next)

		req := httptest.NewRequest("GET", "/protected", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
		}
		if nextCalled {
			t.Error("next handler should not be called")
		}
	})

	t.Run("redirects when email not verified", func(t *testing.T) {
		nextCalled = false
		user := &SessionUser{
			ID:              uuid.New(),
			Email:           "test@example.com",
			EmailVerifiedAt: nil, // Not verified
		}

		handler := RequireEmailVerified(next)
		req := httptest.NewRequest("GET", "/protected", nil)
		ctx := context.WithValue(req.Context(), UserContextKey, user)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
		}
		location := rec.Header().Get("Location")
		if location != "/verify-email" {
			t.Errorf("redirect = %s, want /verify-email", location)
		}
		if nextCalled {
			t.Error("next handler should not be called")
		}
	})

	t.Run("calls next when email verified", func(t *testing.T) {
		nextCalled = false
		now := time.Now()
		user := &SessionUser{
			ID:              uuid.New(),
			Email:           "test@example.com",
			EmailVerifiedAt: &now,
		}

		handler := RequireEmailVerified(next)
		req := httptest.NewRequest("GET", "/protected", nil)
		ctx := context.WithValue(req.Context(), UserContextKey, user)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !nextCalled {
			t.Error("next handler should be called")
		}
	})
}

func TestRequireAdmin(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	t.Run("redirects when no user in context", func(t *testing.T) {
		nextCalled = false
		handler := RequireAdmin(next)

		req := httptest.NewRequest("GET", "/admin", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
		}
		if nextCalled {
			t.Error("next handler should not be called")
		}
	})

	t.Run("returns forbidden for non-admin user", func(t *testing.T) {
		nextCalled = false
		user := &SessionUser{
			ID:   uuid.New(),
			Role: db.UserRoleUser,
		}

		handler := RequireAdmin(next)
		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), UserContextKey, user)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
		if nextCalled {
			t.Error("next handler should not be called")
		}
	})

	t.Run("calls next for admin user", func(t *testing.T) {
		nextCalled = false
		user := &SessionUser{
			ID:   uuid.New(),
			Role: db.UserRoleAdmin,
		}

		handler := RequireAdmin(next)
		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), UserContextKey, user)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !nextCalled {
			t.Error("next handler should be called")
		}
	})
}
