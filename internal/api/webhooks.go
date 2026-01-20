package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/webhook"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type WebhookQuerier interface {
	CreateWebhook(ctx context.Context, arg db.CreateWebhookParams) (db.Webhook, error)
	GetWebhook(ctx context.Context, arg db.GetWebhookParams) (db.Webhook, error)
	ListWebhooksByUser(ctx context.Context, arg db.ListWebhooksByUserParams) ([]db.Webhook, error)
	CountWebhooksByUser(ctx context.Context, userID pgtype.UUID) (int64, error)
	UpdateWebhook(ctx context.Context, arg db.UpdateWebhookParams) (db.Webhook, error)
	DeleteWebhook(ctx context.Context, arg db.DeleteWebhookParams) error
	ListDeliveriesByWebhook(ctx context.Context, arg db.ListDeliveriesByWebhookParams) ([]db.WebhookDelivery, error)
	CountDeliveriesByWebhook(ctx context.Context, webhookID pgtype.UUID) (int64, error)
	CreateWebhookDelivery(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error)
	// Webhook DLQ methods
	GetWebhookDLQEntry(ctx context.Context, id pgtype.UUID) (db.WebhookDlq, error)
	ListWebhookDLQByUser(ctx context.Context, arg db.ListWebhookDLQByUserParams) ([]db.WebhookDlq, error)
	CountWebhookDLQByUser(ctx context.Context, userID pgtype.UUID) (int64, error)
	MarkWebhookDLQRetried(ctx context.Context, id pgtype.UUID) error
	DeleteWebhookDLQEntry(ctx context.Context, id pgtype.UUID) error
}

type WebhookConfig struct {
	Queries WebhookQuerier
	Broker  Broker
}

type CreateWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type UpdateWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Active bool     `json:"active"`
}

type WebhookResponse struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Secret    string   `json:"secret,omitempty"`
	Events    []string `json:"events"`
	Active    bool     `json:"active"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type DeliveryResponse struct {
	ID            string  `json:"id"`
	EventType     string  `json:"event_type"`
	Status        string  `json:"status"`
	Attempts      int     `json:"attempts"`
	LastAttemptAt *string `json:"last_attempt_at,omitempty"`
	ResponseCode  *int    `json:"response_code,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

type ListWebhooksResponse struct {
	Webhooks   []WebhookResponse `json:"webhooks"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
}

type ListDeliveriesResponse struct {
	Deliveries []DeliveryResponse `json:"deliveries"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PerPage    int                `json:"per_page"`
	TotalPages int                `json:"total_pages"`
}

func CreateWebhookHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		userID, ok := GetUserID(ctx)
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		var req CreateWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.ErrBadRequest)
			return
		}

		if req.URL == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_url", "url is required", http.StatusBadRequest))
			return
		}

		if _, err := url.ParseRequestURI(req.URL); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_url", "invalid url", http.StatusBadRequest))
			return
		}

		if len(req.Events) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_events", "events is required", http.StatusBadRequest))
			return
		}

		for _, event := range req.Events {
			if !webhook.ValidEventTypes[event] {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_event_type", "invalid event type: "+event, http.StatusBadRequest))
				return
			}
		}

		secret, err := webhook.GenerateSecret()
		if err != nil {
			log.Error("failed to generate webhook secret", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		wh, err := cfg.Queries.CreateWebhook(ctx, db.CreateWebhookParams{
			UserID: pgUserID,
			Url:    req.URL,
			Secret: secret,
			Events: req.Events,
		})
		if err != nil {
			log.Error("failed to create webhook", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		resp := webhookToResponse(wh)
		resp.Secret = secret

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func ListWebhooksHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		userID, ok := GetUserID(ctx)
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		page, perPage := getWebhookPagination(r, 20, 100)
		offset := (page - 1) * perPage

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		webhooks, err := cfg.Queries.ListWebhooksByUser(ctx, db.ListWebhooksByUserParams{
			UserID: pgUserID,
			Limit:  int32(perPage),
			Offset: int32(offset),
		})
		if err != nil {
			log.Error("failed to list webhooks", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		total, err := cfg.Queries.CountWebhooksByUser(ctx, pgUserID)
		if err != nil {
			log.Error("failed to count webhooks", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		resp := ListWebhooksResponse{
			Webhooks:   make([]WebhookResponse, len(webhooks)),
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: int((total + int64(perPage) - 1) / int64(perPage)),
		}

		for i, wh := range webhooks {
			resp.Webhooks[i] = webhookToResponse(wh)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func GetWebhookHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		userID, ok := GetUserID(ctx)
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		webhookID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrBadRequest)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		pgWebhookID := pgtype.UUID{Bytes: webhookID, Valid: true}

		wh, err := cfg.Queries.GetWebhook(ctx, db.GetWebhookParams{
			ID:     pgWebhookID,
			UserID: pgUserID,
		})
		if err != nil {
			log.Error("failed to get webhook", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(webhookToResponse(wh))
	}
}

func UpdateWebhookHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		userID, ok := GetUserID(ctx)
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		webhookID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrBadRequest)
			return
		}

		var req UpdateWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.ErrBadRequest)
			return
		}

		if req.URL == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_url", "url is required", http.StatusBadRequest))
			return
		}

		if _, err := url.ParseRequestURI(req.URL); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_url", "invalid url", http.StatusBadRequest))
			return
		}

		if len(req.Events) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_events", "events is required", http.StatusBadRequest))
			return
		}

		for _, event := range req.Events {
			if !webhook.ValidEventTypes[event] {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_event_type", "invalid event type: "+event, http.StatusBadRequest))
				return
			}
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		pgWebhookID := pgtype.UUID{Bytes: webhookID, Valid: true}

		wh, err := cfg.Queries.UpdateWebhook(ctx, db.UpdateWebhookParams{
			ID:     pgWebhookID,
			UserID: pgUserID,
			Url:    req.URL,
			Events: req.Events,
			Active: req.Active,
		})
		if err != nil {
			log.Error("failed to update webhook", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(webhookToResponse(wh))
	}
}

func DeleteWebhookHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		userID, ok := GetUserID(ctx)
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		webhookID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrBadRequest)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		pgWebhookID := pgtype.UUID{Bytes: webhookID, Valid: true}

		err = cfg.Queries.DeleteWebhook(ctx, db.DeleteWebhookParams{
			ID:     pgWebhookID,
			UserID: pgUserID,
		})
		if err != nil {
			log.Error("failed to delete webhook", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func ListDeliveriesHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		userID, ok := GetUserID(ctx)
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		webhookID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrBadRequest)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		pgWebhookID := pgtype.UUID{Bytes: webhookID, Valid: true}

		_, err = cfg.Queries.GetWebhook(ctx, db.GetWebhookParams{
			ID:     pgWebhookID,
			UserID: pgUserID,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		page, perPage := getWebhookPagination(r, 20, 100)
		offset := (page - 1) * perPage

		deliveries, err := cfg.Queries.ListDeliveriesByWebhook(ctx, db.ListDeliveriesByWebhookParams{
			WebhookID: pgWebhookID,
			Limit:     int32(perPage),
			Offset:    int32(offset),
		})
		if err != nil {
			log.Error("failed to list deliveries", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		total, err := cfg.Queries.CountDeliveriesByWebhook(ctx, pgWebhookID)
		if err != nil {
			log.Error("failed to count deliveries", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		resp := ListDeliveriesResponse{
			Deliveries: make([]DeliveryResponse, len(deliveries)),
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: int((total + int64(perPage) - 1) / int64(perPage)),
		}

		for i, d := range deliveries {
			resp.Deliveries[i] = deliveryToResponse(d)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func TestWebhookHandler(cfg *WebhookConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		userID, ok := GetUserID(ctx)
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		webhookID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrBadRequest)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		pgWebhookID := pgtype.UUID{Bytes: webhookID, Valid: true}

		wh, err := cfg.Queries.GetWebhook(ctx, db.GetWebhookParams{
			ID:     pgWebhookID,
			UserID: pgUserID,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		testEvent, err := webhook.NewEvent("test", map[string]any{
			"message": "This is a test webhook delivery",
		})
		if err != nil {
			log.Error("failed to create test event", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		payloadBytes, err := testEvent.Marshal()
		if err != nil {
			log.Error("failed to marshal test event", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		delivery, err := cfg.Queries.CreateWebhookDelivery(ctx, db.CreateWebhookDeliveryParams{
			WebhookID: wh.ID,
			EventType: "test",
			Payload:   payloadBytes,
		})
		if err != nil {
			log.Error("failed to create test delivery", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		_, err = cfg.Broker.Enqueue("webhook_delivery", webhook.DeliveryPayload{
			DeliveryID: webhookUUIDToString(delivery.ID),
		})
		if err != nil {
			log.Error("failed to enqueue test delivery", "error", err)
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message":     "test webhook queued",
			"delivery_id": webhookUUIDToString(delivery.ID),
		})
	}
}

func webhookToResponse(wh db.Webhook) WebhookResponse {
	return WebhookResponse{
		ID:        webhookUUIDToString(wh.ID),
		URL:       wh.Url,
		Events:    wh.Events,
		Active:    wh.Active,
		CreatedAt: wh.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: wh.UpdatedAt.Time.Format(time.RFC3339),
	}
}

func deliveryToResponse(d db.WebhookDelivery) DeliveryResponse {
	resp := DeliveryResponse{
		ID:        webhookUUIDToString(d.ID),
		EventType: d.EventType,
		Status:    string(d.Status),
		Attempts:  int(d.Attempts),
		CreatedAt: d.CreatedAt.Time.Format(time.RFC3339),
	}

	if d.LastAttemptAt.Valid {
		t := d.LastAttemptAt.Time.Format(time.RFC3339)
		resp.LastAttemptAt = &t
	}

	if d.ResponseCode != nil {
		code := int(*d.ResponseCode)
		resp.ResponseCode = &code
	}

	return resp
}

func webhookUUIDToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u := uuid.UUID(id.Bytes)
	return u.String()
}

func getWebhookPagination(r *http.Request, defaultPerPage, maxPerPage int) (page, perPage int) {
	page = 1
	perPage = defaultPerPage

	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= maxPerPage {
			perPage = parsed
		}
	}

	return page, perPage
}
