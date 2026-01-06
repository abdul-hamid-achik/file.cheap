package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file-processor/internal/apperror"
	"github.com/abdul-hamid-achik/file-processor/internal/auth"
	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/logger"
	"github.com/abdul-hamid-achik/file-processor/internal/web/templates/pages"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handlers struct {
	cfg            *Config
	sessionManager *auth.SessionManager
	authService    *auth.Service
	oauthService   *auth.OAuthService
}

func NewHandlers(cfg *Config, sm *auth.SessionManager, authSvc *auth.Service, oauthSvc *auth.OAuthService) *Handlers {
	return &Handlers{
		cfg:            cfg,
		sessionManager: sm,
		authService:    authSvc,
		oauthService:   oauthSvc,
	}
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	pages.Landing(user).Render(r.Context(), w)
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	data := pages.LoginPageData{
		ReturnURL:     r.URL.Query().Get("return"),
		GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
		GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
	}
	pages.Login(data).Render(r.Context(), w)
}

func (h *Handlers) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		data := pages.LoginPageData{
			Error:         apperror.SafeMessage(apperror.Wrap(err, apperror.ErrBadRequest)),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		pages.Login(data).Render(r.Context(), w)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	returnURL := r.FormValue("return")

	user, err := h.authService.Login(r.Context(), auth.LoginInput{
		Email:    email,
		Password: password,
	})
	if err != nil {
		data := pages.LoginPageData{
			Error:         apperror.SafeMessage(err),
			ReturnURL:     returnURL,
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		pages.Login(data).Render(r.Context(), w)
		return
	}

	if err := h.sessionManager.CreateSession(r.Context(), w, r, user.ID.Bytes); err != nil {
		data := pages.LoginPageData{
			Error:         apperror.SafeMessage(err),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		pages.Login(data).Render(r.Context(), w)
		return
	}

	if returnURL == "" {
		returnURL = "/dashboard"
	}
	http.Redirect(w, r, returnURL, http.StatusFound)
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	data := pages.RegisterPageData{
		GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
		GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
	}
	pages.Register(data).Render(r.Context(), w)
}

func (h *Handlers) RegisterPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		data := pages.RegisterPageData{
			Error:         apperror.SafeMessage(apperror.Wrap(err, apperror.ErrBadRequest)),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		pages.Register(data).Render(r.Context(), w)
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	password := r.FormValue("password")
	passwordConfirm := r.FormValue("password_confirm")

	if password != passwordConfirm {
		data := pages.RegisterPageData{
			Error:         apperror.ErrPasswordMismatch.Message,
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		pages.Register(data).Render(r.Context(), w)
		return
	}

	result, err := h.authService.Register(r.Context(), auth.RegisterInput{
		Email:    email,
		Password: password,
		Name:     name,
	})
	if err != nil {
		data := pages.RegisterPageData{
			Error:         apperror.SafeMessage(err),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		pages.Register(data).Render(r.Context(), w)
		return
	}

	if err := h.sessionManager.CreateSession(r.Context(), w, r, result.User.ID.Bytes); err != nil {
		data := pages.RegisterPageData{
			Error:         apperror.SafeMessage(err),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		pages.Register(data).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	h.sessionManager.DeleteSession(r.Context(), w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handlers) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	pages.ForgotPassword(pages.ForgotPasswordPageData{}).Render(r.Context(), w)
}

func (h *Handlers) ForgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		data := pages.ForgotPasswordPageData{
			Error: apperror.SafeMessage(apperror.Wrap(err, apperror.ErrBadRequest)),
		}
		pages.ForgotPassword(data).Render(r.Context(), w)
		return
	}

	email := r.FormValue("email")
	h.authService.RequestPasswordReset(r.Context(), email)

	data := pages.ForgotPasswordPageData{
		Success: "If an account exists with this email, you will receive a password reset link.",
	}
	pages.ForgotPassword(data).Render(r.Context(), w)
}

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data := pages.DashboardPageData{
		Stats: pages.DashboardStats{
			TotalFiles:     0,
			ProcessedFiles: 0,
			PendingJobs:    0,
			StorageUsed:    "0 MB",
			StorageTrend:   "",
			StorageTrendUp: false,
		},
		RecentFiles: []pages.RecentFile{},
	}

	pages.Dashboard(user, data).Render(r.Context(), w)
}

func (h *Handlers) UploadPage(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data := pages.UploadPageData{}
	pages.Upload(user, data).Render(r.Context(), w)
}

func (h *Handlers) UploadFile(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/files", http.StatusFound)
}

func (h *Handlers) UploadFileAPI(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
		return
	}

	log = log.With("user_id", user.ID.String())

	maxSize := int64(100 * 1024 * 1024)
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrFileTooLarge))
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "missing_file", "Please select at least one file to upload", http.StatusBadRequest))
		return
	}

	var results []map[string]any

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			log.Error("failed to open uploaded file", "filename", fileHeader.Filename, "error", err)
			continue
		}

		fileID := uuid.New()
		storageKey := fmt.Sprintf("uploads/%s/%s/%s", user.ID.String(), fileID.String(), fileHeader.Filename)

		log.Info("uploading file", "filename", fileHeader.Filename, "size", fileHeader.Size)

		if err := h.cfg.Storage.Upload(r.Context(), storageKey, file, fileHeader.Header.Get("Content-Type"), fileHeader.Size); err != nil {
			file.Close()
			log.Error("storage upload failed", "filename", fileHeader.Filename, "error", err)
			continue
		}
		file.Close()

		if h.cfg.Queries != nil {
			contentType := fileHeader.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			var pgUserID pgtype.UUID
			pgUserID.Scan(user.ID)

			dbFile, err := h.cfg.Queries.CreateFile(r.Context(), db.CreateFileParams{
				UserID:      pgUserID,
				Filename:    fileHeader.Filename,
				ContentType: contentType,
				SizeBytes:   fileHeader.Size,
				StorageKey:  storageKey,
				Status:      db.FileStatusPending,
			})
			if err != nil {
				log.Error("database create file failed", "filename", fileHeader.Filename, "error", err)
				continue
			}

			var dbFileID uuid.UUID
			copy(dbFileID[:], dbFile.ID.Bytes[:])

			results = append(results, map[string]any{
				"id":       dbFileID.String(),
				"filename": dbFile.Filename,
				"status":   string(dbFile.Status),
			})

			log.Info("file created", "file_id", dbFileID.String())
		} else {
			results = append(results, map[string]any{
				"id":       fileID.String(),
				"filename": fileHeader.Filename,
				"status":   "pending",
			})
		}
	}

	if len(results) == 0 {
		apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "upload_failed", "Failed to upload files", http.StatusInternalServerError))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"files":   results,
		"message": fmt.Sprintf("Successfully uploaded %d file(s)", len(results)),
	})
}

func (h *Handlers) FileList(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data := pages.FileListPageData{
		Files:       []pages.FileItem{},
		TotalCount:  0,
		CurrentPage: 1,
		TotalPages:  1,
		PageSize:    12,
		Query:       r.URL.Query().Get("q"),
		Filter:      r.URL.Query().Get("filter"),
	}

	pages.FileList(user, data).Render(r.Context(), w)
}

func (h *Handlers) FileDetail(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	fileID := r.PathValue("id")

	data := pages.FileDetailPageData{
		File: pages.FileDetail{
			ID:          fileID,
			Name:        "example.jpg",
			Size:        "1.2 MB",
			ContentType: "image/jpeg",
			Status:      "completed",
			URL:         "#",
			CreatedAt:   "Jan 5, 2026",
			UpdatedAt:   "Jan 5, 2026",
		},
		Variants: []pages.FileVariant{},
		Jobs:     []pages.ProcessingJob{},
	}

	pages.FileDetailPage(user, data).Render(r.Context(), w)
}

func (h *Handlers) DeleteFile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/files", http.StatusFound)
}

func (h *Handlers) Profile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data := pages.ProfilePageData{
		Name:          user.Name,
		Email:         user.Email,
		AvatarURL:     "",
		EmailVerified: false,
		CreatedAt:     "January 2026",
		OAuthAccounts: []pages.OAuthAccountInfo{},
	}

	if user.AvatarURL != nil {
		data.AvatarURL = *user.AvatarURL
	}

	pages.Profile(user, data).Render(r.Context(), w)
}

func (h *Handlers) ProfilePost(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile?error=invalid_form", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/profile?success=1", http.StatusFound)
}

func (h *Handlers) ProfileAvatar(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/profile", http.StatusFound)
}

func (h *Handlers) ProfileDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile?error=invalid_form", http.StatusFound)
		return
	}

	confirmation := r.FormValue("confirmation")
	if confirmation != "DELETE" {
		http.Redirect(w, r, "/profile?error=invalid_confirmation", http.StatusFound)
		return
	}

	h.sessionManager.DeleteSession(r.Context(), w, r)
	http.Redirect(w, r, "/?deleted=1", http.StatusFound)
}

func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	settings, err := h.authService.GetOrCreateUserSettings(r.Context(), user.ID)
	if err != nil {
		apperror.WriteHTTP(w, r, err)
		return
	}

	tokens, err := h.authService.ListAPITokens(r.Context(), user.ID)
	if err != nil {
		apperror.WriteHTTP(w, r, err)
		return
	}

	apiTokens := make([]pages.APIToken, len(tokens))
	for i, t := range tokens {
		lastUsed := "Never"
		if t.LastUsedAt.Valid {
			lastUsed = t.LastUsedAt.Time.Format("Jan 2, 2006")
		}
		apiTokens[i] = pages.APIToken{
			ID:        uuidToString(t.ID),
			Name:      t.Name,
			Prefix:    t.TokenPrefix,
			LastUsed:  lastUsed,
			CreatedAt: t.CreatedAt.Time.Format("Jan 2, 2006"),
		}
	}

	data := pages.SettingsPageData{
		EmailNotifications: settings.EmailNotifications,
		ProcessingAlerts:   settings.ProcessingAlerts,
		MarketingEmails:    settings.MarketingEmails,
		DefaultRetention:   fmt.Sprintf("%d", settings.DefaultRetentionDays),
		AutoDeleteEnabled:  settings.AutoDeleteOriginals,
		APITokens:          apiTokens,
	}

	if r.URL.Query().Get("password_error") != "" {
		data.PasswordError = "Failed to update password. Please check your current password."
	}
	if r.URL.Query().Get("password_success") == "1" {
		data.PasswordSuccess = "Password updated successfully."
	}
	if r.URL.Query().Get("success") == "1" {
		data.Success = "Settings saved successfully."
	}
	if r.URL.Query().Get("token_created") == "1" {
		if newToken := r.URL.Query().Get("new_token"); newToken != "" {
			data.NewToken = newToken
		}
	}
	if r.URL.Query().Get("token_deleted") == "1" {
		data.Success = "API token deleted successfully."
	}
	if r.URL.Query().Get("error") != "" {
		data.Error = "An error occurred. Please try again."
	}
	if tab := r.URL.Query().Get("tab"); tab != "" {
		data.ActiveTab = tab
	}

	pages.Settings(user, data).Render(r.Context(), w)
}

func (h *Handlers) SettingsPassword(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?password_error=invalid_form&tab=security", http.StatusFound)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if newPassword != confirmPassword {
		http.Redirect(w, r, "/settings?password_error=mismatch&tab=security", http.StatusFound)
		return
	}

	if err := h.authService.ChangePassword(r.Context(), user.ID, currentPassword, newPassword); err != nil {
		http.Redirect(w, r, "/settings?password_error="+apperror.Code(err)+"&tab=security", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/settings?password_success=1&tab=security", http.StatusFound)
}

func (h *Handlers) SettingsNotifications(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?error=invalid_form", http.StatusFound)
		return
	}

	emailNotifications := r.FormValue("email_notifications") == "on"
	processingAlerts := r.FormValue("processing_alerts") == "on"
	marketingEmails := r.FormValue("marketing_emails") == "on"

	if err := h.authService.UpdateNotificationSettings(r.Context(), user.ID, emailNotifications, processingAlerts, marketingEmails); err != nil {
		http.Redirect(w, r, "/settings?error=1", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/settings?success=1&tab=notifications", http.StatusFound)
}

func (h *Handlers) SettingsFiles(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?error=invalid_form", http.StatusFound)
		return
	}

	retentionStr := r.FormValue("default_retention")
	retention, err := strconv.ParseInt(retentionStr, 10, 32)
	if err != nil {
		retention = 30
	}

	autoDelete := r.FormValue("auto_delete") == "on"

	if err := h.authService.UpdateFileSettings(r.Context(), user.ID, int32(retention), autoDelete); err != nil {
		http.Redirect(w, r, "/settings?error=1", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/settings?success=1&tab=files", http.StatusFound)
}

func (h *Handlers) SettingsCreateToken(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?error=invalid_form", http.StatusFound)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		name = "Unnamed Token"
	}

	rawToken, _, err := h.authService.CreateAPIToken(r.Context(), user.ID, name)
	if err != nil {
		http.Redirect(w, r, "/settings?error=1", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/settings?token_created=1&new_token="+rawToken+"&tab=api", http.StatusFound)
}

func (h *Handlers) SettingsDeleteToken(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	tokenIDStr := r.PathValue("id")
	tokenID, err := parseUUID(tokenIDStr)
	if err != nil {
		http.Redirect(w, r, "/settings?error=invalid_token", http.StatusFound)
		return
	}

	if err := h.authService.DeleteAPIToken(r.Context(), user.ID, tokenID); err != nil {
		http.Redirect(w, r, "/settings?error=1", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/settings?token_deleted=1&tab=api", http.StatusFound)
}

func (h *Handlers) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		data := pages.VerifyEmailPageData{
			Error: "Invalid verification link. Please request a new one.",
		}
		pages.VerifyEmail(data).Render(r.Context(), w)
		return
	}

	data := pages.VerifyEmailPageData{
		Success: true,
	}
	pages.VerifyEmail(data).Render(r.Context(), w)
}

func (h *Handlers) ResendVerification(w http.ResponseWriter, r *http.Request) {
	pages.ResendVerification(pages.ResendVerificationPageData{}).Render(r.Context(), w)
}

func (h *Handlers) ResendVerificationPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		data := pages.ResendVerificationPageData{Error: "Invalid form data"}
		pages.ResendVerification(data).Render(r.Context(), w)
		return
	}

	data := pages.ResendVerificationPageData{
		Success: "If an account exists with this email, you will receive a verification link.",
	}
	pages.ResendVerification(data).Render(r.Context(), w)
}

func (h *Handlers) Privacy(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	pages.Privacy(user).Render(r.Context(), w)
}

func (h *Handlers) Terms(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	pages.Terms(user).Render(r.Context(), w)
}

func (h *Handlers) Docs(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	section := r.URL.Query().Get("section")
	if section == "" {
		section = "getting-started"
	}
	pages.Docs(user, section).Render(r.Context(), w)
}

func (h *Handlers) NotFound(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	w.WriteHeader(http.StatusNotFound)
	pages.NotFound(user).Render(r.Context(), w)
}

func (h *Handlers) ServerError(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	w.WriteHeader(http.StatusInternalServerError)
	pages.ServerError(user).Render(r.Context(), w)
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return ""
	}
	return u.String()
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
