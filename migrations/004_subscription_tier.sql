-- Migration: 004_subscription_tier.sql
-- Description: Add subscription tier to users for premium features

-- Create subscription tier enum
CREATE TYPE subscription_tier AS ENUM ('free', 'pro', 'enterprise');

-- Add subscription_tier column to users table
ALTER TABLE users ADD COLUMN subscription_tier subscription_tier NOT NULL DEFAULT 'free';

-- Index for queries filtering by tier
CREATE INDEX idx_users_subscription_tier ON users(subscription_tier) WHERE deleted_at IS NULL;
