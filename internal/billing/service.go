package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stripe/stripe-go/v83"
)

type Service struct {
	client  *Client
	queries *db.Queries
	baseURL string
}

func NewService(client *Client, queries *db.Queries, baseURL string) *Service {
	return &Service{
		client:  client,
		queries: queries,
		baseURL: baseURL,
	}
}

func (s *Service) IsConfigured() bool {
	return s.client != nil && s.client.IsConfigured()
}

func (s *Service) GetSubscriptionInfo(ctx context.Context, userID uuid.UUID) (*SubscriptionInfo, error) {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	billing, err := s.queries.GetUserBillingInfo(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get billing info: %w", err)
	}

	filesCount, err := s.queries.GetUserFilesCount(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files count: %w", err)
	}

	info := &SubscriptionInfo{
		Tier:        billing.SubscriptionTier,
		Status:      billing.SubscriptionStatus,
		FilesLimit:  int(billing.FilesLimit),
		MaxFileSize: billing.MaxFileSize,
		FilesUsed:   filesCount,
	}

	if billing.SubscriptionPeriodEnd.Valid {
		info.PeriodEnd = &billing.SubscriptionPeriodEnd.Time
	}
	if billing.TrialEndsAt.Valid {
		info.TrialEndsAt = &billing.TrialEndsAt.Time
	}

	return info, nil
}

func (s *Service) StartTrial(ctx context.Context, userID uuid.UUID) error {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	trialEnd := time.Now().Add(TrialDuration)

	_, err := s.queries.StartUserTrial(ctx, db.StartUserTrialParams{
		ID:          pgUserID,
		TrialEndsAt: pgtype.Timestamptz{Time: trialEnd, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to start trial: %w", err)
	}

	return nil
}

func (s *Service) CreateCheckoutSession(ctx context.Context, userID uuid.UUID, email string) (string, error) {
	if !s.IsConfigured() {
		return "", fmt.Errorf("stripe not configured")
	}

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgUserID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	var customerID string
	if user.StripeCustomerID != nil && *user.StripeCustomerID != "" {
		customerID = *user.StripeCustomerID
	} else {
		customerParams := &stripe.CustomerCreateParams{
			Email: stripe.String(email),
			Metadata: map[string]string{
				"user_id": userID.String(),
			},
		}
		customer, err := s.client.StripeClient().V1Customers.Create(ctx, customerParams)
		if err != nil {
			return "", fmt.Errorf("failed to create stripe customer: %w", err)
		}
		customerID = customer.ID

		_, err = s.queries.UpdateUserStripeCustomer(ctx, db.UpdateUserStripeCustomerParams{
			ID:               pgUserID,
			StripeCustomerID: &customerID,
		})
		if err != nil {
			return "", fmt.Errorf("failed to update stripe customer id: %w", err)
		}
	}

	params := &stripe.CheckoutSessionCreateParams{
		Customer: stripe.String(customerID),
		Mode:     stripe.String("subscription"),
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{
			{
				Price:    stripe.String(s.client.PriceIDPro()),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(s.baseURL + "/billing?success=1"),
		CancelURL:  stripe.String(s.baseURL + "/billing?canceled=1"),
		SubscriptionData: &stripe.CheckoutSessionCreateSubscriptionDataParams{
			Metadata: map[string]string{
				"user_id": userID.String(),
			},
		},
	}

	session, err := s.client.StripeClient().V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to create checkout session: %w", err)
	}

	return session.URL, nil
}

func (s *Service) CreatePortalSession(ctx context.Context, userID uuid.UUID) (string, error) {
	if !s.IsConfigured() {
		return "", fmt.Errorf("stripe not configured")
	}

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgUserID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if user.StripeCustomerID == nil || *user.StripeCustomerID == "" {
		return "", fmt.Errorf("user has no stripe customer id")
	}

	params := &stripe.BillingPortalSessionCreateParams{
		Customer:  user.StripeCustomerID,
		ReturnURL: stripe.String(s.baseURL + "/billing"),
	}

	session, err := s.client.StripeClient().V1BillingPortalSessions.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to create portal session: %w", err)
	}

	return session.URL, nil
}

func (s *Service) HandleSubscriptionCreated(ctx context.Context, sub *stripe.Subscription) error {
	userID, err := s.getUserIDFromSubscription(ctx, sub)
	if err != nil {
		return err
	}

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	status := mapStripeStatus(sub.Status)
	var periodEnd pgtype.Timestamptz
	if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].CurrentPeriodEnd > 0 {
		periodEnd = pgtype.Timestamptz{Time: time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0), Valid: true}
	}

	var trialEnd pgtype.Timestamptz
	if sub.TrialEnd > 0 {
		trialEnd = pgtype.Timestamptz{Time: time.Unix(sub.TrialEnd, 0), Valid: true}
	}

	_, err = s.queries.UpdateUserSubscription(ctx, db.UpdateUserSubscriptionParams{
		ID:                    pgUserID,
		StripeSubscriptionID:  &sub.ID,
		SubscriptionStatus:    status,
		SubscriptionTier:      db.SubscriptionTierPro,
		SubscriptionPeriodEnd: periodEnd,
		TrialEndsAt:           trialEnd,
		FilesLimit:            ProFilesLimit,
		MaxFileSize:           ProMaxFileSize,
	})
	if err != nil {
		return fmt.Errorf("failed to update user subscription: %w", err)
	}

	return nil
}

func (s *Service) HandleSubscriptionUpdated(ctx context.Context, sub *stripe.Subscription) error {
	userID, err := s.getUserIDFromSubscription(ctx, sub)
	if err != nil {
		return err
	}

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	status := mapStripeStatus(sub.Status)
	var periodEnd pgtype.Timestamptz
	if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].CurrentPeriodEnd > 0 {
		periodEnd = pgtype.Timestamptz{Time: time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0), Valid: true}
	}

	_, err = s.queries.UpdateUserSubscriptionStatus(ctx, db.UpdateUserSubscriptionStatusParams{
		ID:                    pgUserID,
		SubscriptionStatus:    status,
		SubscriptionPeriodEnd: periodEnd,
	})
	if err != nil {
		return fmt.Errorf("failed to update subscription status: %w", err)
	}

	return nil
}

func (s *Service) HandleSubscriptionDeleted(ctx context.Context, sub *stripe.Subscription) error {
	userID, err := s.getUserIDFromSubscription(ctx, sub)
	if err != nil {
		return err
	}

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	_, err = s.queries.CancelUserSubscription(ctx, pgUserID)
	if err != nil {
		return fmt.Errorf("failed to cancel user subscription: %w", err)
	}

	return nil
}

func (s *Service) getUserIDFromSubscription(ctx context.Context, sub *stripe.Subscription) (uuid.UUID, error) {
	if userIDStr, ok := sub.Metadata["user_id"]; ok && userIDStr != "" {
		return uuid.Parse(userIDStr)
	}

	if sub.Customer != nil {
		user, err := s.queries.GetUserByStripeCustomerID(ctx, &sub.Customer.ID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to find user by customer id: %w", err)
		}
		userID, err := uuid.FromBytes(user.ID.Bytes[:])
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid user id: %w", err)
		}
		return userID, nil
	}

	return uuid.Nil, fmt.Errorf("could not determine user from subscription")
}

func mapStripeStatus(status stripe.SubscriptionStatus) db.SubscriptionStatus {
	switch status {
	case stripe.SubscriptionStatusTrialing:
		return db.SubscriptionStatusTrialing
	case stripe.SubscriptionStatusActive:
		return db.SubscriptionStatusActive
	case stripe.SubscriptionStatusPastDue:
		return db.SubscriptionStatusPastDue
	case stripe.SubscriptionStatusCanceled:
		return db.SubscriptionStatusCanceled
	case stripe.SubscriptionStatusUnpaid:
		return db.SubscriptionStatusUnpaid
	default:
		return db.SubscriptionStatusNone
	}
}

func (s *Service) CheckTrialExpiration(ctx context.Context, userID uuid.UUID) error {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	billing, err := s.queries.GetUserBillingInfo(ctx, pgUserID)
	if err != nil {
		return fmt.Errorf("failed to get billing info: %w", err)
	}

	if billing.SubscriptionStatus != db.SubscriptionStatusTrialing {
		return nil
	}

	if !billing.TrialEndsAt.Valid {
		return nil
	}

	if time.Now().After(billing.TrialEndsAt.Time) {
		_, err = s.queries.CancelUserSubscription(ctx, pgUserID)
		if err != nil {
			return fmt.Errorf("failed to expire trial: %w", err)
		}
	}

	return nil
}
