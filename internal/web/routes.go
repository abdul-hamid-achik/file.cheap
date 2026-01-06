package web

import (
	"net/http"

	"github.com/abdul-hamid-achik/file-processor/internal/auth"
	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/storage"
)

type Config struct {
	Storage storage.Storage
	Queries *db.Queries
	BaseURL string
	Secure  bool
}

func NewRouter(cfg *Config, sm *auth.SessionManager, authSvc *auth.Service, oauthSvc *auth.OAuthService) http.Handler {
	mux := http.NewServeMux()
	h := NewHandlers(cfg, sm, authSvc, oauthSvc)

	if sm != nil {
		mux.Handle("GET /", auth.OptionalAuth(sm)(http.HandlerFunc(h.Home)))
	} else {
		mux.HandleFunc("GET /", h.Home)
	}

	if sm != nil {
		authRedirect := auth.RedirectIfAuthenticated(sm)
		mux.Handle("GET /login", authRedirect(http.HandlerFunc(h.Login)))
		mux.Handle("GET /register", authRedirect(http.HandlerFunc(h.Register)))
		mux.Handle("GET /forgot-password", authRedirect(http.HandlerFunc(h.ForgotPassword)))
	} else {
		mux.HandleFunc("GET /login", h.Login)
		mux.HandleFunc("GET /register", h.Register)
		mux.HandleFunc("GET /forgot-password", h.ForgotPassword)
	}
	mux.HandleFunc("POST /login", h.LoginPost)
	mux.HandleFunc("POST /register", h.RegisterPost)
	mux.HandleFunc("POST /logout", h.Logout)
	mux.HandleFunc("POST /forgot-password", h.ForgotPasswordPost)

	if sm != nil {
		requireAuth := auth.RequireAuth(sm)
		mux.Handle("GET /dashboard", requireAuth(http.HandlerFunc(h.Dashboard)))
		mux.Handle("GET /upload", requireAuth(http.HandlerFunc(h.UploadPage)))
		mux.Handle("POST /upload", requireAuth(http.HandlerFunc(h.UploadFile)))
		mux.Handle("POST /api/files", requireAuth(http.HandlerFunc(h.UploadFileAPI)))
		mux.Handle("GET /files", requireAuth(http.HandlerFunc(h.FileList)))
		mux.Handle("GET /files/{id}", requireAuth(http.HandlerFunc(h.FileDetail)))
		mux.Handle("POST /files/{id}/delete", requireAuth(http.HandlerFunc(h.DeleteFile)))
		mux.Handle("GET /profile", requireAuth(http.HandlerFunc(h.Profile)))
		mux.Handle("POST /profile", requireAuth(http.HandlerFunc(h.ProfilePost)))
		mux.Handle("POST /profile/avatar", requireAuth(http.HandlerFunc(h.ProfileAvatar)))
		mux.Handle("POST /profile/delete", requireAuth(http.HandlerFunc(h.ProfileDelete)))
		mux.Handle("GET /settings", requireAuth(http.HandlerFunc(h.Settings)))
		mux.Handle("POST /settings/password", requireAuth(http.HandlerFunc(h.SettingsPassword)))
		mux.Handle("POST /settings/notifications", requireAuth(http.HandlerFunc(h.SettingsNotifications)))
		mux.Handle("POST /settings/files", requireAuth(http.HandlerFunc(h.SettingsFiles)))
		mux.Handle("POST /settings/tokens", requireAuth(http.HandlerFunc(h.SettingsCreateToken)))
		mux.Handle("POST /settings/tokens/{id}/delete", requireAuth(http.HandlerFunc(h.SettingsDeleteToken)))
	} else {
		redirectToLogin := func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusFound)
		}
		mux.HandleFunc("GET /dashboard", redirectToLogin)
		mux.HandleFunc("GET /upload", redirectToLogin)
		mux.HandleFunc("POST /upload", redirectToLogin)
		mux.HandleFunc("GET /files", redirectToLogin)
		mux.HandleFunc("GET /files/{id}", redirectToLogin)
		mux.HandleFunc("POST /files/{id}/delete", redirectToLogin)
		mux.HandleFunc("GET /profile", redirectToLogin)
		mux.HandleFunc("POST /profile", redirectToLogin)
		mux.HandleFunc("POST /profile/avatar", redirectToLogin)
		mux.HandleFunc("POST /profile/delete", redirectToLogin)
		mux.HandleFunc("GET /settings", redirectToLogin)
		mux.HandleFunc("POST /settings/password", redirectToLogin)
		mux.HandleFunc("POST /settings/notifications", redirectToLogin)
		mux.HandleFunc("POST /settings/files", redirectToLogin)
		mux.HandleFunc("POST /settings/tokens", redirectToLogin)
		mux.HandleFunc("POST /settings/tokens/{id}/delete", redirectToLogin)
	}

	mux.HandleFunc("GET /verify-email", h.VerifyEmail)
	mux.HandleFunc("GET /resend-verification", h.ResendVerification)
	mux.HandleFunc("POST /resend-verification", h.ResendVerificationPost)

	if oauthSvc != nil {
		mux.HandleFunc("GET /auth/google", h.OAuthGoogleStart)
		mux.HandleFunc("GET /auth/google/callback", h.OAuthGoogleCallback)
		mux.HandleFunc("GET /auth/github", h.OAuthGitHubStart)
		mux.HandleFunc("GET /auth/github/callback", h.OAuthGitHubCallback)
	}

	if sm != nil {
		mux.Handle("GET /privacy", auth.OptionalAuth(sm)(http.HandlerFunc(h.Privacy)))
		mux.Handle("GET /terms", auth.OptionalAuth(sm)(http.HandlerFunc(h.Terms)))
		mux.Handle("GET /docs", auth.OptionalAuth(sm)(http.HandlerFunc(h.Docs)))
	} else {
		mux.HandleFunc("GET /privacy", h.Privacy)
		mux.HandleFunc("GET /terms", h.Terms)
		mux.HandleFunc("GET /docs", h.Docs)
	}

	fs := http.FileServer(http.Dir("static"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fs))

	return mux
}
