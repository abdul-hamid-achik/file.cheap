package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type JobQuerier interface {
	GetJobByUser(ctx context.Context, arg db.GetJobByUserParams) (db.GetJobByUserRow, error)
	GetJob(ctx context.Context, id pgtype.UUID) (db.ProcessingJob, error)
	RetryJob(ctx context.Context, id pgtype.UUID) error
	CancelJob(ctx context.Context, id pgtype.UUID) error
	BulkRetryFailedJobs(ctx context.Context, userID pgtype.UUID) error
	ListJobsByUserWithStatus(ctx context.Context, arg db.ListJobsByUserWithStatusParams) ([]db.ListJobsByUserWithStatusRow, error)
	CountJobsByUser(ctx context.Context, arg db.CountJobsByUserParams) (int64, error)
}

type JobConfig struct {
	Queries JobQuerier
}

type JobResponse struct {
	ID           string  `json:"id"`
	FileID       string  `json:"file_id"`
	Filename     string  `json:"filename"`
	ContentType  string  `json:"content_type"`
	JobType      string  `json:"job_type"`
	Status       string  `json:"status"`
	Priority     int     `json:"priority"`
	Attempts     int     `json:"attempts"`
	ErrorMessage *string `json:"error_message,omitempty"`
	CreatedAt    string  `json:"created_at"`
	StartedAt    *string `json:"started_at,omitempty"`
	CompletedAt  *string `json:"completed_at,omitempty"`
}

type JobListResponse struct {
	Jobs    []JobResponse `json:"jobs"`
	Total   int64         `json:"total"`
	HasMore bool          `json:"has_more"`
}

func ListJobsHandler(cfg *JobConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")
		statusStr := r.URL.Query().Get("status")

		limit := int32(20)
		offset := int32(0)

		if limitStr != "" {
			l, err := strconv.Atoi(limitStr)
			if err != nil || l < 0 || l > 100 {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_limit", "Invalid limit parameter", http.StatusBadRequest))
				return
			}
			limit = int32(l)
		}

		if offsetStr != "" {
			o, err := strconv.Atoi(offsetStr)
			if err != nil || o < 0 {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_offset", "Invalid offset parameter", http.StatusBadRequest))
				return
			}
			offset = int32(o)
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		var statusFilter db.JobStatus
		if statusStr != "" {
			statusFilter = db.JobStatus(statusStr)
		}

		jobs, err := cfg.Queries.ListJobsByUserWithStatus(r.Context(), db.ListJobsByUserWithStatusParams{
			UserID:  pgUserID,
			Column2: statusFilter,
			Limit:   limit,
			Offset:  offset,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		total, err := cfg.Queries.CountJobsByUser(r.Context(), db.CountJobsByUserParams{
			UserID:  pgUserID,
			Column2: statusFilter,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		jobResponses := make([]JobResponse, len(jobs))
		for i, j := range jobs {
			resp := JobResponse{
				ID:          uuidFromPgtype(j.ID),
				FileID:      uuidFromPgtype(j.FileID),
				Filename:    j.Filename,
				ContentType: j.ContentType,
				JobType:     string(j.JobType),
				Status:      string(j.Status),
				Priority:    int(j.Priority),
				Attempts:    int(j.Attempts),
				CreatedAt:   j.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			}
			if j.ErrorMessage != nil {
				resp.ErrorMessage = j.ErrorMessage
			}
			if j.StartedAt.Valid {
				s := j.StartedAt.Time.Format("2006-01-02T15:04:05Z07:00")
				resp.StartedAt = &s
			}
			if j.CompletedAt.Valid {
				c := j.CompletedAt.Time.Format("2006-01-02T15:04:05Z07:00")
				resp.CompletedAt = &c
			}
			jobResponses[i] = resp
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JobListResponse{
			Jobs:    jobResponses,
			Total:   total,
			HasMore: int64(offset)+int64(len(jobs)) < total,
		})
	}
}

func RetryJobHandler(cfg *JobConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		jobIDStr := r.PathValue("id")
		jobID, err := uuid.Parse(jobIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_job_id", "Invalid job ID format", http.StatusBadRequest))
			return
		}

		pgJobID := pgtype.UUID{Bytes: jobID, Valid: true}
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		job, err := cfg.Queries.GetJobByUser(r.Context(), db.GetJobByUserParams{
			ID:     pgJobID,
			UserID: pgUserID,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		if job.Status != db.JobStatusFailed {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_status", "Only failed jobs can be retried", http.StatusBadRequest))
			return
		}

		if err := cfg.Queries.RetryJob(r.Context(), pgJobID); err != nil {
			log.Error("failed to retry job", "job_id", jobIDStr, "error", err)
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		log.Info("job retry requested", "job_id", jobIDStr)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      jobIDStr,
			"status":  "pending",
			"message": "Job queued for retry",
		})
	}
}

func CancelJobHandler(cfg *JobConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		jobIDStr := r.PathValue("id")
		jobID, err := uuid.Parse(jobIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_job_id", "Invalid job ID format", http.StatusBadRequest))
			return
		}

		pgJobID := pgtype.UUID{Bytes: jobID, Valid: true}
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		job, err := cfg.Queries.GetJobByUser(r.Context(), db.GetJobByUserParams{
			ID:     pgJobID,
			UserID: pgUserID,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		if job.Status != db.JobStatusPending && job.Status != db.JobStatusRunning {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_status", "Only pending or running jobs can be cancelled", http.StatusBadRequest))
			return
		}

		if err := cfg.Queries.CancelJob(r.Context(), pgJobID); err != nil {
			log.Error("failed to cancel job", "job_id", jobIDStr, "error", err)
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		log.Info("job cancelled", "job_id", jobIDStr)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      jobIDStr,
			"status":  "failed",
			"message": "Job cancelled",
		})
	}
}

func BulkRetryJobsHandler(cfg *JobConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		if err := cfg.Queries.BulkRetryFailedJobs(r.Context(), pgUserID); err != nil {
			log.Error("failed to bulk retry jobs", "error", err)
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		log.Info("bulk job retry requested", "user_id", userID.String())

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "All failed jobs queued for retry",
		})
	}
}
