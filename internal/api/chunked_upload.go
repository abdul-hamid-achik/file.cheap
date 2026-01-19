package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/billing"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/metrics"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/video"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	"github.com/abdul-hamid-achik/file.cheap/internal/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ChunkedUploadConfig holds configuration for chunked uploads
type ChunkedUploadConfig struct {
	Storage       storage.Storage
	Queries       Querier
	Broker        Broker
	MaxUploadSize int64
	ChunkSize     int64 // Default chunk size (5MB minimum for S3)
}

// uploadSession tracks an in-progress chunked upload
type uploadSession struct {
	ID           string
	UserID       uuid.UUID
	Filename     string
	ContentType  string
	TotalSize    int64
	ChunksTotal  int
	ChunksLoaded map[int]bool
	StorageKey   string
	CreatedAt    time.Time
	mu           sync.Mutex
}

// uploadSessionStore stores active upload sessions (in-memory for single instance)
// For production, use Redis
type uploadSessionStore struct {
	sessions map[string]*uploadSession
	mu       sync.RWMutex
	done     chan struct{}
}

var sessionStore = &uploadSessionStore{
	sessions: make(map[string]*uploadSession),
	done:     make(chan struct{}),
}

// StartCleanup starts a background goroutine to clean up expired upload sessions
func (s *uploadSessionStore) StartCleanup(ctx context.Context, storageClient storage.Storage) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanupExpired(ctx, storageClient)
		case <-s.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

// StopCleanup stops the cleanup goroutine
func (s *uploadSessionStore) StopCleanup() {
	select {
	case <-s.done:
		// already closed
	default:
		close(s.done)
	}
}

// cleanupExpired removes sessions older than 1 hour and their orphaned chunks
func (s *uploadSessionStore) cleanupExpired(ctx context.Context, storageClient storage.Storage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	for id, session := range s.sessions {
		if session.CreatedAt.Before(cutoff) {
			// Delete orphaned chunks from storage
			for i := 0; i < session.ChunksTotal; i++ {
				chunkKey := fmt.Sprintf("%s.chunk.%d", session.StorageKey, i)
				_ = storageClient.Delete(ctx, chunkKey)
			}
			delete(s.sessions, id)
		}
	}
}

// GetSessionStore returns the global session store for cleanup initialization
func GetSessionStore() *uploadSessionStore {
	return sessionStore
}

func (s *uploadSessionStore) Get(id string) (*uploadSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	return session, ok
}

func (s *uploadSessionStore) Set(session *uploadSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
}

func (s *uploadSessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// InitUploadRequest is the request to start a chunked upload
type InitUploadRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	TotalSize   int64  `json:"total_size"`
}

// InitUploadResponse is returned when starting a chunked upload
type InitUploadResponse struct {
	UploadID    string `json:"upload_id"`
	ChunkSize   int64  `json:"chunk_size"`
	ChunksTotal int    `json:"chunks_total"`
}

// UploadChunkResponse is returned after uploading a chunk
type UploadChunkResponse struct {
	UploadID     string `json:"upload_id"`
	ChunkIndex   int    `json:"chunk_index"`
	ChunksLoaded int    `json:"chunks_loaded"`
	ChunksTotal  int    `json:"chunks_total"`
	Complete     bool   `json:"complete"`
	FileID       string `json:"file_id,omitempty"`
}

// InitChunkedUploadHandler starts a new chunked upload session
func InitChunkedUploadHandler(cfg *ChunkedUploadConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		var req InitUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid request body", http.StatusBadRequest))
			return
		}

		if req.Filename == "" || req.TotalSize <= 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "missing_required_fields", "Filename and total_size are required", http.StatusBadRequest))
			return
		}

		billingInfo := GetBilling(r.Context())
		if billingInfo != nil {
			if billingInfo.FilesLimit >= 0 && billingInfo.FilesCount >= int64(billingInfo.FilesLimit) {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "file_limit_reached", "File limit reached", http.StatusForbidden))
				return
			}

			maxSize := billingInfo.MaxFileSize
			limits := billing.GetTierLimits(billingInfo.Tier)

			if video.IsVideoType(req.ContentType) {
				maxSize = limits.MaxVideoSize

				// Check video storage quota
				pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
				videoUsageBytes, err := cfg.Queries.GetUserVideoStorageUsage(r.Context(), pgUserID)
				if err == nil && videoUsageBytes+req.TotalSize > limits.VideoStorageBytes {
					apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "video_storage_quota_exceeded", "Video storage quota exceeded, please upgrade or delete old videos", http.StatusForbidden))
					return
				}
			}

			if req.TotalSize > maxSize {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "file_too_large", fmt.Sprintf("File too large, max size: %d MB", maxSize/(1024*1024)), http.StatusForbidden))
				return
			}
		}

		chunkSize := cfg.ChunkSize
		if chunkSize <= 0 {
			chunkSize = 5 * 1024 * 1024 // 5MB default
		}
		chunksTotal := int((req.TotalSize + chunkSize - 1) / chunkSize)

		uploadID := uuid.New().String()
		storageKey := fmt.Sprintf("uploads/%s/%s/%s", userID.String(), uploadID, req.Filename)

		session := &uploadSession{
			ID:           uploadID,
			UserID:       userID,
			Filename:     req.Filename,
			ContentType:  req.ContentType,
			TotalSize:    req.TotalSize,
			ChunksTotal:  chunksTotal,
			ChunksLoaded: make(map[int]bool),
			StorageKey:   storageKey,
			CreatedAt:    time.Now(),
		}

		sessionStore.Set(session)

		log.Info("chunked upload initiated",
			"upload_id", uploadID,
			"filename", req.Filename,
			"total_size", req.TotalSize,
			"chunks_total", chunksTotal,
		)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(InitUploadResponse{
			UploadID:    uploadID,
			ChunkSize:   chunkSize,
			ChunksTotal: chunksTotal,
		})
	}
}

// UploadChunkHandler handles uploading a single chunk
func UploadChunkHandler(cfg *ChunkedUploadConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		uploadID := r.PathValue("uploadId")
		chunkIndexStr := r.URL.Query().Get("chunk")
		chunkIndex, err := strconv.Atoi(chunkIndexStr)
		if err != nil || chunkIndex < 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_chunk_index", "Invalid chunk index", http.StatusBadRequest))
			return
		}

		session, ok := sessionStore.Get(uploadID)
		if !ok {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "session_not_found", "Upload session not found", http.StatusNotFound))
			return
		}

		if session.UserID != userID {
			apperror.WriteJSON(w, r, apperror.ErrForbidden)
			return
		}

		if chunkIndex >= session.ChunksTotal {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "chunk_out_of_range", "Chunk index out of range", http.StatusBadRequest))
			return
		}

		chunkData, err := io.ReadAll(r.Body)
		if err != nil {
			log.Error("failed to read chunk data", "error", err)
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "read_chunk_failed", "Failed to read chunk data", http.StatusBadRequest))
			return
		}

		chunkKey := fmt.Sprintf("%s.chunk.%d", session.StorageKey, chunkIndex)
		if err := cfg.Storage.Upload(r.Context(), chunkKey, strings.NewReader(string(chunkData)), "application/octet-stream", int64(len(chunkData))); err != nil {
			log.Error("failed to store chunk", "error", err)
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		session.mu.Lock()
		session.ChunksLoaded[chunkIndex] = true
		chunksLoaded := len(session.ChunksLoaded)
		complete := chunksLoaded == session.ChunksTotal
		session.mu.Unlock()

		log.Debug("chunk uploaded",
			"upload_id", uploadID,
			"chunk_index", chunkIndex,
			"chunks_loaded", chunksLoaded,
			"complete", complete,
		)

		response := UploadChunkResponse{
			UploadID:     uploadID,
			ChunkIndex:   chunkIndex,
			ChunksLoaded: chunksLoaded,
			ChunksTotal:  session.ChunksTotal,
			Complete:     complete,
		}

		if complete {
			fileID, err := assembleChunks(r.Context(), cfg, session, log)
			if err != nil {
				log.Error("failed to assemble chunks", "error", err)
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "assemble_failed", "Failed to assemble file", http.StatusInternalServerError))
				return
			}
			response.FileID = fileID
			sessionStore.Delete(uploadID)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

// assembleChunks combines all chunks into the final file using streaming
func assembleChunks(ctx context.Context, cfg *ChunkedUploadConfig, session *uploadSession, log *slog.Logger) (string, error) {
	startTime := time.Now()

	// Create a pipe for streaming assembly
	pr, pw := io.Pipe()

	// Writer goroutine - streams chunks sequentially to the pipe
	go func() {
		defer func() { _ = pw.Close() }()
		for i := 0; i < session.ChunksTotal; i++ {
			chunkKey := fmt.Sprintf("%s.chunk.%d", session.StorageKey, i)
			reader, err := cfg.Storage.Download(ctx, chunkKey)
			if err != nil {
				pw.CloseWithError(fmt.Errorf("failed to download chunk %d: %w", i, err))
				return
			}
			if _, err := io.Copy(pw, reader); err != nil {
				_ = reader.Close()
				pw.CloseWithError(fmt.Errorf("failed to copy chunk %d: %w", i, err))
				return
			}
			_ = reader.Close()
		}
	}()

	// Upload from pipe reader (streams directly to storage)
	if err := cfg.Storage.Upload(ctx, session.StorageKey, pr, session.ContentType, session.TotalSize); err != nil {
		metrics.RecordFileUpload("error", 0, time.Since(startTime).Seconds())
		return "", fmt.Errorf("failed to upload combined file: %w", err)
	}

	// Clean up chunks after successful assembly
	for i := 0; i < session.ChunksTotal; i++ {
		chunkKey := fmt.Sprintf("%s.chunk.%d", session.StorageKey, i)
		_ = cfg.Storage.Delete(ctx, chunkKey)
	}

	if cfg.Queries != nil {
		pgUserID := pgtype.UUID{Bytes: session.UserID, Valid: true}

		contentType := session.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		dbFile, err := cfg.Queries.CreateFile(ctx, db.CreateFileParams{
			UserID:      pgUserID,
			Filename:    session.Filename,
			ContentType: contentType,
			SizeBytes:   session.TotalSize,
			StorageKey:  session.StorageKey,
			Status:      db.FileStatusPending,
		})
		if err != nil {
			metrics.RecordFileUpload("error", 0, time.Since(startTime).Seconds())
			return "", fmt.Errorf("failed to create file record: %w", err)
		}

		metrics.RecordFileUpload("success", session.TotalSize, time.Since(startTime).Seconds())
		fileIDStr := uuidFromPgtype(dbFile.ID)
		log.Info("file record created from chunked upload", "file_id", fileIDStr, "filename", session.Filename)

		if cfg.Broker != nil {
			var fileUUID uuid.UUID
			copy(fileUUID[:], dbFile.ID.Bytes[:])

			switch {
			case strings.HasPrefix(contentType, "image/"):
				payload := worker.NewThumbnailPayload(fileUUID)
				if jobID, err := worker.EnqueueWithTracking(ctx, cfg.Queries, cfg.Broker, &payload, db.JobTypeThumbnail); err != nil {
					log.Error("failed to enqueue thumbnail job", "error", err)
				} else {
					metrics.RecordJobEnqueued("thumbnail")
					log.Info("thumbnail job enqueued", "job_id", jobID)
				}
			case contentType == "application/pdf":
				payload := worker.NewPDFThumbnailPayload(fileUUID)
				if jobID, err := worker.EnqueueWithTracking(ctx, cfg.Queries, cfg.Broker, &payload, db.JobTypePdfThumbnail); err != nil {
					log.Error("failed to enqueue pdf_thumbnail job", "error", err)
				} else {
					metrics.RecordJobEnqueued("pdf_thumbnail")
					log.Info("pdf_thumbnail job enqueued", "job_id", jobID)
				}
			case video.IsVideoType(contentType):
				payload := worker.NewVideoThumbnailPayload(fileUUID)
				if jobID, err := worker.EnqueueWithTracking(ctx, cfg.Queries, cfg.Broker, &payload, db.JobTypeVideoThumbnail); err != nil {
					log.Error("failed to enqueue video_thumbnail job", "error", err)
				} else {
					metrics.RecordJobEnqueued("video_thumbnail")
					log.Info("video_thumbnail job enqueued", "job_id", jobID)
				}
			}
		}

		return fileIDStr, nil
	}

	return session.ID, nil
}

// GetUploadStatusHandler returns the status of an upload session
func GetUploadStatusHandler(cfg *ChunkedUploadConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		uploadID := r.PathValue("uploadId")
		session, ok := sessionStore.Get(uploadID)
		if !ok {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "session_not_found", "Upload session not found", http.StatusNotFound))
			return
		}

		if session.UserID != userID {
			apperror.WriteJSON(w, r, apperror.ErrForbidden)
			return
		}

		session.mu.Lock()
		chunksLoaded := len(session.ChunksLoaded)
		session.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"upload_id":     uploadID,
			"filename":      session.Filename,
			"total_size":    session.TotalSize,
			"chunks_total":  session.ChunksTotal,
			"chunks_loaded": chunksLoaded,
			"complete":      chunksLoaded == session.ChunksTotal,
		})
	}
}

// CancelUploadHandler cancels an in-progress upload
func CancelUploadHandler(cfg *ChunkedUploadConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		uploadID := r.PathValue("uploadId")
		session, ok := sessionStore.Get(uploadID)
		if !ok {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "session_not_found", "Upload session not found", http.StatusNotFound))
			return
		}

		if session.UserID != userID {
			apperror.WriteJSON(w, r, apperror.ErrForbidden)
			return
		}

		for i := 0; i < session.ChunksTotal; i++ {
			chunkKey := fmt.Sprintf("%s.chunk.%d", session.StorageKey, i)
			_ = cfg.Storage.Delete(r.Context(), chunkKey)
		}

		sessionStore.Delete(uploadID)

		log.Info("chunked upload cancelled", "upload_id", uploadID)

		w.WriteHeader(http.StatusNoContent)
	}
}
