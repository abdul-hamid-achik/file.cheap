package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type BulkDownloadConfig struct {
	Queries Querier
	Broker  Broker
}

type BulkDownloadRequest struct {
	FileIDs []string `json:"file_ids"`
}

type BulkDownloadResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
}

type BulkDownloadStatusResponse struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	FileCount    int     `json:"file_count"`
	SizeBytes    *int64  `json:"size_bytes,omitempty"`
	DownloadURL  *string `json:"download_url,omitempty"`
	ExpiresAt    *string `json:"expires_at,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	CreatedAt    string  `json:"created_at"`
	CompletedAt  *string `json:"completed_at,omitempty"`
}

const maxBulkDownloadFiles = 100

// CreateBulkDownloadHandler creates a new ZIP download request
func CreateBulkDownloadHandler(cfg *BulkDownloadConfig, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		log = log.With("user_id", userID.String())

		var req BulkDownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid JSON request body", http.StatusBadRequest))
			return
		}

		if len(req.FileIDs) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_files", "At least one file ID is required", http.StatusBadRequest))
			return
		}

		if len(req.FileIDs) > maxBulkDownloadFiles {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "too_many_files",
				fmt.Sprintf("Maximum %d files per download. You requested %d.", maxBulkDownloadFiles, len(req.FileIDs)),
				http.StatusBadRequest))
			return
		}

		// Check for pending/running downloads
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		pendingCount, err := cfg.Queries.CountPendingZipDownloadsByUser(r.Context(), pgUserID)
		if err == nil && pendingCount >= 3 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "too_many_pending",
				"You have too many pending downloads. Please wait for them to complete.",
				http.StatusTooManyRequests))
			return
		}

		// Parse and validate file IDs
		var fileIDs []uuid.UUID
		for _, idStr := range req.FileIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				continue
			}
			fileIDs = append(fileIDs, id)
		}

		if len(fileIDs) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_valid_files", "No valid file IDs provided", http.StatusBadRequest))
			return
		}

		// Create zip download record
		pgFileIDs := make([]pgtype.UUID, len(fileIDs))
		for i, id := range fileIDs {
			pgFileIDs[i] = pgtype.UUID{Bytes: id, Valid: true}
		}

		zipDownload, err := cfg.Queries.CreateZipDownload(r.Context(), db.CreateZipDownloadParams{
			UserID:  pgUserID,
			FileIds: pgFileIDs,
		})
		if err != nil {
			log.Error("failed to create zip download", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		zipID := uuid.UUID(zipDownload.ID.Bytes)
		log = log.With("zip_download_id", zipID.String())

		// Enqueue job
		payload := worker.NewZipDownloadPayload(zipID, userID, fileIDs)
		jobID, err := cfg.Broker.Enqueue("zip_download", payload)
		if err != nil {
			log.Error("failed to enqueue zip download job", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		log.Info("zip download job enqueued", "job_id", jobID, "file_count", len(fileIDs))

		statusURL := fmt.Sprintf("/v1/downloads/%s", zipID.String())
		if baseURL != "" {
			statusURL = baseURL + statusURL
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(BulkDownloadResponse{
			ID:        zipID.String(),
			Status:    string(zipDownload.Status),
			StatusURL: statusURL,
		})
	}
}

// GetBulkDownloadHandler returns the status of a ZIP download request
func GetBulkDownloadHandler(cfg *BulkDownloadConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		downloadIDStr := r.PathValue("id")
		downloadID, err := uuid.Parse(downloadIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_download_id", "Invalid download ID format", http.StatusBadRequest))
			return
		}

		pgDownloadID := pgtype.UUID{Bytes: downloadID, Valid: true}
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		zipDownload, err := cfg.Queries.GetZipDownloadByUser(r.Context(), db.GetZipDownloadByUserParams{
			ID:     pgDownloadID,
			UserID: pgUserID,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		response := BulkDownloadStatusResponse{
			ID:        uuidFromPgtype(zipDownload.ID),
			Status:    string(zipDownload.Status),
			FileCount: len(zipDownload.FileIds),
			CreatedAt: zipDownload.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		if zipDownload.SizeBytes != nil {
			response.SizeBytes = zipDownload.SizeBytes
		}

		if zipDownload.DownloadUrl != nil {
			response.DownloadURL = zipDownload.DownloadUrl
		}

		if zipDownload.ExpiresAt.Valid {
			expiresAt := zipDownload.ExpiresAt.Time.Format("2006-01-02T15:04:05Z07:00")
			response.ExpiresAt = &expiresAt
		}

		if zipDownload.ErrorMessage != nil {
			response.ErrorMessage = zipDownload.ErrorMessage
		}

		if zipDownload.CompletedAt.Valid {
			completedAt := zipDownload.CompletedAt.Time.Format("2006-01-02T15:04:05Z07:00")
			response.CompletedAt = &completedAt
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

// ListBulkDownloadsHandler returns the user's ZIP download history
func ListBulkDownloadsHandler(cfg *BulkDownloadConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		limit := int32(20)
		offset := int32(0)

		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = int32(v)
			}
		}

		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.Atoi(o); err == nil && v >= 0 {
				offset = int32(v)
			}
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		downloads, err := cfg.Queries.ListZipDownloadsByUser(r.Context(), db.ListZipDownloadsByUserParams{
			UserID: pgUserID,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		results := make([]BulkDownloadStatusResponse, len(downloads))
		for i, d := range downloads {
			results[i] = BulkDownloadStatusResponse{
				ID:        uuidFromPgtype(d.ID),
				Status:    string(d.Status),
				FileCount: len(d.FileIds),
				CreatedAt: d.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			}
			if d.SizeBytes != nil {
				results[i].SizeBytes = d.SizeBytes
			}
			if d.DownloadUrl != nil {
				results[i].DownloadURL = d.DownloadUrl
			}
			if d.ExpiresAt.Valid {
				expiresAt := d.ExpiresAt.Time.Format("2006-01-02T15:04:05Z07:00")
				results[i].ExpiresAt = &expiresAt
			}
			if d.ErrorMessage != nil {
				results[i].ErrorMessage = d.ErrorMessage
			}
			if d.CompletedAt.Valid {
				completedAt := d.CompletedAt.Time.Format("2006-01-02T15:04:05Z07:00")
				results[i].CompletedAt = &completedAt
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"downloads": results,
		})
	}
}
