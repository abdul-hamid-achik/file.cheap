-- Migration: 006_stripe_billing.sql
-- Description: Add Stripe billing fields for subscription management

-- Subscription status enum
CREATE TYPE subscription_status AS ENUM ('none', 'trialing', 'active', 'past_due', 'canceled', 'unpaid');

-- Add billing columns to users table
ALTER TABLE users ADD COLUMN stripe_customer_id VARCHAR(255);
ALTER TABLE users ADD COLUMN stripe_subscription_id VARCHAR(255);
ALTER TABLE users ADD COLUMN subscription_status subscription_status NOT NULL DEFAULT 'none';
ALTER TABLE users ADD COLUMN subscription_period_end TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN trial_ends_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN files_limit INTEGER NOT NULL DEFAULT 100;
ALTER TABLE users ADD COLUMN max_file_size BIGINT NOT NULL DEFAULT 10485760;

-- Indexes for Stripe lookups
CREATE UNIQUE INDEX idx_users_stripe_customer_id ON users(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;
CREATE INDEX idx_users_stripe_subscription_id ON users(stripe_subscription_id) WHERE stripe_subscription_id IS NOT NULL;
CREATE INDEX idx_users_subscription_status ON users(subscription_status) WHERE deleted_at IS NULL;
