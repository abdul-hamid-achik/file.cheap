package billing

import (
	"github.com/stripe/stripe-go/v83"
)

type Client struct {
	client         *stripe.Client
	webhookSecret  string
	priceIDPro     string
	publishableKey string
}

func NewClient(secretKey, publishableKey, webhookSecret, priceIDPro string) *Client {
	if secretKey == "" {
		return nil
	}

	return &Client{
		client:         stripe.NewClient(secretKey),
		webhookSecret:  webhookSecret,
		priceIDPro:     priceIDPro,
		publishableKey: publishableKey,
	}
}

func (c *Client) IsConfigured() bool {
	return c != nil && c.client != nil
}

func (c *Client) PublishableKey() string {
	if c == nil {
		return ""
	}
	return c.publishableKey
}

func (c *Client) PriceIDPro() string {
	if c == nil {
		return ""
	}
	return c.priceIDPro
}

func (c *Client) WebhookSecret() string {
	if c == nil {
		return ""
	}
	return c.webhookSecret
}

func (c *Client) StripeClient() *stripe.Client {
	if c == nil {
		return nil
	}
	return c.client
}
