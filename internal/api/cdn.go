package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/logger"
	"github.com/abdul-hamid-achik/file-processor/internal/processor"
	"github.com/abdul-hamid-achik/file-processor/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	cacheThreshold = 3
	// For redirects to presigned URLs that expire in 3600 seconds,
	// cache for at most 3000 seconds to allow for some buffer
	cacheControlRedirect = "public, max-age=3000"
	// For processed results served directly (not cached yet)
	cacheControlShort = "public, max-age=3600"
)

type CDNQuerier interface {
	GetFileShareByToken(ctx context.Context, token string) (db.GetFileShareByTokenRow, error)
	IncrementShareAccessCount(ctx context.Context, id pgtype.UUID) error
	GetTransformCache(ctx context.Context, arg db.GetTransformCacheParams) (db.TransformCache, error)
	CreateTransformCache(ctx context.Context, arg db.CreateTransformCacheParams) (db.TransformCache, error)
	IncrementTransformCacheCount(ctx context.Context, arg db.IncrementTransformCacheCountParams) error
	GetTransformRequestCount(ctx context.Context, arg db.GetTransformRequestCountParams) (int32, error)
	GetFile(ctx context.Context, id pgtype.UUID) (db.File, error)
	CreateFileShare(ctx context.Context, arg db.CreateFileShareParams) (db.FileShare, error)
	ListFileSharesByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileShare, error)
	DeleteFileShare(ctx context.Context, arg db.DeleteFileShareParams) error
}

type CDNConfig struct {
	Storage  storage.Storage
	Queries  CDNQuerier
	Registry *processor.Registry
}

func GenerateShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(b)
	token = strings.TrimRight(token, "=")
	if len(token) > 43 {
		token = token[:43]
	}
	return token, nil
}

func CDNHandler(cfg *CDNConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		token := r.PathValue("token")
		transforms := r.PathValue("transforms")
		filename := r.PathValue("filename")

		if token == "" {
			http.Error(w, `{"error":{"code":"bad_request","message":"missing share token"}}`, http.StatusBadRequest)
			return
		}

		share, err := cfg.Queries.GetFileShareByToken(r.Context(), token)
		if err != nil {
			log.Debug("share not found", "token", token, "error", err)
			http.Error(w, `{"error":{"code":"not_found","message":"share not found or expired"}}`, http.StatusNotFound)
			return
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = cfg.Queries.IncrementShareAccessCount(ctx, share.ID)
		}()

		opts, err := ParseTransforms(transforms)
		if err != nil {
			log.Debug("invalid transforms", "transforms", transforms, "error", err)
			http.Error(w, fmt.Sprintf(`{"error":{"code":"bad_request","message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}

		if err := ValidateTransforms(opts); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"bad_request","message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}

		if len(share.AllowedTransforms) > 0 {
			if !isTransformAllowed(transforms, share.AllowedTransforms) {
				http.Error(w, `{"error":{"code":"forbidden","message":"transform not allowed for this share"}}`, http.StatusForbidden)
				return
			}
		}

		if !opts.RequiresProcessing() {
			serveOriginal(w, r, cfg, share.StorageKey, share.ContentType, filename)
			return
		}

		cacheKey := opts.CacheKey()
		fileID := share.FileID

		cached, err := cfg.Queries.GetTransformCache(r.Context(), db.GetTransformCacheParams{
			FileID:   fileID,
			CacheKey: cacheKey,
		})
		if err == nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = cfg.Queries.IncrementTransformCacheCount(ctx, db.IncrementTransformCacheCountParams{
					FileID:   fileID,
					CacheKey: cacheKey,
				})
			}()

			serveCached(w, r, cfg, cached.StorageKey, cached.ContentType, filename)
			return
		}

		requestCount, _ := cfg.Queries.GetTransformRequestCount(r.Context(), db.GetTransformRequestCountParams{
			FileID:   fileID,
			CacheKey: cacheKey,
		})

		shouldCache := requestCount >= cacheThreshold

		result, err := processTransform(r.Context(), cfg, share.StorageKey, share.ContentType, opts)
		if err != nil {
			log.Error("transform failed", "error", err)
			http.Error(w, `{"error":{"code":"processing_error","message":"failed to process image"}}`, http.StatusInternalServerError)
			return
		}

		if shouldCache {
			cacheResult(r.Context(), cfg, fileID, cacheKey, transforms, result, log)
		}

		serveResult(w, r, result, filename)
	}
}

func isTransformAllowed(requested string, allowed []string) bool {
	if requested == "" || requested == "_" || requested == "original" {
		return true
	}
	for _, a := range allowed {
		if a == "*" || a == requested {
			return true
		}
	}
	return false
}

func serveOriginal(w http.ResponseWriter, r *http.Request, cfg *CDNConfig, storageKey, contentType, filename string) {
	url, err := cfg.Storage.GetPresignedURL(r.Context(), storageKey, 3600)
	if err != nil {
		http.Error(w, `{"error":{"code":"internal","message":"failed to generate URL"}}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", cacheControlRedirect)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func serveCached(w http.ResponseWriter, r *http.Request, cfg *CDNConfig, storageKey, contentType, filename string) {
	url, err := cfg.Storage.GetPresignedURL(r.Context(), storageKey, 3600)
	if err != nil {
		http.Error(w, `{"error":{"code":"internal","message":"failed to generate URL"}}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", cacheControlRedirect)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func processTransform(ctx context.Context, cfg *CDNConfig, storageKey, contentType string, opts *TransformOptions) (*processor.Result, error) {
	reader, err := cfg.Storage.Download(ctx, storageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer func() { _ = reader.Close() }()

	procName := opts.ProcessorNameForContentType(contentType)
	if procName == "" {
		procName = "resize"
	}

	proc, ok := cfg.Registry.Get(procName)
	if !ok {
		return nil, fmt.Errorf("processor not found: %s", procName)
	}

	result, err := proc.Process(ctx, opts.ToProcessorOptions(), reader)
	if err != nil {
		return nil, fmt.Errorf("processing failed: %w", err)
	}

	return result, nil
}

func cacheResult(ctx context.Context, cfg *CDNConfig, fileID pgtype.UUID, cacheKey, transforms string, result *processor.Result, log *slog.Logger) {
	data, err := io.ReadAll(result.Data)
	if err != nil {
		log.Warn("failed to read result for caching", "error", err)
		return
	}

	fileUUID, _ := uuid.FromBytes(fileID.Bytes[:])
	storageKey := fmt.Sprintf("cache/%s/%s/%s", fileUUID.String(), cacheKey, result.Filename)

	uploadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cfg.Storage.Upload(uploadCtx, storageKey, bytes.NewReader(data), result.ContentType, int64(len(data))); err != nil {
		log.Warn("failed to upload cache", "error", err)
		return
	}

	var width, height *int32
	if result.Metadata.Width > 0 {
		w := int32(result.Metadata.Width)
		width = &w
	}
	if result.Metadata.Height > 0 {
		h := int32(result.Metadata.Height)
		height = &h
	}

	_, err = cfg.Queries.CreateTransformCache(uploadCtx, db.CreateTransformCacheParams{
		FileID:          fileID,
		CacheKey:        cacheKey,
		TransformParams: transforms,
		StorageKey:      storageKey,
		ContentType:     result.ContentType,
		SizeBytes:       int64(len(data)),
		Width:           width,
		Height:          height,
	})
	if err != nil {
		log.Warn("failed to save cache record", "error", err)
	}
}

func serveResult(w http.ResponseWriter, r *http.Request, result *processor.Result, filename string) {
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Cache-Control", cacheControlShort)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))

	_, _ = io.Copy(w, result.Data)
}

func CreateShareHandler(cfg *CDNConfig, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, `{"error":{"code":"unauthorized","message":"authentication required"}}`, http.StatusUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			http.Error(w, `{"error":{"code":"bad_request","message":"invalid file ID"}}`, http.StatusBadRequest)
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			http.Error(w, `{"error":{"code":"not_found","message":"file not found"}}`, http.StatusNotFound)
			return
		}

		fileUserID, _ := uuid.FromBytes(file.UserID.Bytes[:])
		if fileUserID != userID {
			http.Error(w, `{"error":{"code":"not_found","message":"file not found"}}`, http.StatusNotFound)
			return
		}

		token, err := GenerateShareToken()
		if err != nil {
			log.Error("failed to generate share token", "error", err)
			http.Error(w, `{"error":{"code":"internal","message":"failed to create share"}}`, http.StatusInternalServerError)
			return
		}

		var expiresAt pgtype.Timestamptz
		if expStr := r.URL.Query().Get("expires"); expStr != "" {
			if d, err := time.ParseDuration(expStr); err == nil {
				expiresAt = pgtype.Timestamptz{Time: time.Now().Add(d), Valid: true}
			}
		}

		share, err := cfg.Queries.CreateFileShare(r.Context(), db.CreateFileShareParams{
			FileID:    pgFileID,
			Token:     token,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			log.Error("failed to create share", "error", err)
			http.Error(w, `{"error":{"code":"internal","message":"failed to create share"}}`, http.StatusInternalServerError)
			return
		}

		shareURL := fmt.Sprintf("%s/cdn/%s/_/%s", baseURL, token, file.Filename)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"id":"%s","token":"%s","share_url":"%s"`, uuidFromPgtype(share.ID), token, shareURL)
		if expiresAt.Valid {
			_, _ = fmt.Fprintf(w, `,"expires_at":"%s"`, expiresAt.Time.Format(time.RFC3339))
		}
		_, _ = fmt.Fprint(w, "}")
	}
}

func ListSharesHandler(cfg *CDNConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, `{"error":{"code":"unauthorized","message":"authentication required"}}`, http.StatusUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			http.Error(w, `{"error":{"code":"bad_request","message":"invalid file ID"}}`, http.StatusBadRequest)
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			http.Error(w, `{"error":{"code":"not_found","message":"file not found"}}`, http.StatusNotFound)
			return
		}

		fileUserID, _ := uuid.FromBytes(file.UserID.Bytes[:])
		if fileUserID != userID {
			http.Error(w, `{"error":{"code":"not_found","message":"file not found"}}`, http.StatusNotFound)
			return
		}

		shares, err := cfg.Queries.ListFileSharesByFile(r.Context(), pgFileID)
		if err != nil {
			http.Error(w, `{"error":{"code":"internal","message":"failed to list shares"}}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"shares":[`)
		for i, s := range shares {
			if i > 0 {
				_, _ = fmt.Fprint(w, ",")
			}
			_, _ = fmt.Fprintf(w, `{"id":"%s","token":"%s","access_count":%d,"created_at":"%s"`,
				uuidFromPgtype(s.ID), s.Token, s.AccessCount, s.CreatedAt.Time.Format(time.RFC3339))
			if s.ExpiresAt.Valid {
				_, _ = fmt.Fprintf(w, `,"expires_at":"%s"`, s.ExpiresAt.Time.Format(time.RFC3339))
			}
			_, _ = fmt.Fprint(w, "}")
		}
		_, _ = fmt.Fprint(w, "]}")
	}
}

func DeleteShareHandler(cfg *CDNConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, `{"error":{"code":"unauthorized","message":"authentication required"}}`, http.StatusUnauthorized)
			return
		}

		shareIDStr := r.PathValue("shareId")
		shareID, err := uuid.Parse(shareIDStr)
		if err != nil {
			http.Error(w, `{"error":{"code":"bad_request","message":"invalid share ID"}}`, http.StatusBadRequest)
			return
		}

		pgShareID := pgtype.UUID{Bytes: shareID, Valid: true}
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		err = cfg.Queries.DeleteFileShare(r.Context(), db.DeleteFileShareParams{
			ID:     pgShareID,
			UserID: pgUserID,
		})
		if err != nil {
			http.Error(w, `{"error":{"code":"not_found","message":"share not found"}}`, http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
