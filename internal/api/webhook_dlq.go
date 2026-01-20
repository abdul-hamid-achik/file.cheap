package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type WebhookDLQResponse struct {
	ID               string  `json:"id"`
	WebhookID        string  `json:"webhook_id"`
	DeliveryID       *string `json:"delivery_id,omitempty"`
	EventType        string  `json:"event_type"`
	FinalError       string  `json:"final_error"`
	Attempts         int     `json:"attempts"`
	LastResponseCode *int    `json:"last_response_code,omitempty"`
	CanRetry         bool    `json:"can_retry"`
	RetriedAt        *string `json:"retried_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// ListWebhookDLQHandler returns failed webhook deliveries for a user
func ListWebhookDLQHandler(cfg *WebhookConfig) http.HandlerFunc {
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

		entries, err := cfg.Queries.ListWebhookDLQByUser(r.Context(), db.ListWebhookDLQByUserParams{
			UserID: pgUserID,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		total, _ := cfg.Queries.CountWebhookDLQByUser(r.Context(), pgUserID)

		results := make([]WebhookDLQResponse, len(entries))
		for i, e := range entries {
			results[i] = WebhookDLQResponse{
				ID:         uuidFromPgtype(e.ID),
				WebhookID:  uuidFromPgtype(e.WebhookID),
				EventType:  e.EventType,
				FinalError: e.FinalError,
				Attempts:   int(e.Attempts),
				CanRetry:   e.CanRetry,
				CreatedAt:  e.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			}
			if e.DeliveryID.Valid {
				deliveryID := uuidFromPgtype(e.DeliveryID)
				results[i].DeliveryID = &deliveryID
			}
			if e.LastResponseCode != nil {
				code := int(*e.LastResponseCode)
				results[i].LastResponseCode = &code
			}
			if e.RetriedAt.Valid {
				retriedAt := e.RetriedAt.Time.Format("2006-01-02T15:04:05Z07:00")
				results[i].RetriedAt = &retriedAt
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entries":  results,
			"total":    total,
			"has_more": int64(offset)+int64(len(entries)) < total,
		})
	}
}

// RetryWebhookDLQHandler retries a failed webhook delivery
func RetryWebhookDLQHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		entryIDStr := r.PathValue("id")
		entryID, err := uuid.Parse(entryIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_entry_id", "Invalid DLQ entry ID format", http.StatusBadRequest))
			return
		}

		pgEntryID := pgtype.UUID{Bytes: entryID, Valid: true}

		// Get the DLQ entry
		entry, err := cfg.Queries.GetWebhookDLQEntry(r.Context(), pgEntryID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		// Verify ownership through webhook
		webhook, err := cfg.Queries.GetWebhook(r.Context(), db.GetWebhookParams{
			ID:     entry.WebhookID,
			UserID: pgtype.UUID{Bytes: userID, Valid: true},
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		if !entry.CanRetry {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "cannot_retry", "This entry has already been retried", http.StatusBadRequest))
			return
		}

		// Create a new delivery and enqueue it
		delivery, err := cfg.Queries.CreateWebhookDelivery(r.Context(), db.CreateWebhookDeliveryParams{
			WebhookID: webhook.ID,
			EventType: entry.EventType,
			Payload:   entry.Payload,
		})
		if err != nil {
			log.Error("failed to create retry delivery", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		// Enqueue the delivery
		_, err = cfg.Broker.Enqueue("webhook_delivery", map[string]string{
			"delivery_id": uuidFromPgtype(delivery.ID),
		})
		if err != nil {
			log.Error("failed to enqueue retry delivery", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		// Mark the DLQ entry as retried
		if err := cfg.Queries.MarkWebhookDLQRetried(r.Context(), pgEntryID); err != nil {
			log.Warn("failed to mark DLQ entry as retried", "error", err)
		}

		log.Info("webhook DLQ entry retried", "entry_id", entryIDStr, "new_delivery_id", uuidFromPgtype(delivery.ID))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"retried":         true,
			"new_delivery_id": uuidFromPgtype(delivery.ID),
		})
	}
}

// DeleteWebhookDLQEntryHandler deletes a DLQ entry
func DeleteWebhookDLQEntryHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		entryIDStr := r.PathValue("id")
		entryID, err := uuid.Parse(entryIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_entry_id", "Invalid DLQ entry ID format", http.StatusBadRequest))
			return
		}

		pgEntryID := pgtype.UUID{Bytes: entryID, Valid: true}

		// Get the DLQ entry
		entry, err := cfg.Queries.GetWebhookDLQEntry(r.Context(), pgEntryID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		// Verify ownership through webhook
		_, err = cfg.Queries.GetWebhook(r.Context(), db.GetWebhookParams{
			ID:     entry.WebhookID,
			UserID: pgtype.UUID{Bytes: userID, Valid: true},
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		if err := cfg.Queries.DeleteWebhookDLQEntry(r.Context(), pgEntryID); err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
