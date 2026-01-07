-- name: CreateUser :one
INSERT INTO users (email, password_hash, name, avatar_url, role)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateUser :one
UPDATE users
SET name = $2, avatar_url = $3, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: VerifyUserEmail :exec
UPDATE users
SET email_verified_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: DeleteUser :exec
UPDATE users
SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT * FROM users
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE deleted_at IS NULL;

-- name: UpdateUserSubscriptionTier :one
UPDATE users
SET subscription_tier = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: GetUserSubscriptionTier :one
SELECT subscription_tier FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByStripeCustomerID :one
SELECT * FROM users
WHERE stripe_customer_id = $1 AND deleted_at IS NULL;

-- name: UpdateUserStripeCustomer :one
UPDATE users
SET stripe_customer_id = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserSubscription :one
UPDATE users
SET 
    stripe_subscription_id = $2,
    subscription_status = $3,
    subscription_tier = $4,
    subscription_period_end = $5,
    trial_ends_at = $6,
    files_limit = $7,
    max_file_size = $8,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserSubscriptionStatus :one
UPDATE users
SET 
    subscription_status = $2,
    subscription_period_end = $3,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: CancelUserSubscription :one
UPDATE users
SET 
    stripe_subscription_id = NULL,
    subscription_status = 'canceled',
    subscription_tier = 'free',
    files_limit = 100,
    max_file_size = 10485760,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: StartUserTrial :one
UPDATE users
SET 
    subscription_tier = 'pro',
    subscription_status = 'trialing',
    trial_ends_at = $2,
    files_limit = 2000,
    max_file_size = 104857600,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: GetUserBillingInfo :one
SELECT 
    id,
    email,
    name,
    subscription_tier,
    stripe_customer_id,
    stripe_subscription_id,
    subscription_status,
    subscription_period_end,
    trial_ends_at,
    files_limit,
    max_file_size,
    transformations_count,
    transformations_limit,
    transformations_reset_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserFilesCount :one
SELECT COUNT(*) FROM files
WHERE user_id = $1 AND deleted_at IS NULL;
