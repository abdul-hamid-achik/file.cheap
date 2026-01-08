package web

import (
	"net/http"
	"strings"

	"github.com/abdul-hamid-achik/file-processor/internal/apperror"
	"github.com/abdul-hamid-achik/file-processor/internal/auth"
	"github.com/jackc/pgx/v5/pgtype"
)

const linkStatePrefix = "link:"

func (h *Handlers) OAuthGoogleStart(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsGoogleConfigured() {
		apperror.WriteHTTP(w, r, apperror.ErrServiceUnavailable)
		return
	}

	state, err := auth.GenerateState()
	if err != nil {
		apperror.WriteHTTP(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	url, err := h.oauthService.GetGoogleAuthURL(state)
	if err != nil {
		apperror.WriteHTTP(w, r, err)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handlers) OAuthGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsGoogleConfigured() {
		apperror.WriteHTTP(w, r, apperror.ErrServiceUnavailable)
		return
	}

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Redirect(w, r, "/login?error=invalid_state", http.StatusFound)
		return
	}

	stateValue := stateCookie.Value
	isLinkingMode := strings.HasPrefix(stateValue, linkStatePrefix)
	expectedState := r.URL.Query().Get("state")

	if stateValue != expectedState {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error=invalid_state", http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error=invalid_state", http.StatusFound)
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error="+errParam, http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error="+errParam, http.StatusFound)
		}
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error=missing_code", http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error=missing_code", http.StatusFound)
		}
		return
	}

	userInfo, token, err := h.oauthService.ExchangeGoogleCode(r.Context(), code)
	if err != nil {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error="+apperror.Code(err), http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
		}
		return
	}

	if isLinkingMode {
		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login?error=session_expired", http.StatusFound)
			return
		}

		userID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if err := h.oauthService.LinkOAuthAccount(r.Context(), userID, userInfo, token); err != nil {
			http.Redirect(w, r, "/profile?error="+apperror.Code(err), http.StatusFound)
			return
		}

		if h.emailService != nil {
			_ = h.emailService.SendOAuthLinkedEmail(user.Email, user.Name, "Google", userInfo.Email)
		}

		http.Redirect(w, r, "/profile?success=google_linked", http.StatusFound)
		return
	}

	result, err := h.oauthService.FindOrCreateUser(r.Context(), userInfo, token)
	if err != nil {
		http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
		return
	}

	if err := h.sessionManager.CreateSession(r.Context(), w, r, result.User.ID.Bytes); err != nil {
		http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (h *Handlers) OAuthGitHubStart(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsGitHubConfigured() {
		apperror.WriteHTTP(w, r, apperror.ErrServiceUnavailable)
		return
	}

	state, err := auth.GenerateState()
	if err != nil {
		apperror.WriteHTTP(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	url, err := h.oauthService.GetGitHubAuthURL(state)
	if err != nil {
		apperror.WriteHTTP(w, r, err)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handlers) OAuthGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsGitHubConfigured() {
		apperror.WriteHTTP(w, r, apperror.ErrServiceUnavailable)
		return
	}

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Redirect(w, r, "/login?error=invalid_state", http.StatusFound)
		return
	}

	stateValue := stateCookie.Value
	isLinkingMode := strings.HasPrefix(stateValue, linkStatePrefix)
	expectedState := r.URL.Query().Get("state")

	if stateValue != expectedState {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error=invalid_state", http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error=invalid_state", http.StatusFound)
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error="+errParam, http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error="+errParam, http.StatusFound)
		}
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error=missing_code", http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error=missing_code", http.StatusFound)
		}
		return
	}

	userInfo, token, err := h.oauthService.ExchangeGitHubCode(r.Context(), code)
	if err != nil {
		if isLinkingMode {
			http.Redirect(w, r, "/profile?error="+apperror.Code(err), http.StatusFound)
		} else {
			http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
		}
		return
	}

	if isLinkingMode {
		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login?error=session_expired", http.StatusFound)
			return
		}

		userID := pgtype.UUID{Bytes: user.ID, Valid: true}
		if err := h.oauthService.LinkOAuthAccount(r.Context(), userID, userInfo, token); err != nil {
			http.Redirect(w, r, "/profile?error="+apperror.Code(err), http.StatusFound)
			return
		}

		if h.emailService != nil {
			_ = h.emailService.SendOAuthLinkedEmail(user.Email, user.Name, "GitHub", userInfo.Email)
		}

		http.Redirect(w, r, "/profile?success=github_linked", http.StatusFound)
		return
	}

	result, err := h.oauthService.FindOrCreateUser(r.Context(), userInfo, token)
	if err != nil {
		http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
		return
	}

	if err := h.sessionManager.CreateSession(r.Context(), w, r, result.User.ID.Bytes); err != nil {
		http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (h *Handlers) LinkOAuthGoogleStart(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsGoogleConfigured() {
		http.Redirect(w, r, "/profile?error=oauth_not_configured", http.StatusFound)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile?error=invalid_form", http.StatusFound)
		return
	}

	password := r.FormValue("password")
	if user.HasPassword && password != "" {
		if err := h.authService.VerifyPassword(r.Context(), user.ID, password); err != nil {
			http.Redirect(w, r, "/profile?error=invalid_password", http.StatusFound)
			return
		}
	}

	state, err := auth.GenerateState()
	if err != nil {
		http.Redirect(w, r, "/profile?error=internal", http.StatusFound)
		return
	}

	linkState := linkStatePrefix + state

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    linkState,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	url, err := h.oauthService.GetGoogleAuthURL(linkState)
	if err != nil {
		http.Redirect(w, r, "/profile?error=internal", http.StatusFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handlers) LinkOAuthGitHubStart(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsGitHubConfigured() {
		http.Redirect(w, r, "/profile?error=oauth_not_configured", http.StatusFound)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile?error=invalid_form", http.StatusFound)
		return
	}

	password := r.FormValue("password")
	if user.HasPassword && password != "" {
		if err := h.authService.VerifyPassword(r.Context(), user.ID, password); err != nil {
			http.Redirect(w, r, "/profile?error=invalid_password", http.StatusFound)
			return
		}
	}

	state, err := auth.GenerateState()
	if err != nil {
		http.Redirect(w, r, "/profile?error=internal", http.StatusFound)
		return
	}

	linkState := linkStatePrefix + state

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    linkState,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	url, err := h.oauthService.GetGitHubAuthURL(linkState)
	if err != nil {
		http.Redirect(w, r, "/profile?error=internal", http.StatusFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}
