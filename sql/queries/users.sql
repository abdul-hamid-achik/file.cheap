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

-- name: GetUserRole :one
SELECT role FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserStorageQuota :one
SELECT id, storage_limit_bytes, storage_used_bytes
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserTotalStorageUsage :one
SELECT COALESCE(SUM(size_bytes), 0)::bigint AS total_bytes
FROM files
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: UpdateUserStorageUsed :exec
UPDATE users
SET storage_used_bytes = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: IncrementUserStorageUsed :exec
UPDATE users
SET storage_used_bytes = storage_used_bytes + $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: DecrementUserStorageUsed :exec
UPDATE users
SET storage_used_bytes = GREATEST(0, storage_used_bytes - $2), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserStorageLimit :exec
UPDATE users
SET storage_limit_bytes = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserToEnterprise :one
UPDATE users
SET
    subscription_tier = 'enterprise',
    subscription_status = 'active',
    files_limit = -1,
    max_file_size = 10737418240,
    storage_limit_bytes = 1099511627776,
    transformations_limit = -1,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ListUsersForAdmin :many
SELECT
    id, email, name, avatar_url, role,
    subscription_tier, subscription_status,
    files_limit, storage_used_bytes, storage_limit_bytes,
    transformations_count, transformations_limit,
    created_at
FROM users
WHERE deleted_at IS NULL
    AND ($1::text = '' OR email ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUsersForAdmin :one
SELECT COUNT(*)
FROM users
WHERE deleted_at IS NULL
    AND ($1::text = '' OR email ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%');

-- name: UpdateUserTier :one
UPDATE users
SET
    subscription_tier = $2,
    subscription_status = CASE
        WHEN $2 = 'free' THEN 'none'
        ELSE 'active'
    END,
    files_limit = CASE
        WHEN $2 = 'free' THEN 100
        WHEN $2 = 'pro' THEN 2000
        WHEN $2 = 'enterprise' THEN -1
    END,
    max_file_size = CASE
        WHEN $2 = 'free' THEN 10485760
        WHEN $2 = 'pro' THEN 104857600
        WHEN $2 = 'enterprise' THEN 10737418240
    END,
    storage_limit_bytes = CASE
        WHEN $2 = 'free' THEN 1073741824
        WHEN $2 = 'pro' THEN 107374182400
        WHEN $2 = 'enterprise' THEN 1099511627776
    END,
    transformations_limit = CASE
        WHEN $2 = 'free' THEN 100
        WHEN $2 = 'pro' THEN 10000
        WHEN $2 = 'enterprise' THEN -1
    END,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
