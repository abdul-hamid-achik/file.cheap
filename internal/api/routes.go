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
			pgUserID.Scan(userID)

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
			json.NewEncoder(w).Encode(map[string]any{
				"id":       fileIDStr,
				"filename": dbFile.Filename,
				"status":   string(dbFile.Status),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
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
			json.NewEncoder(w).Encode(map[string]any{
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
