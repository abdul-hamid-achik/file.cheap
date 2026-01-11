package web

import (
	"net/http"

	"github.com/abdul-hamid-achik/file.cheap/internal/auth"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/email"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
)

type Broker interface {
	Enqueue(jobType string, payload interface{}) (string, error)
}

type Config struct {
	Storage storage.Storage
	Queries *db.Queries
	Broker  Broker
	BaseURL string
	Secure  bool
}

func NewRouter(cfg *Config, sm *auth.SessionManager, authSvc *auth.Service, oauthSvc *auth.OAuthService, emailSvc *email.Service, billingHandlers *BillingHandlers, analyticsHandlers *AnalyticsHandlers, adminHandlers *AdminHandlers) http.Handler {
	mux := http.NewServeMux()
	h := NewHandlers(cfg, sm, authSvc, oauthSvc, emailSvc)

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
	mux.HandleFunc("GET /reset-password", h.ResetPassword)
	mux.HandleFunc("POST /reset-password", h.ResetPasswordPost)

	if sm != nil {
		requireAuth := auth.RequireAuth(sm)
		mux.Handle("GET /dashboard", requireAuth(http.HandlerFunc(h.Dashboard)))
		mux.Handle("GET /upload", requireAuth(http.HandlerFunc(h.UploadPage)))
		mux.Handle("POST /upload", requireAuth(http.HandlerFunc(h.UploadFile)))
		mux.Handle("POST /files/upload", requireAuth(http.HandlerFunc(h.UploadFileAPI)))
		mux.Handle("GET /files", requireAuth(http.HandlerFunc(h.FileList)))
		mux.Handle("GET /files/{id}", requireAuth(http.HandlerFunc(h.FileDetail)))
		mux.Handle("GET /files/{id}/download", requireAuth(http.HandlerFunc(h.DownloadFile)))
		mux.Handle("POST /files/{id}/delete", requireAuth(http.HandlerFunc(h.DeleteFile)))
		mux.Handle("POST /files/{id}/process", requireAuth(http.HandlerFunc(h.ProcessFile)))
		mux.Handle("POST /files/{id}/process-bundle", requireAuth(http.HandlerFunc(h.ProcessBundle)))
		mux.Handle("GET /files/{id}/status", requireAuth(http.HandlerFunc(h.FileStatus)))
		mux.Handle("GET /files/{id}/info", requireAuth(http.HandlerFunc(h.FileInfo)))
		mux.Handle("GET /files/{id}/preview", requireAuth(http.HandlerFunc(h.FilePreview)))
		mux.Handle("POST /files/batch/delete", requireAuth(http.HandlerFunc(h.BatchDeleteFiles)))
		mux.Handle("POST /files/batch/process", requireAuth(http.HandlerFunc(h.BatchProcessFiles)))
		mux.Handle("GET /profile", requireAuth(http.HandlerFunc(h.Profile)))
		mux.Handle("POST /profile", requireAuth(http.HandlerFunc(h.ProfilePost)))
		mux.Handle("POST /profile/avatar", requireAuth(http.HandlerFunc(h.ProfileAvatar)))
		mux.Handle("POST /profile/delete", requireAuth(http.HandlerFunc(h.ProfileDelete)))
		mux.Handle("POST /profile/link/google", requireAuth(http.HandlerFunc(h.LinkOAuthGoogleStart)))
		mux.Handle("POST /profile/link/github", requireAuth(http.HandlerFunc(h.LinkOAuthGitHubStart)))
		mux.Handle("POST /profile/disconnect/{provider}", requireAuth(http.HandlerFunc(h.DisconnectOAuth)))
		mux.Handle("GET /settings", requireAuth(http.HandlerFunc(h.Settings)))
		mux.Handle("POST /settings/password", requireAuth(http.HandlerFunc(h.SettingsPassword)))
		mux.Handle("POST /settings/notifications", requireAuth(http.HandlerFunc(h.SettingsNotifications)))
		mux.Handle("POST /settings/files", requireAuth(http.HandlerFunc(h.SettingsFiles)))
		mux.Handle("POST /settings/tokens", requireAuth(http.HandlerFunc(h.SettingsCreateToken)))
		mux.Handle("POST /settings/tokens/{id}/delete", requireAuth(http.HandlerFunc(h.SettingsDeleteToken)))

		// Billing routes
		if billingHandlers != nil {
			mux.Handle("GET /billing", requireAuth(http.HandlerFunc(billingHandlers.Billing)))
			mux.Handle("POST /billing/trial", requireAuth(http.HandlerFunc(billingHandlers.StartTrial)))
			mux.Handle("POST /billing/checkout", requireAuth(http.HandlerFunc(billingHandlers.CreateCheckout)))
			mux.Handle("POST /billing/portal", requireAuth(http.HandlerFunc(billingHandlers.CreatePortal)))
		}

		// Analytics routes
		if analyticsHandlers != nil {
			mux.Handle("GET /dashboard/analytics", requireAuth(http.HandlerFunc(analyticsHandlers.Dashboard)))
			mux.Handle("GET /dashboard/analytics/chart", requireAuth(http.HandlerFunc(analyticsHandlers.ChartPartial)))
			mux.Handle("GET /dashboard/analytics/chart/usage", requireAuth(http.HandlerFunc(analyticsHandlers.UsageChart)))
			mux.Handle("GET /dashboard/analytics/chart/transforms", requireAuth(http.HandlerFunc(analyticsHandlers.TransformsChart)))
			mux.Handle("GET /dashboard/analytics/activity", requireAuth(http.HandlerFunc(analyticsHandlers.ActivityFeed)))
		}

		// Admin routes
		if adminHandlers != nil {
			requireAdmin := func(next http.Handler) http.Handler {
				return requireAuth(auth.RequireAdmin(next))
			}
			mux.Handle("GET /admin", requireAdmin(http.HandlerFunc(adminHandlers.Dashboard)))
			mux.Handle("GET /admin/chart/revenue", requireAdmin(http.HandlerFunc(adminHandlers.RevenueChart)))
			mux.Handle("GET /admin/health", requireAdmin(http.HandlerFunc(adminHandlers.HealthStatus)))
			mux.Handle("GET /admin/signups", requireAdmin(http.HandlerFunc(adminHandlers.RecentSignups)))
			mux.Handle("GET /admin/jobs", requireAdmin(http.HandlerFunc(adminHandlers.Jobs)))
			mux.Handle("POST /admin/jobs/{id}/retry", requireAdmin(http.HandlerFunc(adminHandlers.RetryJob)))
		}
	} else {
		redirectToLogin := func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusFound)
		}
		mux.HandleFunc("GET /dashboard", redirectToLogin)
		mux.HandleFunc("GET /upload", redirectToLogin)
		mux.HandleFunc("POST /upload", redirectToLogin)
		mux.HandleFunc("GET /files", redirectToLogin)
		mux.HandleFunc("GET /files/{id}", redirectToLogin)
		mux.HandleFunc("GET /files/{id}/download", redirectToLogin)
		mux.HandleFunc("POST /files/{id}/delete", redirectToLogin)
		mux.HandleFunc("POST /files/{id}/process", redirectToLogin)
		mux.HandleFunc("POST /files/{id}/process-bundle", redirectToLogin)
		mux.HandleFunc("GET /files/{id}/status", redirectToLogin)
		mux.HandleFunc("GET /files/{id}/info", redirectToLogin)
		mux.HandleFunc("GET /files/{id}/preview", redirectToLogin)
		mux.HandleFunc("POST /files/batch/delete", redirectToLogin)
		mux.HandleFunc("POST /files/batch/process", redirectToLogin)
		mux.HandleFunc("GET /profile", redirectToLogin)
		mux.HandleFunc("POST /profile", redirectToLogin)
		mux.HandleFunc("POST /profile/avatar", redirectToLogin)
		mux.HandleFunc("POST /profile/delete", redirectToLogin)
		mux.HandleFunc("POST /profile/link/google", redirectToLogin)
		mux.HandleFunc("POST /profile/link/github", redirectToLogin)
		mux.HandleFunc("POST /profile/disconnect/{provider}", redirectToLogin)
		mux.HandleFunc("GET /settings", redirectToLogin)
		mux.HandleFunc("POST /settings/password", redirectToLogin)
		mux.HandleFunc("POST /settings/notifications", redirectToLogin)
		mux.HandleFunc("POST /settings/files", redirectToLogin)
		mux.HandleFunc("POST /settings/tokens", redirectToLogin)
		mux.HandleFunc("POST /settings/tokens/{id}/delete", redirectToLogin)
		mux.HandleFunc("GET /billing", redirectToLogin)
		mux.HandleFunc("POST /billing/trial", redirectToLogin)
		mux.HandleFunc("POST /billing/checkout", redirectToLogin)
		mux.HandleFunc("POST /billing/portal", redirectToLogin)
		mux.HandleFunc("GET /dashboard/analytics", redirectToLogin)
		mux.HandleFunc("GET /dashboard/analytics/chart", redirectToLogin)
		mux.HandleFunc("GET /dashboard/analytics/chart/usage", redirectToLogin)
		mux.HandleFunc("GET /dashboard/analytics/chart/transforms", redirectToLogin)
		mux.HandleFunc("GET /dashboard/analytics/activity", redirectToLogin)
		mux.HandleFunc("GET /admin", redirectToLogin)
		mux.HandleFunc("GET /admin/chart/revenue", redirectToLogin)
		mux.HandleFunc("GET /admin/health", redirectToLogin)
		mux.HandleFunc("GET /admin/signups", redirectToLogin)
		mux.HandleFunc("GET /admin/jobs", redirectToLogin)
		mux.HandleFunc("POST /admin/jobs/{id}/retry", redirectToLogin)
	}

	// Stripe webhook (no auth required - Stripe sends directly)
	if billingHandlers != nil {
		mux.HandleFunc("POST /billing/webhook", billingHandlers.Webhook)
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

	// Public embed route (no auth required)
	mux.HandleFunc("GET /embed/{id}", h.VideoEmbed)

	return mux
}
