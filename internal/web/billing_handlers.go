package web

import (
	"net/http"

	"github.com/abdul-hamid-achik/file.cheap/internal/auth"
	"github.com/abdul-hamid-achik/file.cheap/internal/billing"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/web/templates/pages"
)

type BillingHandlers struct {
	billingService *billing.Service
	webhookHandler *billing.WebhookHandler
}

func NewBillingHandlers(service *billing.Service, webhookHandler *billing.WebhookHandler) *BillingHandlers {
	return &BillingHandlers{
		billingService: service,
		webhookHandler: webhookHandler,
	}
}

func (h *BillingHandlers) Billing(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data := pages.BillingPageData{
		StripeConfigured: h.billingService != nil && h.billingService.IsConfigured(),
	}

	if r.URL.Query().Get("success") == "1" {
		data.Success = "Your subscription has been activated! Thank you for upgrading to Pro."
	}
	if r.URL.Query().Get("canceled") == "1" {
		data.Error = "Checkout was canceled. You can try again whenever you're ready."
	}
	if r.URL.Query().Get("trial_started") == "1" {
		data.Success = "Your 7-day Pro trial has started! Enjoy all premium features."
	}

	if h.billingService != nil {
		info, err := h.billingService.GetSubscriptionInfo(r.Context(), user.ID)
		if err != nil {
			log.Error("failed to get subscription info", "error", err)
		} else {
			data.Subscription = pages.SubscriptionData{
				Tier:              string(info.Tier),
				Status:            string(info.Status),
				FilesLimit:        info.FilesLimit,
				FilesUsed:         int(info.FilesUsed),
				MaxFileSize:       info.MaxFileSize,
				IsPro:             info.IsPro(),
				IsTrialing:        info.Status == "trialing",
				TrialDaysLeft:     info.TrialDaysRemaining(),
				UsagePercent:      info.UsagePercent(),
				CanUpgrade:        !info.IsPro(),
				HasStripeCustomer: user.SubscriptionTier != "free" || info.Status != "none",
			}
			if info.PeriodEnd != nil {
				data.Subscription.PeriodEnd = info.PeriodEnd.Format("January 2, 2006")
			}
			if info.TrialEndsAt != nil {
				data.Subscription.TrialEndsAt = info.TrialEndsAt.Format("January 2, 2006")
			}
		}
	}

	_ = pages.Billing(user, data).Render(r.Context(), w)
}

func (h *BillingHandlers) StartTrial(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if h.billingService == nil {
		http.Redirect(w, r, "/billing?error=not_configured", http.StatusFound)
		return
	}

	info, err := h.billingService.GetSubscriptionInfo(r.Context(), user.ID)
	if err != nil {
		log.Error("failed to get subscription info", "error", err)
		http.Redirect(w, r, "/billing?error=internal", http.StatusFound)
		return
	}

	if info.IsPro() {
		http.Redirect(w, r, "/billing?error=already_pro", http.StatusFound)
		return
	}

	if info.Status == "trialing" || info.Status == "canceled" {
		http.Redirect(w, r, "/billing?error=trial_used", http.StatusFound)
		return
	}

	if err := h.billingService.StartTrial(r.Context(), user.ID); err != nil {
		log.Error("failed to start trial", "error", err)
		http.Redirect(w, r, "/billing?error=trial_failed", http.StatusFound)
		return
	}

	log.Info("user started trial", "user_id", user.ID.String())
	http.Redirect(w, r, "/billing?trial_started=1", http.StatusFound)
}

func (h *BillingHandlers) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if h.billingService == nil || !h.billingService.IsConfigured() {
		http.Redirect(w, r, "/billing?error=not_configured", http.StatusFound)
		return
	}

	url, err := h.billingService.CreateCheckoutSession(r.Context(), user.ID, user.Email)
	if err != nil {
		log.Error("failed to create checkout session", "error", err)
		http.Redirect(w, r, "/billing?error=checkout_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *BillingHandlers) CreatePortal(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if h.billingService == nil || !h.billingService.IsConfigured() {
		http.Redirect(w, r, "/billing?error=not_configured", http.StatusFound)
		return
	}

	url, err := h.billingService.CreatePortalSession(r.Context(), user.ID)
	if err != nil {
		log.Error("failed to create portal session", "error", err)
		http.Redirect(w, r, "/billing?error=portal_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *BillingHandlers) Webhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookHandler == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	h.webhookHandler.HandleWebhook(w, r)
}
