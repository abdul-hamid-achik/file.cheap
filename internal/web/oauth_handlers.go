package web

import (
	"net/http"

	"github.com/abdul-hamid-achik/file-processor/internal/apperror"
	"github.com/abdul-hamid-achik/file-processor/internal/auth"
)

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
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=invalid_state", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		http.Redirect(w, r, "/login?error="+errParam, http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login?error=missing_code", http.StatusFound)
		return
	}

	userInfo, token, err := h.oauthService.ExchangeGoogleCode(r.Context(), code)
	if err != nil {
		http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
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
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=invalid_state", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		http.Redirect(w, r, "/login?error="+errParam, http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login?error=missing_code", http.StatusFound)
		return
	}

	userInfo, token, err := h.oauthService.ExchangeGitHubCode(r.Context(), code)
	if err != nil {
		http.Redirect(w, r, "/login?error="+apperror.Code(err), http.StatusFound)
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
