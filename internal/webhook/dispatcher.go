package webhook

import (
	"context"
	"log/slog"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type DispatcherQuerier interface {
	ListActiveWebhooksByUserAndEvent(ctx context.Context, arg db.ListActiveWebhooksByUserAndEventParams) ([]db.Webhook, error)
	CreateWebhookDelivery(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error)
}

type Broker interface {
	Enqueue(jobType string, payload any) (string, error)
}

type DeliveryPayload struct {
	DeliveryID string `json:"delivery_id"`
}

type Dispatcher struct {
	queries DispatcherQuerier
	broker  Broker
	logger  *slog.Logger
}

func NewDispatcher(queries DispatcherQuerier, broker Broker) *Dispatcher {
	return &Dispatcher{
		queries: queries,
		broker:  broker,
		logger:  slog.Default(),
	}
}

func (d *Dispatcher) WithLogger(log *slog.Logger) *Dispatcher {
	d.logger = log
	return d
}

func (d *Dispatcher) Dispatch(ctx context.Context, userID uuid.UUID, event *Event) error {
	log := logger.FromContext(ctx).With("event_type", event.Type, "event_id", event.ID)

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	webhooks, err := d.queries.ListActiveWebhooksByUserAndEvent(ctx, db.ListActiveWebhooksByUserAndEventParams{
		UserID:    pgUserID,
		EventType: event.Type,
	})
	if err != nil {
		log.Error("failed to list webhooks", "error", err)
		return err
	}

	if len(webhooks) == 0 {
		log.Debug("no active webhooks for event")
		return nil
	}

	payloadBytes, err := event.Marshal()
	if err != nil {
		log.Error("failed to marshal event", "error", err)
		return err
	}

	for _, webhook := range webhooks {
		delivery, err := d.queries.CreateWebhookDelivery(ctx, db.CreateWebhookDeliveryParams{
			WebhookID: webhook.ID,
			EventType: event.Type,
			Payload:   payloadBytes,
		})
		if err != nil {
			log.Error("failed to create delivery", "webhook_id", uuidToString(webhook.ID), "error", err)
			continue
		}

		_, err = d.broker.Enqueue("webhook_delivery", DeliveryPayload{
			DeliveryID: uuidToString(delivery.ID),
		})
		if err != nil {
			log.Error("failed to enqueue delivery", "delivery_id", uuidToString(delivery.ID), "error", err)
		}
	}

	log.Info("dispatched webhook event", "webhook_count", len(webhooks))
	return nil
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u := uuid.UUID(id.Bytes)
	return u.String()
}
