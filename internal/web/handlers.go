package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file-processor/internal/apperror"
	"github.com/abdul-hamid-achik/file-processor/internal/auth"
	"github.com/abdul-hamid-achik/file-processor/internal/billing"
	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/logger"
	"github.com/abdul-hamid-achik/file-processor/internal/web/templates/pages"
	"github.com/abdul-hamid-achik/file-processor/internal/worker"
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
	_ = pages.Landing(user).Render(r.Context(), w)
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	data := pages.LoginPageData{
		ReturnURL:     r.URL.Query().Get("return"),
		GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
		GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
	}
	_ = pages.Login(data).Render(r.Context(), w)
}

func (h *Handlers) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		data := pages.LoginPageData{
			Error:         apperror.SafeMessage(apperror.Wrap(err, apperror.ErrBadRequest)),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		_ = pages.Login(data).Render(r.Context(), w)
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
		_ = pages.Login(data).Render(r.Context(), w)
		return
	}

	if err := h.sessionManager.CreateSession(r.Context(), w, r, user.ID.Bytes); err != nil {
		data := pages.LoginPageData{
			Error:         apperror.SafeMessage(err),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		_ = pages.Login(data).Render(r.Context(), w)
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
	_ = pages.Register(data).Render(r.Context(), w)
}

func (h *Handlers) RegisterPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		data := pages.RegisterPageData{
			Error:         apperror.SafeMessage(apperror.Wrap(err, apperror.ErrBadRequest)),
			GoogleEnabled: h.oauthService != nil && h.oauthService.IsGoogleConfigured(),
			GitHubEnabled: h.oauthService != nil && h.oauthService.IsGitHubConfigured(),
		}
		_ = pages.Register(data).Render(r.Context(), w)
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
		_ = pages.Register(data).Render(r.Context(), w)
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
	_ = h.sessionManager.DeleteSession(r.Context(), w, r)
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
	_, _ = h.authService.RequestPasswordReset(r.Context(), email)

	data := pages.ForgotPasswordPageData{
		Success: "If an account exists with this email, you will receive a password reset link.",
	}
	pages.ForgotPassword(data).Render(r.Context(), w)
}

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
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

	if h.cfg.Queries != nil {
		pgUserID := pgtype.UUID{
			Bytes: user.ID,
			Valid: true,
		}

		total, err := h.cfg.Queries.CountFilesByUser(r.Context(), pgUserID)
		if err != nil {
			log.Error("failed to count files", "error", err)
		} else {
			data.Stats.TotalFiles = total
		}

		files, err := h.cfg.Queries.ListFilesByUser(r.Context(), db.ListFilesByUserParams{
			UserID: pgUserID,
			Limit:  5,
			Offset: 0,
		})
		if err != nil {
			log.Error("failed to list recent files", "error", err)
		} else {
			data.RecentFiles = make([]pages.RecentFile, len(files))
			totalStorage := int64(0)
			processedCount := int64(0)

			for i, f := range files {
				totalStorage += f.SizeBytes
				if f.Status == db.FileStatusCompleted {
					processedCount++
				}

				data.RecentFiles[i] = pages.RecentFile{
					ID:        uuidToString(f.ID),
					Name:      f.Filename,
					Size:      formatBytes(f.SizeBytes),
					Status:    string(f.Status),
					CreatedAt: f.CreatedAt.Time.Format("Jan 2, 2006"),
				}
			}

			data.Stats.StorageUsed = formatBytes(totalStorage)
			data.Stats.ProcessedFiles = processedCount
		}
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

	limits := billing.GetTierLimits(user.SubscriptionTier)
	maxSize := limits.MaxFileSize
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	if h.cfg.Queries != nil {
		pgUserID := pgtype.UUID{Bytes: user.ID, Valid: true}
		filesCount, err := h.cfg.Queries.GetUserFilesCount(r.Context(), pgUserID)
		if err != nil {
			log.Error("failed to get files count", "error", err)
		} else if filesCount >= int64(limits.FilesLimit) {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "file_limit_reached",
				fmt.Sprintf("You have reached your file limit of %d files. Please upgrade to Pro for more storage.", limits.FilesLimit),
				http.StatusForbidden))
			return
		}
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrFileTooLarge))
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "missing_file", "Please select at least one file to upload", http.StatusBadRequest))
		return
	}

	for _, fh := range files {
		if fh.Size > maxSize {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "file_too_large",
				fmt.Sprintf("File %s exceeds maximum size of %s", fh.Filename, formatBytes(maxSize)),
				http.StatusRequestEntityTooLarge))
			return
		}
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
			_ = file.Close()
			log.Error("storage upload failed", "filename", fileHeader.Filename, "error", err)
			continue
		}
		_ = file.Close()

		if h.cfg.Queries != nil {
			contentType := fileHeader.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			pgUserID := pgtype.UUID{
				Bytes: user.ID,
				Valid: true,
			}

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

			if h.cfg.Broker != nil {
				payload := worker.NewThumbnailPayload(dbFileID)
				jobID, err := h.cfg.Broker.Enqueue("thumbnail", payload)
				if err != nil {
					log.Error("failed to enqueue thumbnail job", "error", err)
				} else {
					log.Info("thumbnail job enqueued", "job_id", jobID, "file_id", dbFileID.String())
				}
			}
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
	_ = json.NewEncoder(w).Encode(map[string]any{
		"files":   results,
		"message": fmt.Sprintf("Successfully uploaded %d file(s)", len(results)),
	})
}

func (h *Handlers) FileList(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	pageSize := int32(12)
	currentPage := int32(1)

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.ParseInt(pageStr, 10, 32); err == nil && p > 0 {
			currentPage = int32(p)
		}
	}

	offset := (currentPage - 1) * pageSize

	data := pages.FileListPageData{
		Files:       []pages.FileItem{},
		TotalCount:  0,
		CurrentPage: int(currentPage),
		TotalPages:  1,
		PageSize:    int(pageSize),
		Query:       r.URL.Query().Get("q"),
		Filter:      r.URL.Query().Get("filter"),
	}

	if h.cfg.Queries != nil {
		pgUserID := pgtype.UUID{
			Bytes: user.ID,
			Valid: true,
		}

		files, err := h.cfg.Queries.ListFilesByUser(r.Context(), db.ListFilesByUserParams{
			UserID: pgUserID,
			Limit:  pageSize,
			Offset: offset,
		})
		if err != nil {
			log.Error("failed to list files", "error", err)
		} else {
			fileIDs := make([]pgtype.UUID, len(files))
			for i, f := range files {
				fileIDs[i] = f.ID
			}

			thumbnailMap := make(map[string]string)
			if len(fileIDs) > 0 {
				thumbnails, err := h.cfg.Queries.GetThumbnailsForFiles(r.Context(), fileIDs)
				if err != nil {
					log.Error("failed to fetch thumbnails", "error", err)
				} else {
					for _, t := range thumbnails {
						thumbnailMap[uuidToString(t.FileID)] = "/files/" + uuidToString(t.FileID) + "/download?variant=thumbnail"
					}
				}
			}

			data.Files = make([]pages.FileItem, len(files))
			for i, f := range files {
				fileIDStr := uuidToString(f.ID)
				data.Files[i] = pages.FileItem{
					ID:           fileIDStr,
					Name:         f.Filename,
					Size:         formatBytes(f.SizeBytes),
					ContentType:  f.ContentType,
					Status:       string(f.Status),
					ThumbnailURL: thumbnailMap[fileIDStr],
					CreatedAt:    f.CreatedAt.Time.Format("Jan 2, 2006"),
				}
			}
		}

		total, err := h.cfg.Queries.CountFilesByUser(r.Context(), pgUserID)
		if err != nil {
			log.Error("failed to count files", "error", err)
		} else {
			data.TotalCount = total
			if total > 0 {
				data.TotalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
			}
		}
	}

	pages.FileList(user, data).Render(r.Context(), w)
}

func (h *Handlers) FileDetail(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	fileIDStr := r.PathValue("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		log.Error("invalid file ID", "file_id", fileIDStr, "error", err)
		http.Redirect(w, r, "/files?error=invalid_id", http.StatusFound)
		return
	}

	isPremium := user.SubscriptionTier == db.SubscriptionTierPro || user.SubscriptionTier == db.SubscriptionTierEnterprise
	data := pages.FileDetailPageData{
		File: pages.FileDetail{
			ID:          fileIDStr,
			Name:        "File not found",
			Size:        "0 B",
			ContentType: "unknown",
			Status:      "unknown",
			URL:         "#",
			CreatedAt:   "",
			UpdatedAt:   "",
		},
		Variants:         []pages.FileVariant{},
		Jobs:             []pages.ProcessingJob{},
		ExistingTypes:    make(map[string]bool),
		IsPremium:        isPremium,
		SubscriptionTier: string(user.SubscriptionTier),
	}

	if h.cfg.Queries != nil {
		pgFileID := pgtype.UUID{
			Bytes: fileID,
			Valid: true,
		}

		file, err := h.cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			log.Error("file not found", "file_id", fileIDStr, "error", err)
			http.Redirect(w, r, "/files?error=not_found", http.StatusFound)
			return
		}

		pgUserID := pgtype.UUID{
			Bytes: user.ID,
			Valid: true,
		}

		if file.UserID.Bytes != pgUserID.Bytes {
			log.Warn("unauthorized file access", "file_id", fileIDStr, "user_id", user.ID.String())
			http.Redirect(w, r, "/files?error=not_found", http.StatusFound)
			return
		}

		thumbnailURL := ""
		variants, err := h.cfg.Queries.ListVariantsByFile(r.Context(), pgFileID)
		if err != nil {
			log.Error("failed to list variants", "file_id", fileIDStr, "error", err)
		} else if len(variants) > 0 {
			data.Variants = make([]pages.FileVariant, len(variants))
			for i, v := range variants {
				variantURL := "/files/" + fileIDStr + "/download?variant=" + string(v.VariantType)
				data.Variants[i] = pages.FileVariant{
					ID:          uuidToString(v.ID),
					Type:        string(v.VariantType),
					Size:        formatBytes(v.SizeBytes),
					URL:         variantURL,
					ContentType: v.ContentType,
					CreatedAt:   v.CreatedAt.Time.Format("Jan 2, 2006 3:04 PM"),
				}
				if v.VariantType == db.VariantTypeThumbnail {
					thumbnailURL = variantURL
				}
				data.ExistingTypes[string(v.VariantType)] = true
			}
		}

		data.File = pages.FileDetail{
			ID:           fileIDStr,
			Name:         file.Filename,
			Size:         formatBytes(file.SizeBytes),
			ContentType:  file.ContentType,
			Status:       string(file.Status),
			URL:          "/files/" + fileIDStr + "/download",
			ThumbnailURL: thumbnailURL,
			CreatedAt:    file.CreatedAt.Time.Format("Jan 2, 2006 3:04 PM"),
			UpdatedAt:    file.UpdatedAt.Time.Format("Jan 2, 2006 3:04 PM"),
		}
	}

	pages.FileDetailPage(user, data).Render(r.Context(), w)
}

func (h *Handlers) DeleteFile(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	fileIDStr := r.PathValue("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		log.Error("invalid file ID", "file_id", fileIDStr, "error", err)
		http.Redirect(w, r, "/files?error=invalid_id", http.StatusFound)
		return
	}

	if h.cfg.Queries == nil {
		http.Redirect(w, r, "/files?error=server_error", http.StatusFound)
		return
	}

	pgFileID := pgtype.UUID{
		Bytes: fileID,
		Valid: true,
	}

	file, err := h.cfg.Queries.GetFile(r.Context(), pgFileID)
	if err != nil {
		log.Error("file not found", "file_id", fileIDStr, "error", err)
		http.Redirect(w, r, "/files?error=not_found", http.StatusFound)
		return
	}

	pgUserID := pgtype.UUID{
		Bytes: user.ID,
		Valid: true,
	}

	if file.UserID.Bytes != pgUserID.Bytes {
		log.Warn("unauthorized delete attempt", "file_id", fileIDStr, "user_id", user.ID.String())
		http.Redirect(w, r, "/files?error=not_found", http.StatusFound)
		return
	}

	if err := h.cfg.Queries.SoftDeleteFile(r.Context(), pgFileID); err != nil {
		log.Error("failed to delete file", "file_id", fileIDStr, "error", err)
		http.Redirect(w, r, "/files?error=delete_failed", http.StatusFound)
		return
	}

	log.Info("file deleted", "file_id", fileIDStr, "user_id", user.ID.String())
	http.Redirect(w, r, "/files?success=deleted", http.StatusFound)
}

func (h *Handlers) ProcessFile(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fileIDStr := r.PathValue("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		log.Error("invalid file ID", "file_id", fileIDStr, "error", err)
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")
	if action == "" {
		http.Error(w, "Action is required", http.StatusBadRequest)
		return
	}

	if !billing.CanUseFeature(user.SubscriptionTier, action) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, `<div class="p-4 bg-nord-11/20 text-nord-11 rounded-lg text-sm">This feature requires a Pro subscription. <a href="/billing" class="underline">Upgrade now</a></div>`)
		return
	}

	if h.cfg.Queries == nil || h.cfg.Broker == nil {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
	pgUserID := pgtype.UUID{Bytes: user.ID, Valid: true}

	file, err := h.cfg.Queries.GetFile(r.Context(), pgFileID)
	if err != nil {
		log.Error("file not found", "file_id", fileIDStr, "error", err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if file.UserID.Bytes != pgUserID.Bytes {
		log.Warn("unauthorized process attempt", "file_id", fileIDStr, "user_id", user.ID.String())
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	var variantType db.VariantType
	var jobType string
	var payload interface{}

	switch action {
	case "thumbnail":
		variantType = db.VariantTypeThumbnail
		jobType = "thumbnail"
		payload = worker.NewThumbnailPayload(fileID)
	case "sm":
		variantType = "sm"
		jobType = "resize"
		payload = worker.NewResponsivePayload(fileID, "sm")
	case "md":
		variantType = "md"
		jobType = "resize"
		payload = worker.NewResponsivePayload(fileID, "md")
	case "lg":
		variantType = "lg"
		jobType = "resize"
		payload = worker.NewResponsivePayload(fileID, "lg")
	case "xl":
		variantType = "xl"
		jobType = "resize"
		payload = worker.NewResponsivePayload(fileID, "xl")
	case "og":
		variantType = "og"
		jobType = "resize"
		payload = worker.NewSocialPayload(fileID, "og")
	case "twitter":
		variantType = "twitter"
		jobType = "resize"
		payload = worker.NewSocialPayload(fileID, "twitter")
	case "instagram_square":
		variantType = "instagram_square"
		jobType = "resize"
		payload = worker.NewSocialPayload(fileID, "instagram_square")
	case "instagram_portrait":
		variantType = "instagram_portrait"
		jobType = "resize"
		payload = worker.NewSocialPayload(fileID, "instagram_portrait")
	case "instagram_story":
		variantType = "instagram_story"
		jobType = "resize"
		payload = worker.NewSocialPayload(fileID, "instagram_story")
	case "webp":
		variantType = db.VariantTypeWebp
		jobType = "webp"
		payload = worker.NewWebPPayload(fileID, 85)
	case "watermark":
		variantType = db.VariantTypeWatermarked
		jobType = "watermark"
		text := r.FormValue("watermark_text")
		position := r.FormValue("watermark_position")
		if position == "" {
			position = "bottom-right"
		}
		isPremium := user.SubscriptionTier == db.SubscriptionTierPro || user.SubscriptionTier == db.SubscriptionTierEnterprise
		payload = worker.NewWatermarkPayload(fileID, text, position, 0.5, isPremium)
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	hasVariant, err := h.cfg.Queries.HasVariant(r.Context(), db.HasVariantParams{
		FileID:      pgFileID,
		VariantType: variantType,
	})
	if err != nil {
		log.Error("failed to check variant", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if hasVariant {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `<div class="p-4 bg-nord-13/20 text-nord-13 rounded-lg text-sm">This variant already exists.</div>`)
		return
	}

	jobID, err := h.cfg.Broker.Enqueue(jobType, payload)
	if err != nil {
		log.Error("failed to enqueue job", "job_type", jobType, "error", err)
		http.Error(w, "Failed to start processing", http.StatusInternalServerError)
		return
	}

	log.Info("processing job enqueued", "file_id", fileIDStr, "job_type", jobType, "job_id", jobID)

	if err := h.cfg.Queries.UpdateFileStatus(r.Context(), db.UpdateFileStatusParams{
		ID:     pgFileID,
		Status: db.FileStatusProcessing,
	}); err != nil {
		log.Warn("failed to update file status", "error", err)
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusAccepted)
	_, _ = fmt.Fprintf(w, `<div class="p-4 bg-nord-14/20 text-nord-14 rounded-lg text-sm">Processing started! Refresh the page to see the result.</div>`)
}

func (h *Handlers) DownloadFile(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fileIDStr := r.PathValue("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		log.Error("invalid file ID", "file_id", fileIDStr, "error", err)
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}

	if h.cfg.Queries == nil || h.cfg.Storage == nil {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	pgFileID := pgtype.UUID{
		Bytes: fileID,
		Valid: true,
	}

	file, err := h.cfg.Queries.GetFile(r.Context(), pgFileID)
	if err != nil {
		log.Error("file not found", "file_id", fileIDStr, "error", err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	pgUserID := pgtype.UUID{
		Bytes: user.ID,
		Valid: true,
	}

	if file.UserID.Bytes != pgUserID.Bytes {
		log.Warn("unauthorized download attempt", "file_id", fileIDStr, "user_id", user.ID.String())
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	storageKey := file.StorageKey
	contentType := file.ContentType
	filename := file.Filename

	variantType := r.URL.Query().Get("variant")
	if variantType != "" {
		variant, err := h.cfg.Queries.GetVariant(r.Context(), db.GetVariantParams{
			FileID:      pgFileID,
			VariantType: db.VariantType(variantType),
		})
		if err != nil {
			log.Error("variant not found", "file_id", fileIDStr, "variant", variantType, "error", err)
			http.Error(w, "Variant not found", http.StatusNotFound)
			return
		}
		storageKey = variant.StorageKey
		contentType = variant.ContentType
		filename = file.Filename
	}

	reader, err := h.cfg.Storage.Download(r.Context(), storageKey)
	if err != nil {
		log.Error("failed to download from storage", "storage_key", storageKey, "error", err)
		http.Error(w, "Failed to download file", http.StatusInternalServerError)
		return
	}
	defer func() { _ = reader.Close() }()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	if _, err := io.Copy(w, reader); err != nil {
		log.Error("failed to stream file", "error", err)
		return
	}

	log.Info("file downloaded", "file_id", fileIDStr, "variant", variantType)
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
	log := logger.FromContext(r.Context())
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

	if h.cfg.Queries == nil {
		http.Redirect(w, r, "/profile?error=server_error", http.StatusFound)
		return
	}

	pgUserID := pgtype.UUID{
		Bytes: user.ID,
		Valid: true,
	}

	if err := h.cfg.Queries.DeleteUser(r.Context(), pgUserID); err != nil {
		log.Error("failed to delete user", "user_id", user.ID.String(), "error", err)
		http.Redirect(w, r, "/profile?error=delete_failed", http.StatusFound)
		return
	}

	log.Info("user account deleted", "user_id", user.ID.String(), "email", user.Email)
	_ = h.sessionManager.DeleteSession(r.Context(), w, r)
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

func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	sizes := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), sizes[exp])
}
