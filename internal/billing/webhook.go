package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/webhook"
)

type WebhookHandler struct {
	service *Service
	secret  string
	logger  *slog.Logger
}

func NewWebhookHandler(service *Service, secret string, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		service: service,
		secret:  secret,
		logger:  logger,
	}
}

func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	const maxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	signatureHeader := r.Header.Get("Stripe-Signature")
	if signatureHeader == "" {
		h.logger.Warn("missing stripe signature header")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(payload, signatureHeader, h.secret)
	if err != nil {
		h.logger.Error("webhook signature verification failed", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	if err := h.handleEvent(ctx, event); err != nil {
		h.logger.Error("failed to handle webhook event", "type", event.Type, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) handleEvent(ctx context.Context, event stripe.Event) error {
	h.logger.Info("processing webhook event", "type", event.Type, "id", event.ID)

	switch event.Type {
	case "checkout.session.completed":
		return h.handleCheckoutCompleted(ctx, event)
	case "customer.subscription.created":
		return h.handleSubscriptionCreated(ctx, event)
	case "customer.subscription.updated":
		return h.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return h.handleSubscriptionDeleted(ctx, event)
	case "invoice.payment_failed":
		return h.handlePaymentFailed(ctx, event)
	case "invoice.payment_succeeded":
		return h.handlePaymentSucceeded(ctx, event)
	default:
		h.logger.Debug("unhandled event type", "type", event.Type)
		return nil
	}
}

func (h *WebhookHandler) handleCheckoutCompleted(ctx context.Context, event stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return fmt.Errorf("failed to unmarshal checkout session: %w", err)
	}

	h.logger.Info("checkout session completed",
		"session_id", session.ID,
		"customer_id", session.Customer.ID,
		"subscription_id", session.Subscription.ID,
	)

	return nil
}

func (h *WebhookHandler) handleSubscriptionCreated(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	h.logger.Info("subscription created",
		"subscription_id", sub.ID,
		"status", sub.Status,
	)

	return h.service.HandleSubscriptionCreated(ctx, &sub)
}

func (h *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	h.logger.Info("subscription updated",
		"subscription_id", sub.ID,
		"status", sub.Status,
	)

	return h.service.HandleSubscriptionUpdated(ctx, &sub)
}

func (h *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	h.logger.Info("subscription deleted", "subscription_id", sub.ID)

	return h.service.HandleSubscriptionDeleted(ctx, &sub)
}

func (h *WebhookHandler) handlePaymentFailed(ctx context.Context, event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %w", err)
	}

	h.logger.Warn("payment failed",
		"invoice_id", invoice.ID,
		"customer_id", invoice.Customer.ID,
		"attempt_count", invoice.AttemptCount,
	)

	return nil
}

func (h *WebhookHandler) handlePaymentSucceeded(ctx context.Context, event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %w", err)
	}

	h.logger.Info("payment succeeded",
		"invoice_id", invoice.ID,
		"customer_id", invoice.Customer.ID,
	)

	return nil
}
