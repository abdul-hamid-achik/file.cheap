package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file-processor/internal/apperror"
	"github.com/abdul-hamid-achik/file-processor/internal/billing"
	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/logger"
	"github.com/abdul-hamid-achik/file-processor/internal/processor"
	"github.com/abdul-hamid-achik/file-processor/internal/storage"
	"github.com/abdul-hamid-achik/file-processor/internal/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Querier interface {
	GetFile(ctx context.Context, id pgtype.UUID) (db.File, error)
	ListFilesByUser(ctx context.Context, arg db.ListFilesByUserParams) ([]db.File, error)
	CountFilesByUser(ctx context.Context, userID pgtype.UUID) (int64, error)
	CreateFile(ctx context.Context, arg db.CreateFileParams) (db.File, error)
	SoftDeleteFile(ctx context.Context, id pgtype.UUID) error
	ListVariantsByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileVariant, error)
	GetAPITokenByHash(ctx context.Context, tokenHash string) (db.GetAPITokenByHashRow, error)
	UpdateAPITokenLastUsed(ctx context.Context, id pgtype.UUID) error
	GetFileShareByToken(ctx context.Context, token string) (db.GetFileShareByTokenRow, error)
	IncrementShareAccessCount(ctx context.Context, id pgtype.UUID) error
	GetTransformCache(ctx context.Context, arg db.GetTransformCacheParams) (db.TransformCache, error)
	CreateTransformCache(ctx context.Context, arg db.CreateTransformCacheParams) (db.TransformCache, error)
	IncrementTransformCacheCount(ctx context.Context, arg db.IncrementTransformCacheCountParams) error
	GetTransformRequestCount(ctx context.Context, arg db.GetTransformRequestCountParams) (int32, error)
	CreateFileShare(ctx context.Context, arg db.CreateFileShareParams) (db.FileShare, error)
	ListFileSharesByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileShare, error)
	DeleteFileShare(ctx context.Context, arg db.DeleteFileShareParams) error
	GetUserBillingInfo(ctx context.Context, id pgtype.UUID) (db.GetUserBillingInfoRow, error)
	GetUserFilesCount(ctx context.Context, userID pgtype.UUID) (int64, error)
	GetUserTransformationUsage(ctx context.Context, id pgtype.UUID) (db.GetUserTransformationUsageRow, error)
	IncrementTransformationCount(ctx context.Context, id pgtype.UUID) error
	CreateBatchOperation(ctx context.Context, arg db.CreateBatchOperationParams) (db.BatchOperation, error)
	GetBatchOperationByUser(ctx context.Context, arg db.GetBatchOperationByUserParams) (db.BatchOperation, error)
	CreateBatchItem(ctx context.Context, arg db.CreateBatchItemParams) (db.BatchItem, error)
	ListBatchItems(ctx context.Context, batchID pgtype.UUID) ([]db.BatchItem, error)
	CountBatchItemsByStatus(ctx context.Context, batchID pgtype.UUID) (db.CountBatchItemsByStatusRow, error)
}

type Broker interface {
	Enqueue(jobType string, payload interface{}) (string, error)
}

type Config struct {
	Storage       storage.Storage
	Broker        Broker
	Queries       Querier
	MaxUploadSize int64
	JWTSecret     string
	RateLimit     int
	RateBurst     int
	BaseURL       string
	Registry      *processor.Registry
}

func NewRouter(cfg *Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	apiMux := http.NewServeMux()

	apiMux.HandleFunc("POST /api/v1/upload", uploadHandler(cfg))
	apiMux.HandleFunc("GET /api/v1/files", listFilesHandler(cfg))
	apiMux.HandleFunc("GET /api/v1/files/{id}", getFileHandler(cfg))
	apiMux.HandleFunc("GET /api/v1/files/{id}/download", downloadHandler(cfg))
	apiMux.HandleFunc("DELETE /api/v1/files/{id}", deleteHandler(cfg))

	cdnCfg := &CDNConfig{
		Storage:  cfg.Storage,
		Queries:  cfg.Queries,
		Registry: cfg.Registry,
	}
	apiMux.HandleFunc("POST /api/v1/files/{id}/share", CreateShareHandler(cdnCfg, cfg.BaseURL))
	apiMux.HandleFunc("GET /api/v1/files/{id}/shares", ListSharesHandler(cdnCfg))
	apiMux.HandleFunc("DELETE /api/v1/shares/{shareId}", DeleteShareHandler(cdnCfg))

	apiMux.HandleFunc("POST /api/v1/files/{id}/transform", transformHandler(cfg))

	apiMux.HandleFunc("POST /api/v1/batch/transform", batchTransformHandler(cfg))
	apiMux.HandleFunc("GET /api/v1/batch/{id}", getBatchHandler(cfg))

	rateLimit := cfg.RateLimit
	if rateLimit <= 0 {
		rateLimit = 100
	}
	rateBurst := cfg.RateBurst
	if rateBurst <= 0 {
		rateBurst = 200
	}
	limiter := NewRateLimiter(rateLimit, rateBurst)

	handler := RateLimit(limiter)(CORS(DualAuthMiddleware(cfg.JWTSecret, cfg.Queries)(BillingMiddleware(cfg.Queries)(apiMux))))
	mux.Handle("/api/", handler)

	mux.HandleFunc("GET /cdn/{token}/{transforms}/{filename}", CDNHandler(cdnCfg))

	return mux
}

func uploadHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		log = log.With("user_id", userID.String())

		billingInfo := GetBilling(r.Context())

		maxSize := cfg.MaxUploadSize
		if maxSize == 0 {
			maxSize = 100 * 1024 * 1024
		}

		if billingInfo != nil {
			if billingInfo.FilesCount >= int64(billingInfo.FilesLimit) {
				limits := billing.GetTierLimits(billingInfo.Tier)
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "file_limit_reached",
					fmt.Sprintf("File limit of %d reached. Upgrade to Pro for %d files.", limits.FilesLimit, billing.ProFilesLimit),
					http.StatusForbidden))
				return
			}

			if billingInfo.MaxFileSize > 0 && billingInfo.MaxFileSize < maxSize {
				maxSize = billingInfo.MaxFileSize
			}
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxSize)

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrFileTooLarge))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "missing_file", "Please select a file to upload", http.StatusBadRequest))
			return
		}
		defer file.Close()

		fileID := uuid.New()
		storageKey := fmt.Sprintf("uploads/%s/%s/%s", userID.String(), fileID.String(), header.Filename)

		log.Info("uploading file", "filename", header.Filename, "size", header.Size, "content_type", header.Header.Get("Content-Type"))

		if err := cfg.Storage.Upload(r.Context(), storageKey, file, header.Header.Get("Content-Type"), header.Size); err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		if cfg.Queries != nil {
			var pgUserID pgtype.UUID
			_ = pgUserID.Scan(userID)

			contentType := header.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			dbFile, err := cfg.Queries.CreateFile(r.Context(), db.CreateFileParams{
				UserID:      pgUserID,
				Filename:    header.Filename,
				ContentType: contentType,
				SizeBytes:   header.Size,
				StorageKey:  storageKey,
				Status:      db.FileStatusPending,
			})
			if err != nil {
				apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
				return
			}

			fileIDStr := uuidFromPgtype(dbFile.ID)
			log.Info("file created", "file_id", fileIDStr)

			if cfg.Broker != nil {
				var fileUUID uuid.UUID
				copy(fileUUID[:], dbFile.ID.Bytes[:])
				payload := worker.NewThumbnailPayload(fileUUID)
				jobID, err := cfg.Broker.Enqueue("thumbnail", payload)
				if err != nil {
					log.Error("failed to enqueue thumbnail job", "error", err)
				} else {
					log.Info("thumbnail job enqueued", "job_id", jobID)
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       fileIDStr,
				"filename": dbFile.Filename,
				"status":   string(dbFile.Status),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       fileID.String(),
			"filename": header.Filename,
			"status":   "pending",
		})
	}
}

func listFilesHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit := int32(20)
		offset := int32(0)

		if limitStr != "" {
			l, err := strconv.Atoi(limitStr)
			if err != nil || l < 0 || l > 100 {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = int32(l)
		}

		if offsetStr != "" {
			o, err := strconv.Atoi(offsetStr)
			if err != nil || o < 0 {
				http.Error(w, "invalid offset", http.StatusBadRequest)
				return
			}
			offset = int32(o)
		}

		if cfg.Queries == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files":    []any{},
				"total":    0,
				"has_more": false,
			})
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		files, err := cfg.Queries.ListFilesByUser(r.Context(), db.ListFilesByUserParams{
			UserID: pgUserID,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		total, _ := cfg.Queries.CountFilesByUser(r.Context(), pgUserID)

		filesList := make([]map[string]any, len(files))
		for i, f := range files {
			filesList[i] = fileToJSON(f)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"files":    filesList,
			"total":    total,
			"has_more": int64(offset)+int64(len(files)) < total,
		})
	}
}

func getFileHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			http.Error(w, "invalid file ID", http.StatusBadRequest)
			return
		}

		if cfg.Queries == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}

		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		fileUserID := uuidFromPgtype(file.UserID)
		if fileUserID != userID.String() {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if file.DeletedAt.Valid {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		response := fileToJSON(file)

		variants, err := cfg.Queries.ListVariantsByFile(r.Context(), pgFileID)
		if err == nil && len(variants) > 0 {
			variantsList := make([]map[string]any, len(variants))
			for i, v := range variants {
				variantsList[i] = map[string]any{
					"id":           uuidFromPgtype(v.ID),
					"variant_type": string(v.VariantType),
					"content_type": v.ContentType,
					"size_bytes":   v.SizeBytes,
					"width":        v.Width,
					"height":       v.Height,
				}
			}
			response["variants"] = variantsList
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func downloadHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			http.Error(w, "invalid file ID", http.StatusBadRequest)
			return
		}

		if cfg.Queries == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}

		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		fileUserID := uuidFromPgtype(file.UserID)
		if fileUserID != userID.String() {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		storageKey := file.StorageKey
		variantType := r.URL.Query().Get("variant")
		if variantType != "" {
			variants, err := cfg.Queries.ListVariantsByFile(r.Context(), pgFileID)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			found := false
			for _, v := range variants {
				if string(v.VariantType) == variantType {
					storageKey = v.StorageKey
					found = true
					break
				}
			}
			if !found {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}

		url, err := cfg.Storage.GetPresignedURL(r.Context(), storageKey, 3600)
		if err != nil {
			http.Error(w, "download failed", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

func deleteHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			http.Error(w, "invalid file ID", http.StatusBadRequest)
			return
		}

		log = log.With("user_id", userID.String(), "file_id", fileIDStr)

		if cfg.Queries == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}

		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			log.Debug("file not found for delete", "error", err)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		fileUserID := uuidFromPgtype(file.UserID)
		if fileUserID != userID.String() {
			log.Warn("unauthorized delete attempt", "owner_id", fileUserID)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if file.DeletedAt.Valid {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if err := cfg.Queries.SoftDeleteFile(r.Context(), pgFileID); err != nil {
			log.Error("soft delete failed", "error", err)
			http.Error(w, "delete failed", http.StatusInternalServerError)
			return
		}

		log.Info("file deleted")
		w.WriteHeader(http.StatusNoContent)
	}
}

func fileToJSON(f db.File) map[string]any {
	return map[string]any{
		"id":           uuidFromPgtype(f.ID),
		"filename":     f.Filename,
		"content_type": f.ContentType,
		"size_bytes":   f.SizeBytes,
		"status":       string(f.Status),
		"created_at":   f.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func uuidFromPgtype(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u := uuid.UUID(id.Bytes)
	return u.String()
}

type TransformRequest struct {
	Presets   []string `json:"presets"`
	WebP      bool     `json:"webp"`
	Quality   int      `json:"quality"`
	Watermark string   `json:"watermark"`
}

type TransformResponse struct {
	FileID string   `json:"file_id"`
	Jobs   []string `json:"jobs"`
}

func transformHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_file_id", "Invalid file ID format", http.StatusBadRequest))
			return
		}

		log = log.With("user_id", userID.String(), "file_id", fileIDStr)

		if cfg.Queries == nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		fileUserID := uuidFromPgtype(file.UserID)
		if fileUserID != userID.String() {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		if file.DeletedAt.Valid {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		var req TransformRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid JSON request body", http.StatusBadRequest))
			return
		}

		if len(req.Presets) == 0 && !req.WebP && req.Watermark == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_transformations", "At least one transformation is required", http.StatusBadRequest))
			return
		}

		jobCount := len(req.Presets)
		if req.WebP {
			jobCount++
		}
		if req.Watermark != "" {
			jobCount++
		}

		billingInfo := GetBilling(r.Context())
		if billingInfo != nil {
			usage, err := cfg.Queries.GetUserTransformationUsage(r.Context(), pgUserID)
			if err == nil {
				remaining := int(usage.TransformationsLimit) - int(usage.TransformationsCount)
				if usage.TransformationsLimit != -1 && remaining < jobCount {
					apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "transformation_limit_reached",
						fmt.Sprintf("Not enough transformations remaining. Need %d, have %d.", jobCount, remaining),
						http.StatusForbidden))
					return
				}
			}

			for _, preset := range req.Presets {
				if !billing.CanUseFeature(billingInfo.Tier, preset) {
					apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "feature_not_available",
						fmt.Sprintf("Preset '%s' is not available on your plan. Upgrade to Pro for access.", preset),
						http.StatusForbidden))
					return
				}
			}

			if req.Watermark != "" && !billing.CanUseFeature(billingInfo.Tier, "watermark") {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "feature_not_available",
					"Custom watermarks are not available on your plan. Upgrade to Pro for access.",
					http.StatusForbidden))
				return
			}
		}

		if cfg.Broker == nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "service_unavailable", "Job queue is not available", http.StatusServiceUnavailable))
			return
		}

		var jobIDs []string
		quality := req.Quality
		if quality <= 0 {
			quality = 85
		}

		for _, preset := range req.Presets {
			var payload interface{}
			var jobType string

			switch preset {
			case "thumbnail":
				payload = worker.NewThumbnailPayload(fileID)
				jobType = "thumbnail"
			case "sm", "md", "lg", "xl":
				payload = worker.NewResponsivePayload(fileID, preset)
				jobType = "resize"
			case "og", "twitter", "instagram_square", "instagram_portrait", "instagram_story":
				payload = worker.NewSocialPayload(fileID, preset)
				jobType = "resize"
			default:
				log.Warn("unknown preset requested", "preset", preset)
				continue
			}

			jobID, err := cfg.Broker.Enqueue(jobType, payload)
			if err != nil {
				log.Error("failed to enqueue job", "preset", preset, "error", err)
				continue
			}
			jobIDs = append(jobIDs, jobID)

			if err := cfg.Queries.IncrementTransformationCount(r.Context(), pgUserID); err != nil {
				log.Error("failed to increment transformation count", "error", err)
			}
		}

		if req.WebP {
			payload := worker.NewWebPPayload(fileID, quality)
			jobID, err := cfg.Broker.Enqueue("webp", payload)
			if err != nil {
				log.Error("failed to enqueue webp job", "error", err)
			} else {
				jobIDs = append(jobIDs, jobID)
				if err := cfg.Queries.IncrementTransformationCount(r.Context(), pgUserID); err != nil {
					log.Error("failed to increment transformation count", "error", err)
				}
			}
		}

		if req.Watermark != "" {
			isPremium := billingInfo != nil && (billingInfo.Tier == db.SubscriptionTierPro || billingInfo.Tier == db.SubscriptionTierEnterprise)
			payload := worker.NewWatermarkPayload(fileID, req.Watermark, "bottom-right", 0.5, isPremium)
			jobID, err := cfg.Broker.Enqueue("watermark", payload)
			if err != nil {
				log.Error("failed to enqueue watermark job", "error", err)
			} else {
				jobIDs = append(jobIDs, jobID)
				if err := cfg.Queries.IncrementTransformationCount(r.Context(), pgUserID); err != nil {
					log.Error("failed to increment transformation count", "error", err)
				}
			}
		}

		if len(jobIDs) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_jobs_created", "Failed to create any transformation jobs", http.StatusInternalServerError))
			return
		}

		log.Info("transform jobs created", "job_count", len(jobIDs))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(TransformResponse{
			FileID: fileIDStr,
			Jobs:   jobIDs,
		})
	}
}

const maxBatchFiles = 100

type BatchTransformRequest struct {
	FileIDs   []string `json:"file_ids"`
	Presets   []string `json:"presets"`
	WebP      bool     `json:"webp"`
	Quality   int      `json:"quality"`
	Watermark string   `json:"watermark"`
}

type BatchTransformResponse struct {
	BatchID    string `json:"batch_id"`
	TotalFiles int    `json:"total_files"`
	TotalJobs  int    `json:"total_jobs"`
	Status     string `json:"status"`
	StatusURL  string `json:"status_url"`
}

func batchTransformHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		log = log.With("user_id", userID.String())

		if cfg.Queries == nil || cfg.Broker == nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		var req BatchTransformRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid JSON request body", http.StatusBadRequest))
			return
		}

		if len(req.FileIDs) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_files", "At least one file ID is required", http.StatusBadRequest))
			return
		}

		if len(req.FileIDs) > maxBatchFiles {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "too_many_files",
				fmt.Sprintf("Maximum %d files per batch. You requested %d.", maxBatchFiles, len(req.FileIDs)),
				http.StatusBadRequest))
			return
		}

		if len(req.Presets) == 0 && !req.WebP && req.Watermark == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_transformations", "At least one transformation is required", http.StatusBadRequest))
			return
		}

		jobsPerFile := len(req.Presets)
		if req.WebP {
			jobsPerFile++
		}
		if req.Watermark != "" {
			jobsPerFile++
		}
		totalJobs := jobsPerFile * len(req.FileIDs)

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		billingInfo := GetBilling(r.Context())

		if billingInfo != nil {
			usage, err := cfg.Queries.GetUserTransformationUsage(r.Context(), pgUserID)
			if err == nil {
				remaining := int(usage.TransformationsLimit) - int(usage.TransformationsCount)
				if usage.TransformationsLimit != -1 && remaining < totalJobs {
					apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "transformation_limit_reached",
						fmt.Sprintf("Not enough transformations remaining. Need %d, have %d.", totalJobs, remaining),
						http.StatusForbidden))
					return
				}
			}

			for _, preset := range req.Presets {
				if !billing.CanUseFeature(billingInfo.Tier, preset) {
					apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "feature_not_available",
						fmt.Sprintf("Preset '%s' is not available on your plan. Upgrade to Pro for access.", preset),
						http.StatusForbidden))
					return
				}
			}

			if req.Watermark != "" && !billing.CanUseFeature(billingInfo.Tier, "watermark") {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "feature_not_available",
					"Custom watermarks are not available on your plan. Upgrade to Pro for access.",
					http.StatusForbidden))
				return
			}
		}

		var validFileIDs []uuid.UUID
		for _, fileIDStr := range req.FileIDs {
			fileID, err := uuid.Parse(fileIDStr)
			if err != nil {
				continue
			}

			pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
			file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
			if err != nil {
				continue
			}

			if uuidFromPgtype(file.UserID) != userID.String() || file.DeletedAt.Valid {
				continue
			}

			validFileIDs = append(validFileIDs, fileID)
		}

		if len(validFileIDs) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_valid_files", "No valid file IDs found", http.StatusBadRequest))
			return
		}

		quality := req.Quality
		if quality <= 0 {
			quality = 85
		}

		var watermark *string
		if req.Watermark != "" {
			watermark = &req.Watermark
		}

		batch, err := cfg.Queries.CreateBatchOperation(r.Context(), db.CreateBatchOperationParams{
			UserID:     pgUserID,
			TotalFiles: int32(len(validFileIDs)),
			Presets:    req.Presets,
			Webp:       req.WebP,
			Quality:    int32(quality),
			Watermark:  watermark,
		})
		if err != nil {
			log.Error("failed to create batch operation", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		batchIDStr := uuidFromPgtype(batch.ID)
		log = log.With("batch_id", batchIDStr)

		isPremium := billingInfo != nil && (billingInfo.Tier == db.SubscriptionTierPro || billingInfo.Tier == db.SubscriptionTierEnterprise)

		var totalJobsCreated int
		for _, fileID := range validFileIDs {
			var jobIDs []string

			for _, preset := range req.Presets {
				var payload interface{}
				var jobType string

				switch preset {
				case "thumbnail":
					payload = worker.NewThumbnailPayload(fileID)
					jobType = "thumbnail"
				case "sm", "md", "lg", "xl":
					payload = worker.NewResponsivePayload(fileID, preset)
					jobType = "resize"
				case "og", "twitter", "instagram_square", "instagram_portrait", "instagram_story":
					payload = worker.NewSocialPayload(fileID, preset)
					jobType = "resize"
				default:
					continue
				}

				jobID, err := cfg.Broker.Enqueue(jobType, payload)
				if err != nil {
					log.Error("failed to enqueue job", "file_id", fileID, "preset", preset, "error", err)
					continue
				}
				jobIDs = append(jobIDs, jobID)

				if err := cfg.Queries.IncrementTransformationCount(r.Context(), pgUserID); err != nil {
					log.Error("failed to increment transformation count", "error", err)
				}
			}

			if req.WebP {
				payload := worker.NewWebPPayload(fileID, quality)
				jobID, err := cfg.Broker.Enqueue("webp", payload)
				if err != nil {
					log.Error("failed to enqueue webp job", "file_id", fileID, "error", err)
				} else {
					jobIDs = append(jobIDs, jobID)
					if err := cfg.Queries.IncrementTransformationCount(r.Context(), pgUserID); err != nil {
						log.Error("failed to increment transformation count", "error", err)
					}
				}
			}

			if req.Watermark != "" {
				payload := worker.NewWatermarkPayload(fileID, req.Watermark, "bottom-right", 0.5, isPremium)
				jobID, err := cfg.Broker.Enqueue("watermark", payload)
				if err != nil {
					log.Error("failed to enqueue watermark job", "file_id", fileID, "error", err)
				} else {
					jobIDs = append(jobIDs, jobID)
					if err := cfg.Queries.IncrementTransformationCount(r.Context(), pgUserID); err != nil {
						log.Error("failed to increment transformation count", "error", err)
					}
				}
			}

			if len(jobIDs) > 0 {
				pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
				_, err := cfg.Queries.CreateBatchItem(r.Context(), db.CreateBatchItemParams{
					BatchID: batch.ID,
					FileID:  pgFileID,
					JobIds:  jobIDs,
				})
				if err != nil {
					log.Error("failed to create batch item", "file_id", fileID, "error", err)
				}
				totalJobsCreated += len(jobIDs)
			}
		}

		log.Info("batch transform created", "total_files", len(validFileIDs), "total_jobs", totalJobsCreated)

		statusURL := fmt.Sprintf("/api/v1/batch/%s", batchIDStr)
		if cfg.BaseURL != "" {
			statusURL = cfg.BaseURL + statusURL
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(BatchTransformResponse{
			BatchID:    batchIDStr,
			TotalFiles: len(validFileIDs),
			TotalJobs:  totalJobsCreated,
			Status:     string(batch.Status),
			StatusURL:  statusURL,
		})
	}
}

type BatchStatusResponse struct {
	ID             string                    `json:"id"`
	Status         string                    `json:"status"`
	TotalFiles     int                       `json:"total_files"`
	CompletedFiles int                       `json:"completed_files"`
	FailedFiles    int                       `json:"failed_files"`
	Presets        []string                  `json:"presets"`
	WebP           bool                      `json:"webp"`
	Quality        int                       `json:"quality"`
	Watermark      *string                   `json:"watermark,omitempty"`
	CreatedAt      string                    `json:"created_at"`
	StartedAt      *string                   `json:"started_at,omitempty"`
	CompletedAt    *string                   `json:"completed_at,omitempty"`
	Items          []BatchItemStatusResponse `json:"items,omitempty"`
}

type BatchItemStatusResponse struct {
	FileID       string   `json:"file_id"`
	Status       string   `json:"status"`
	JobIDs       []string `json:"job_ids"`
	ErrorMessage *string  `json:"error_message,omitempty"`
}

func getBatchHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		batchIDStr := r.PathValue("id")
		batchID, err := uuid.Parse(batchIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_batch_id", "Invalid batch ID format", http.StatusBadRequest))
			return
		}

		if cfg.Queries == nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		pgBatchID := pgtype.UUID{Bytes: batchID, Valid: true}
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		batch, err := cfg.Queries.GetBatchOperationByUser(r.Context(), db.GetBatchOperationByUserParams{
			ID:     pgBatchID,
			UserID: pgUserID,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		response := BatchStatusResponse{
			ID:             uuidFromPgtype(batch.ID),
			Status:         string(batch.Status),
			TotalFiles:     int(batch.TotalFiles),
			CompletedFiles: int(batch.CompletedFiles),
			FailedFiles:    int(batch.FailedFiles),
			Presets:        batch.Presets,
			WebP:           batch.Webp,
			Quality:        int(batch.Quality),
			Watermark:      batch.Watermark,
			CreatedAt:      batch.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		if batch.StartedAt.Valid {
			startedAt := batch.StartedAt.Time.Format("2006-01-02T15:04:05Z07:00")
			response.StartedAt = &startedAt
		}

		if batch.CompletedAt.Valid {
			completedAt := batch.CompletedAt.Time.Format("2006-01-02T15:04:05Z07:00")
			response.CompletedAt = &completedAt
		}

		includeItems := r.URL.Query().Get("include_items") == "true"
		if includeItems {
			items, err := cfg.Queries.ListBatchItems(r.Context(), pgBatchID)
			if err == nil {
				response.Items = make([]BatchItemStatusResponse, len(items))
				for i, item := range items {
					response.Items[i] = BatchItemStatusResponse{
						FileID:       uuidFromPgtype(item.FileID),
						Status:       string(item.Status),
						JobIDs:       item.JobIds,
						ErrorMessage: item.ErrorMessage,
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
