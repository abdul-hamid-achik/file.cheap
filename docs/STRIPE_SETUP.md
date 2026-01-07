# Stripe Integration Setup Guide

This guide walks you through setting up Stripe for the file.cheap billing system.

## Overview

The billing system implements:
- **Pro Plan**: $19/month for 2,000 files
- **7-day Free Trial**: Explicit opt-in (user clicks "Start Trial")
- **Stripe Checkout**: Hosted payment page
- **Customer Portal**: Self-service subscription management
- **Webhooks**: Automatic subscription status updates

## Prerequisites

- A Stripe account (test mode for development)
- Access to the Stripe Dashboard
- The application deployed or running locally

---

## Step 1: Get Your API Keys

1. Log in to [Stripe Dashboard](https://dashboard.stripe.com)
2. Make sure you're in **Test mode** (toggle in the top-right)
3. Go to **Developers → API keys**
4. Copy your keys:
   - **Publishable key**: `pk_test_...`
   - **Secret key**: `sk_test_...` (click "Reveal test key")

Add these to your `.env` file:

```bash
STRIPE_SECRET_KEY=sk_test_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
STRIPE_PUBLISHABLE_KEY=pk_test_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

---

## Step 2: Create the Pro Plan Product

### 2.1 Create the Product

1. Go to **Products** in the Stripe Dashboard
2. Click **+ Add product**
3. Fill in:
   - **Name**: `Pro Plan`
   - **Description**: `2,000 files, 100 MB max file size, all processing features, full API access`
   - **Image**: (optional) Upload a product image

### 2.2 Add the Price

1. Under **Pricing**, click **Add price**
2. Configure:
   - **Pricing model**: Standard pricing
   - **Price**: `$19.00`
   - **Billing period**: Monthly
   - **Usage type**: Licensed (not metered)
3. Click **Save product**

### 2.3 Copy the Price ID

1. After saving, click on the product to view details
2. Under **Pricing**, find your monthly price
3. Copy the **Price ID** (starts with `price_`)

Add to your `.env` file:

```bash
STRIPE_PRICE_ID_PRO=price_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

---

## Step 3: Configure the Customer Portal

The Customer Portal lets users manage their subscription without your involvement.

1. Go to **Settings → Billing → Customer portal**
2. Click **Activate test link** (or configure if already active)
3. Configure these settings:

### Business Information
- **Business name**: file.cheap
- **Terms of service URL**: `https://your-domain.com/terms`
- **Privacy policy URL**: `https://your-domain.com/privacy`

### Features
Enable these features:
- ✅ **Customers can update their payment methods**
- ✅ **Customers can update subscriptions** → Allow switching plans (if you add more later)
- ✅ **Customers can cancel subscriptions**
  - Cancellation mode: **At end of billing period**
  - Proration behavior: **None**
- ✅ **Customers can view their invoice history**

### Products
- Click **+ Add product** and select your "Pro Plan"

4. Click **Save changes**

---

## Step 4: Set Up Webhooks

Webhooks notify your application when subscription events occur (new subscription, payment failed, cancellation, etc.).

### 4.1 Create the Webhook Endpoint

1. Go to **Developers → Webhooks**
2. Click **+ Add endpoint**
3. Configure:
   - **Endpoint URL**: `https://your-domain.com/billing/webhook`
   - **Description**: file.cheap billing webhooks
   - **Listen to**: Events on your account

### 4.2 Select Events to Listen For

Click **Select events** and check:

**Checkout**
- `checkout.session.completed`

**Customer**
- `customer.subscription.created`
- `customer.subscription.updated`
- `customer.subscription.deleted`

**Invoice**
- `invoice.payment_succeeded`
- `invoice.payment_failed`

4. Click **Add endpoint**

### 4.3 Copy the Webhook Secret

1. Click on your newly created webhook endpoint
2. Under **Signing secret**, click **Reveal**
3. Copy the secret (starts with `whsec_`)

Add to your `.env` file:

```bash
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

---

## Step 5: Test Locally with Stripe CLI

For local development, use the Stripe CLI to forward webhooks.

### 5.1 Install Stripe CLI

```bash
# macOS
brew install stripe/stripe-cli/stripe

# Windows (scoop)
scoop install stripe

# Linux
# Download from https://stripe.com/docs/stripe-cli
```

### 5.2 Login to Stripe

```bash
stripe login
```

### 5.3 Forward Webhooks to Local Server

```bash
stripe listen --forward-to localhost:8080/billing/webhook
```

This will output a webhook signing secret for local testing:

```
Ready! Your webhook signing secret is whsec_xxxxx (^C to quit)
```

**Important**: Use this local secret in your `.env` for development, not the one from the Dashboard.

### 5.4 Trigger Test Events

In another terminal, you can trigger test events:

```bash
# Test checkout completed
stripe trigger checkout.session.completed

# Test subscription created
stripe trigger customer.subscription.created

# Test payment failed
stripe trigger invoice.payment_failed
```

---

## Step 6: Run Database Migration

Before using billing, run the migration to add the required columns:

```bash
task migrate
```

Or manually:

```bash
psql $DATABASE_URL -f migrations/006_stripe_billing.sql
```

---

## Step 7: Verify Configuration

### Check Environment Variables

Your `.env` should have:

```bash
# Stripe (all required for billing)
STRIPE_SECRET_KEY=sk_test_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
STRIPE_PUBLISHABLE_KEY=pk_test_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
STRIPE_PRICE_ID_PRO=price_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Start the Server

```bash
task run:api
```

Look for this log message:
```
stripe billing configured
```

If Stripe is not configured, you'll see the billing page without payment options (trial only).

---

## Step 8: Test the Integration

### 8.1 Test the Trial Flow

1. Register a new user
2. Go to `/billing`
3. Click **Start 7-Day Free Trial**
4. Verify the user's `subscription_status` is `trialing` in the database
5. Verify Pro features are accessible

### 8.2 Test the Checkout Flow

1. On the billing page, click **Subscribe Now**
2. You should be redirected to Stripe Checkout
3. Use test card: `4242 4242 4242 4242`
   - Any future expiry date
   - Any 3-digit CVC
   - Any billing ZIP
4. Complete checkout
5. You should be redirected to `/billing?success=1`
6. Verify the user's `subscription_tier` is `pro` in the database

### 8.3 Test the Customer Portal

1. With an active subscription, go to `/billing`
2. Click **Open Billing Portal**
3. You should be redirected to the Stripe Customer Portal
4. Verify you can:
   - View subscription details
   - Update payment method
   - Cancel subscription

### 8.4 Test Webhook Handling

With `stripe listen` running:

```bash
# Simulate a subscription cancellation
stripe trigger customer.subscription.deleted

# Check your server logs for:
# "subscription deleted" subscription_id=sub_xxx
```

---

## Production Checklist

Before going live, ensure:

### Stripe Dashboard
- [ ] Switch to **Live mode** in Stripe Dashboard
- [ ] Create the Pro Plan product in live mode
- [ ] Create a new webhook endpoint with your production URL
- [ ] Configure the Customer Portal in live mode

### Environment Variables
- [ ] Replace test keys with live keys (`sk_live_...`, `pk_live_...`)
- [ ] Update webhook secret with live webhook secret
- [ ] Update price ID with live price ID

### Application
- [ ] Database migration applied in production
- [ ] Webhook endpoint accessible (HTTPS required)
- [ ] Error monitoring configured (webhook failures)
- [ ] Logging configured for billing events

### Testing
- [ ] Test checkout flow in production with a real card (refund after)
- [ ] Test webhook delivery in Stripe Dashboard
- [ ] Test Customer Portal access

---

## Troubleshooting

### "Stripe not configured" in logs

Check that `STRIPE_SECRET_KEY` is set and not empty.

### Checkout redirects fail

1. Verify `BASE_URL` is set correctly in your `.env`
2. Check that the URL is accessible from the internet (for redirect)
3. Verify `STRIPE_PRICE_ID_PRO` is correct

### Webhooks not received

1. Check webhook endpoint URL is correct
2. Verify webhook secret matches
3. Check Stripe Dashboard → Webhooks → Recent deliveries for errors
4. Ensure your endpoint returns 200 OK quickly (within 5 seconds)

### "Invalid signature" errors

1. Verify `STRIPE_WEBHOOK_SECRET` matches the webhook endpoint
2. For local development, use the secret from `stripe listen`, not Dashboard
3. Ensure the raw request body is passed to signature verification (not parsed JSON)

### Subscription status not updating

1. Check webhook logs in Stripe Dashboard
2. Verify webhook events are being received (check application logs)
3. Ensure database migration has been applied

---

## Webhook Event Reference

| Event | When It Fires | Action Taken |
|-------|---------------|--------------|
| `checkout.session.completed` | User completes payment | Log only (subscription.created handles the update) |
| `customer.subscription.created` | Subscription starts | Upgrade user to Pro, set limits |
| `customer.subscription.updated` | Status changes (active, past_due) | Update subscription status |
| `customer.subscription.deleted` | Subscription canceled/expired | Downgrade to Free tier |
| `invoice.payment_succeeded` | Successful payment | Log for records |
| `invoice.payment_failed` | Payment fails | Log warning, Stripe will retry |

---

## Test Cards

Use these test card numbers in test mode:

| Card Number | Description |
|-------------|-------------|
| `4242 4242 4242 4242` | Succeeds |
| `4000 0000 0000 0002` | Declines |
| `4000 0000 0000 3220` | Requires 3D Secure |
| `4000 0000 0000 9995` | Insufficient funds |

All test cards accept:
- Any future expiry date
- Any 3-digit CVC
- Any billing postal code

---

## Support

- [Stripe Documentation](https://stripe.com/docs)
- [Stripe API Reference](https://stripe.com/docs/api)
- [Stripe CLI Reference](https://stripe.com/docs/stripe-cli)
- [Testing Guide](https://stripe.com/docs/testing)
