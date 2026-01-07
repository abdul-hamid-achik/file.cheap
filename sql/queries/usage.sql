-- name: GetUserTransformationUsage :one
SELECT transformations_count, transformations_limit, transformations_reset_at
FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: IncrementTransformationCount :exec
UPDATE users 
SET transformations_count = transformations_count + 1, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ResetExpiredTransformations :exec
UPDATE users 
SET transformations_count = 0, 
    transformations_reset_at = DATE_TRUNC('month', NOW()) + INTERVAL '1 month',
    updated_at = NOW()
WHERE transformations_reset_at <= NOW() AND deleted_at IS NULL;

-- name: UpdateUserTransformationLimit :exec
UPDATE users
SET transformations_limit = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpsertMonthlyUsage :one
INSERT INTO monthly_usage (user_id, year_month, transformations_count, bytes_processed, files_uploaded)
VALUES ($1, TO_CHAR(NOW(), 'YYYY-MM'), $2, $3, $4)
ON CONFLICT (user_id, year_month) DO UPDATE SET
    transformations_count = monthly_usage.transformations_count + EXCLUDED.transformations_count,
    bytes_processed = monthly_usage.bytes_processed + EXCLUDED.bytes_processed,
    files_uploaded = monthly_usage.files_uploaded + EXCLUDED.files_uploaded,
    updated_at = NOW()
RETURNING *;

-- name: GetCurrentMonthUsage :one
SELECT * FROM monthly_usage 
WHERE user_id = $1 AND year_month = TO_CHAR(NOW(), 'YYYY-MM');

-- name: ListMonthlyUsageHistory :many
SELECT * FROM monthly_usage 
WHERE user_id = $1 
ORDER BY year_month DESC 
LIMIT $2;

-- name: GetUserBillingInfo :one
SELECT 
    subscription_tier,
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
SELECT COUNT(*) FROM files WHERE user_id = $1 AND deleted_at IS NULL;
