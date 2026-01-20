package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/metrics"
	"github.com/abdul-hamid-achik/file.cheap/internal/webhook"
	"github.com/abdul-hamid-achik/job-queue/pkg/job"
	"github.com/abdul-hamid-achik/job-queue/pkg/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxRetries      = 10
	deliveryTimeout = 30 * time.Second
	maxResponseBody = 1024
)

// calculateBackoff calculates exponential backoff with jitter
// Base delay is 30 seconds, max is 4 hours
func calculateBackoff(attempt int) time.Duration {
	base := 30 * time.Second
	max := 4 * time.Hour

	// Exponential: 30s, 1m, 2m, 4m, 8m, 16m, 32m, 64m, 128m, 256m (capped at 4h)
	backoff := base * time.Duration(1<<uint(attempt))
	if backoff > max {
		backoff = max
	}

	// Add jitter: ±25%
	jitter := time.Duration(rand.Float64()*0.5-0.25) * backoff
	return backoff + jitter
}

type WebhookDependencies struct {
	Queries    WebhookQuerier
	HTTPClient *http.Client
}

type WebhookQuerier interface {
	GetWebhookDelivery(ctx context.Context, id pgtype.UUID) (db.WebhookDelivery, error)
	GetWebhookByDeliveryID(ctx context.Context, id pgtype.UUID) (db.Webhook, error)
	MarkDeliverySuccess(ctx context.Context, arg db.MarkDeliverySuccessParams) error
	MarkDeliveryFailed(ctx context.Context, arg db.MarkDeliveryFailedParams) error
	UpdateDeliveryRetry(ctx context.Context, arg db.UpdateDeliveryRetryParams) error
	// DLQ support
	CreateWebhookDLQEntry(ctx context.Context, arg db.CreateWebhookDLQEntryParams) (db.WebhookDlq, error)
}

func WebhookDeliveryHandler(deps *WebhookDependencies) func(context.Context, *job.Job) error {
	return func(ctx context.Context, j *job.Job) error {
		log := logger.FromContext(ctx).With("job_id", j.ID, "job_type", "webhook_delivery")
		log.Info("webhook delivery job started")
		startTime := time.Now()

		var payload webhook.DeliveryPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			log.Error("invalid payload", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid payload: %w", err))
		}

		deliveryID, err := uuid.Parse(payload.DeliveryID)
		if err != nil {
			log.Error("invalid delivery ID", "error", err)
			return middleware.Permanent(fmt.Errorf("invalid delivery ID: %w", err))
		}

		pgDeliveryID := pgtype.UUID{Bytes: deliveryID, Valid: true}
		log = log.With("delivery_id", payload.DeliveryID)

		delivery, err := deps.Queries.GetWebhookDelivery(ctx, pgDeliveryID)
		if err != nil {
			log.Error("failed to get delivery", "error", err)
			return fmt.Errorf("failed to get delivery: %w", err)
		}

		wh, err := deps.Queries.GetWebhookByDeliveryID(ctx, pgDeliveryID)
		if err != nil {
			log.Error("failed to get webhook", "error", err)
			return fmt.Errorf("failed to get webhook: %w", err)
		}

		webhookID := uuidToString(wh.ID)
		log = log.With("webhook_id", webhookID, "webhook_url", wh.Url)

		timestamp := time.Now()
		signature := webhook.GenerateSignature(delivery.Payload, wh.Secret, timestamp)
		signatureHeader := webhook.BuildSignatureHeader(signature, timestamp)

		req, err := http.NewRequestWithContext(ctx, "POST", wh.Url, bytes.NewReader(delivery.Payload))
		if err != nil {
			log.Error("failed to create request", "error", err)
			return middleware.Permanent(fmt.Errorf("failed to create request: %w", err))
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Signature", signatureHeader)
		req.Header.Set("X-Webhook-ID", payload.DeliveryID)
		req.Header.Set("User-Agent", "file.cheap-webhook/1.0")

		client := deps.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: deliveryTimeout}
		}

		resp, err := client.Do(req)
		deliveryDuration := time.Since(startTime).Seconds()

		var responseCode int32
		var responseBody string

		if err != nil {
			responseBody = err.Error()
			log.Warn("webhook request failed", "error", err, "duration_seconds", deliveryDuration)
		} else {
			responseCode = int32(resp.StatusCode)
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
			responseBody = string(body)
			_ = resp.Body.Close()
		}

		if responseCode >= 200 && responseCode < 300 {
			if err := deps.Queries.MarkDeliverySuccess(ctx, db.MarkDeliverySuccessParams{
				ID:           pgDeliveryID,
				ResponseCode: &responseCode,
				ResponseBody: &responseBody,
			}); err != nil {
				log.Error("failed to mark success", "error", err)
			}
			metrics.RecordWebhookDelivery("success", deliveryDuration)
			log.Info("webhook delivered successfully", "response_code", responseCode, "duration_seconds", deliveryDuration)
			return nil
		}

		newAttempts := int(delivery.Attempts) + 1
		log.Warn("webhook delivery failed", "response_code", responseCode, "attempts", newAttempts, "max_retries", maxRetries)

		if newAttempts >= maxRetries {
			if err := deps.Queries.MarkDeliveryFailed(ctx, db.MarkDeliveryFailedParams{
				ID:           pgDeliveryID,
				ResponseCode: &responseCode,
				ResponseBody: &responseBody,
			}); err != nil {
				log.Error("failed to mark failed", "error", err)
			}

			// Add to Dead Letter Queue for visibility and manual retry
			finalError := fmt.Sprintf("max retries exceeded after %d attempts, last response: %d", newAttempts, responseCode)
			if responseCode == 0 {
				finalError = fmt.Sprintf("max retries exceeded after %d attempts, connection error: %s", newAttempts, responseBody)
			}
			dlqResponseCode := int32(responseCode)
			_, dlqErr := deps.Queries.CreateWebhookDLQEntry(ctx, db.CreateWebhookDLQEntryParams{
				WebhookID:        wh.ID,
				DeliveryID:       pgDeliveryID,
				EventType:        delivery.EventType,
				Payload:          delivery.Payload,
				FinalError:       finalError,
				Attempts:         int32(newAttempts),
				LastResponseCode: &dlqResponseCode,
				LastResponseBody: &responseBody,
			})
			if dlqErr != nil {
				log.Error("failed to add to DLQ", "error", dlqErr)
			} else {
				log.Warn("webhook delivery added to dead letter queue", "dlq_error", finalError)
				metrics.RecordWebhookDLQ()
			}

			metrics.RecordWebhookDelivery("failed", deliveryDuration)
			log.Warn("webhook delivery permanently failed after max retries", "attempts", newAttempts)
			return middleware.Permanent(fmt.Errorf("max retries exceeded after %d attempts", newAttempts))
		}

		retryDelay := calculateBackoff(newAttempts - 1)
		nextRetry := time.Now().Add(retryDelay)

		if err := deps.Queries.UpdateDeliveryRetry(ctx, db.UpdateDeliveryRetryParams{
			ID:           pgDeliveryID,
			NextRetryAt:  pgtype.Timestamptz{Time: nextRetry, Valid: true},
			ResponseCode: &responseCode,
			ResponseBody: &responseBody,
		}); err != nil {
			log.Error("failed to update retry", "error", err)
		}

		metrics.RecordWebhookRetry(webhookID)
		metrics.RecordWebhookDelivery("retry", deliveryDuration)
		log.Info("webhook delivery scheduled for retry", "next_retry", nextRetry, "retry_delay", retryDelay, "attempt", newAttempts)
		return fmt.Errorf("delivery failed with status %d, will retry", responseCode)
	}
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u := uuid.UUID(id.Bytes)
	return u.String()
}
