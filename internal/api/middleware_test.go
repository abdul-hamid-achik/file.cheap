package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestAuthMiddleware tests JWT authentication middleware.
// See docs/06-phase6-api.md for implementation guidance.
func TestAuthMiddleware(t *testing.T) {
	testUserID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name          string
		setupAuth     func(req *http.Request)
		jwtSecret     string
		wantStatus    int
		wantUserID    bool
		wantUserIDVal uuid.UUID
	}{
		{
			name: "valid token - user ID extracted",
			setupAuth: func(req *http.Request) {
				token := createValidToken(t, testUserID, testJWTSecret, 1*time.Hour)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			jwtSecret:     testJWTSecret,
			wantStatus:    http.StatusOK,
			wantUserID:    true,
			wantUserIDVal: testUserID,
		},
		{
			name: "missing authorization header",
			setupAuth: func(req *http.Request) {
				// No auth header
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "empty authorization header",
			setupAuth: func(req *http.Request) {
				req.Header.Set("Authorization", "")
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "invalid format - no Bearer prefix",
			setupAuth: func(req *http.Request) {
				token := createValidToken(t, testUserID, testJWTSecret, 1*time.Hour)
				req.Header.Set("Authorization", token) // Missing "Bearer "
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "invalid format - Basic auth",
			setupAuth: func(req *http.Request) {
				req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "invalid token - malformed",
			setupAuth: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer not.a.valid.jwt.token")
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "invalid token - wrong secret",
			setupAuth: func(req *http.Request) {
				token := createValidToken(t, testUserID, "wrong-secret", 1*time.Hour)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "expired token",
			setupAuth: func(req *http.Request) {
				token := createExpiredToken(t, testUserID, testJWTSecret)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "token missing subject claim",
			setupAuth: func(req *http.Request) {
				token := createTokenWithoutSubject(t, testJWTSecret)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "token with invalid user ID format",
			setupAuth: func(req *http.Request) {
				token := createTokenWithInvalidSubject(t, testJWTSecret, "not-a-uuid")
				req.Header.Set("Authorization", "Bearer "+token)
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "token with empty subject",
			setupAuth: func(req *http.Request) {
				token := createTokenWithInvalidSubject(t, testJWTSecret, "")
				req.Header.Set("Authorization", "Bearer "+token)
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized,
			wantUserID: false,
		},
		{
			name: "Bearer with extra spaces",
			setupAuth: func(req *http.Request) {
				token := createValidToken(t, testUserID, testJWTSecret, 1*time.Hour)
				req.Header.Set("Authorization", "Bearer  "+token) // Double space
			},
			jwtSecret:  testJWTSecret,
			wantStatus: http.StatusUnauthorized, // Should reject malformed header
			wantUserID: false,
		},
		{
			name: "token just about to expire (still valid)",
			setupAuth: func(req *http.Request) {
				token := createValidToken(t, testUserID, testJWTSecret, 1*time.Second)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			jwtSecret:     testJWTSecret,
			wantStatus:    http.StatusOK,
			wantUserID:    true,
			wantUserIDVal: testUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler that captures the user ID from context
			var capturedUserID uuid.UUID
			var hasUserID bool

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedUserID, hasUserID = GetUserID(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			})

			// Wrap with auth middleware
			handler := AuthMiddleware(tt.jwtSecret)(nextHandler)

			// Create request
			req := httptest.NewRequest("GET", "/test", nil)
			tt.setupAuth(req)

			// Record response
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// Check status
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			// Check user ID in context (only if request was successful)
			if tt.wantStatus == http.StatusOK {
				if tt.wantUserID && !hasUserID {
					t.Error("expected user ID in context, but not found")
				}
				if !tt.wantUserID && hasUserID {
					t.Error("unexpected user ID in context")
				}
				if tt.wantUserID && hasUserID && capturedUserID != tt.wantUserIDVal {
					t.Errorf("user ID = %s, want %s", capturedUserID, tt.wantUserIDVal)
				}
			}
		})
	}
}

// TestCORS tests CORS middleware with origin validation.
func TestCORS(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		origin      string
		devMode     bool
		wantHeaders map[string]string
		wantStatus  int
	}{
		{
			name:    "preflight OPTIONS request from allowed origin",
			method:  "OPTIONS",
			origin:  "https://file.cheap",
			devMode: false,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin":  "https://file.cheap",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
				"Access-Control-Allow-Headers": "Accept, Authorization, Content-Type",
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:    "preflight OPTIONS from localhost in dev mode",
			method:  "OPTIONS",
			origin:  "http://localhost:3000",
			devMode: true,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin":  "http://localhost:3000",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:    "regular GET request from allowed origin",
			method:  "GET",
			origin:  "https://api.file.cheap",
			devMode: false,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://api.file.cheap",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "POST request from disallowed origin",
			method:  "POST",
			origin:  "https://example.com",
			devMode: false,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "request without origin header",
			method:  "GET",
			origin:  "",
			devMode: false,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "DELETE request from allowed origin",
			method:  "DELETE",
			origin:  "https://www.file.cheap",
			devMode: false,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://www.file.cheap",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "preflight OPTIONS from disallowed origin",
			method:  "OPTIONS",
			origin:  "https://evil.com",
			devMode: false,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "",
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			})

			handler := CORSWithOrigins(tt.devMode)(nextHandler)

			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			for header, want := range tt.wantHeaders {
				got := rec.Header().Get(header)
				if got != want {
					t.Errorf("header %s = %q, want %q", header, got, want)
				}
			}
		})
	}
}

// TestCORSWithMaxAge tests that CORS sets Access-Control-Max-Age header.
func TestCORSWithMaxAge(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Use dev mode to allow localhost
	handler := CORSWithOrigins(true)(nextHandler)

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	maxAge := rec.Header().Get("Access-Control-Max-Age")
	if maxAge != "3600" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", maxAge, "3600")
	}
}

// TestRateLimiter tests rate limiting.
func TestRateLimiter(t *testing.T) {
	tests := []struct {
		name         string
		rate         int
		burst        int
		requests     int
		wantAllowed  int
		wantRejected int
	}{
		{
			name:         "all requests within limit",
			rate:         10,
			burst:        10,
			requests:     5,
			wantAllowed:  5,
			wantRejected: 0,
		},
		{
			name:         "requests exceed limit",
			rate:         5,
			burst:        5,
			requests:     10,
			wantAllowed:  5,
			wantRejected: 5,
		},
		{
			name:         "burst capacity allows initial spike",
			rate:         2,
			burst:        5,
			requests:     5,
			wantAllowed:  5,
			wantRejected: 0,
		},
		{
			name:         "single request",
			rate:         10,
			burst:        10,
			requests:     1,
			wantAllowed:  1,
			wantRejected: 0,
		},
		{
			name:         "zero burst rejects all",
			rate:         10,
			burst:        0,
			requests:     5,
			wantAllowed:  0,
			wantRejected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewRateLimiter(tt.rate, tt.burst)

			allowed := 0
			for i := 0; i < tt.requests; i++ {
				if limiter.Allow("test-key") {
					allowed++
				}
			}

			if allowed != tt.wantAllowed {
				t.Errorf("allowed = %d, want %d", allowed, tt.wantAllowed)
			}

			rejected := tt.requests - allowed
			if rejected != tt.wantRejected {
				t.Errorf("rejected = %d, want %d", rejected, tt.wantRejected)
			}
		})
	}
}

// TestRateLimiterPerKey tests that rate limits are per-key.
func TestRateLimiterPerKey(t *testing.T) {
	limiter := NewRateLimiter(2, 2)

	// Exhaust limit for key1
	if !limiter.Allow("key1") {
		t.Error("key1 first request should be allowed")
	}
	if !limiter.Allow("key1") {
		t.Error("key1 second request should be allowed")
	}
	if limiter.Allow("key1") {
		t.Error("key1 third request should be rejected")
	}

	// key2 should have its own limit
	if !limiter.Allow("key2") {
		t.Error("key2 first request should be allowed")
	}
	if !limiter.Allow("key2") {
		t.Error("key2 second request should be allowed")
	}
}

// TestRateLimitMiddleware tests the rate limit HTTP middleware.
func TestRateLimitMiddleware(t *testing.T) {
	limiter := NewRateLimiter(2, 2)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	handler := RateLimit(limiter)(nextHandler)

	// First two requests should pass
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("third request: status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

// TestRateLimitMiddlewareUsesIP tests that rate limit uses client IP.
func TestRateLimitMiddlewareUsesIP(t *testing.T) {
	limiter := NewRateLimiter(1, 1)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimit(limiter)(nextHandler)

	// Request from IP1
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("IP1 first request: status = %d, want %d", rec1.Code, http.StatusOK)
	}

	// Request from different IP should have its own limit
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:12345"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("IP2 first request: status = %d, want %d", rec2.Code, http.StatusOK)
	}
}

// TestGetUserID tests extracting user ID from context.
func TestGetUserID(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func() context.Context
		wantOK     bool
		wantUserID uuid.UUID
	}{
		{
			name: "user ID present",
			setupCtx: func() context.Context {
				userID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
				return context.WithValue(context.Background(), UserIDKey, userID)
			},
			wantOK:     true,
			wantUserID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		},
		{
			name: "user ID missing",
			setupCtx: func() context.Context {
				return context.Background()
			},
			wantOK: false,
		},
		{
			name: "wrong type in context",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), UserIDKey, "not-a-uuid")
			},
			wantOK: false,
		},
		{
			name: "nil UUID in context",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), UserIDKey, uuid.Nil)
			},
			wantOK:     true,
			wantUserID: uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			userID, ok := GetUserID(ctx)

			if ok != tt.wantOK {
				t.Errorf("GetUserID() ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && userID != tt.wantUserID {
				t.Errorf("GetUserID() = %v, want %v", userID, tt.wantUserID)
			}
		})
	}
}

// TestExtractBearerToken tests the helper function.
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		want       string
	}{
		{
			name:       "valid Bearer token",
			authHeader: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			want:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name:       "empty header",
			authHeader: "",
			want:       "",
		},
		{
			name:       "no Bearer prefix",
			authHeader: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			want:       "",
		},
		{
			name:       "Basic auth",
			authHeader: "Basic dXNlcjpwYXNz",
			want:       "",
		},
		{
			name:       "lowercase bearer",
			authHeader: "bearer token",
			want:       "", // Should be case-sensitive
		},
		{
			name:       "Bearer only",
			authHeader: "Bearer ",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBearerToken(tt.authHeader)
			if got != tt.want {
				t.Errorf("extractBearerToken(%q) = %q, want %q", tt.authHeader, got, tt.want)
			}
		})
	}
}

// Helper functions for creating test JWT tokens

func createValidToken(t *testing.T, userID uuid.UUID, secret string, expiry time.Duration) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": userID.String(),
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(expiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to create valid token: %v", err)
	}
	return tokenString
}

func createExpiredToken(t *testing.T, userID uuid.UUID, secret string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": userID.String(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-1 * time.Hour).Unix(), // Expired 1 hour ago
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to create expired token: %v", err)
	}
	return tokenString
}

func createTokenWithoutSubject(t *testing.T, secret string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		// No "sub" claim
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to create token without subject: %v", err)
	}
	return tokenString
}

func createTokenWithInvalidSubject(t *testing.T, secret string, subject string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": subject,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to create token with invalid subject: %v", err)
	}
	return tokenString
}

// Billing Middleware Tests

type mockBillingQuerier struct {
	billingInfo db.GetUserBillingInfoRow
	filesCount  int64
	err         error
}

func (m *mockBillingQuerier) GetUserBillingInfo(ctx context.Context, id pgtype.UUID) (db.GetUserBillingInfoRow, error) {
	return m.billingInfo, m.err
}

func (m *mockBillingQuerier) GetUserFilesCount(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return m.filesCount, m.err
}

func TestBillingMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		tier           db.SubscriptionTier
		method         string
		wantStatusCode int
		wantBilling    bool
	}{
		{
			name:           "free_can_read",
			tier:           db.SubscriptionTierFree,
			method:         http.MethodGet,
			wantStatusCode: http.StatusOK,
			wantBilling:    true,
		},
		{
			name:           "free_cannot_write",
			tier:           db.SubscriptionTierFree,
			method:         http.MethodPost,
			wantStatusCode: http.StatusForbidden,
			wantBilling:    false,
		},
		{
			name:           "pro_can_read",
			tier:           db.SubscriptionTierPro,
			method:         http.MethodGet,
			wantStatusCode: http.StatusOK,
			wantBilling:    true,
		},
		{
			name:           "pro_can_write",
			tier:           db.SubscriptionTierPro,
			method:         http.MethodPost,
			wantStatusCode: http.StatusOK,
			wantBilling:    true,
		},
		{
			name:           "free_cannot_delete",
			tier:           db.SubscriptionTierFree,
			method:         http.MethodDelete,
			wantStatusCode: http.StatusForbidden,
			wantBilling:    false,
		},
		{
			name:           "pro_can_delete",
			tier:           db.SubscriptionTierPro,
			method:         http.MethodDelete,
			wantStatusCode: http.StatusOK,
			wantBilling:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := &mockBillingQuerier{
				billingInfo: db.GetUserBillingInfoRow{
					SubscriptionTier:   tt.tier,
					SubscriptionStatus: db.SubscriptionStatusActive,
					FilesLimit:         100,
					MaxFileSize:        10 * 1024 * 1024,
				},
				filesCount: 50,
			}

			var gotBilling *BillingInfo
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotBilling = GetBilling(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			middleware := BillingMiddleware(mockQueries)

			req := httptest.NewRequest(tt.method, "/test", nil)
			userID := uuid.New()
			ctx := context.WithValue(req.Context(), UserIDKey, userID)
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			middleware(handler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.wantBilling && gotBilling == nil {
				t.Error("expected billing info in context, got nil")
			}
			if !tt.wantBilling && gotBilling != nil && rec.Code == http.StatusOK {
				t.Error("expected no billing info, but got one")
			}
		})
	}
}

func TestBillingMiddleware_NoAuth(t *testing.T) {
	mockQueries := &mockBillingQuerier{}

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := BillingMiddleware(mockQueries)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called when no auth")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestBillingMiddleware_NilQueries(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := BillingMiddleware(nil)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	userID := uuid.New()
	ctx := context.WithValue(req.Context(), UserIDKey, userID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called when queries is nil")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestGetBilling(t *testing.T) {
	t.Run("returns_billing_when_present", func(t *testing.T) {
		billing := &BillingInfo{
			Tier:       db.SubscriptionTierPro,
			FilesLimit: 2000,
		}
		ctx := context.WithValue(context.Background(), BillingKey, billing)

		got := GetBilling(ctx)
		if got == nil {
			t.Fatal("expected billing, got nil")
		}
		if got.Tier != db.SubscriptionTierPro {
			t.Errorf("tier = %s, want pro", got.Tier)
		}
	})

	t.Run("returns_nil_when_missing", func(t *testing.T) {
		got := GetBilling(context.Background())
		if got != nil {
			t.Error("expected nil, got billing")
		}
	})
}

func TestIsReadOnlyMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{http.MethodGet, true},
		{http.MethodHead, true},
		{http.MethodOptions, true},
		{http.MethodPost, false},
		{http.MethodPut, false},
		{http.MethodPatch, false},
		{http.MethodDelete, false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := isReadOnlyMethod(tt.method); got != tt.want {
				t.Errorf("isReadOnlyMethod(%s) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}
