-- name: GetAdminMetrics :one
SELECT
    COALESCE(
        SUM(CASE subscription_tier 
            WHEN 'pro' THEN 19.00 
            WHEN 'enterprise' THEN 99.00 
            ELSE 0 
        END), 0
    )::float8 as mrr,
    COUNT(*)::bigint as total_users,
    COUNT(*) FILTER (
        WHERE created_at >= NOW() - INTERVAL '7 days'
    )::bigint as new_users_this_week
FROM users
WHERE deleted_at IS NULL 
    AND (subscription_status IN ('active', 'trialing') OR subscription_tier = 'free');

-- name: GetMRRHistory :many
SELECT 
    DATE(created_at) as date,
    SUM(CASE subscription_tier 
        WHEN 'pro' THEN 19.00 
        WHEN 'enterprise' THEN 99.00 
        ELSE 0 
    END)::float8 as mrr,
    COUNT(*)::bigint as users
FROM users
WHERE created_at >= $1
    AND deleted_at IS NULL
    AND (subscription_status IN ('active', 'trialing') OR subscription_tier = 'free')
GROUP BY DATE(created_at)
ORDER BY date;

-- name: GetUsersByPlan :many
SELECT 
    subscription_tier::text as plan,
    COUNT(*)::bigint as count
FROM users
WHERE deleted_at IS NULL
GROUP BY subscription_tier
ORDER BY count DESC;

-- name: GetTotalFilesAndStorage :one
SELECT 
    COUNT(*)::bigint as total_files,
    COALESCE(SUM(size_bytes), 0)::bigint as total_storage_bytes
FROM files
WHERE deleted_at IS NULL;

-- name: GetJobStats24h :one
SELECT 
    COUNT(*)::bigint as total_jobs,
    COUNT(*) FILTER (WHERE status = 'completed')::bigint as completed,
    COUNT(*) FILTER (WHERE status = 'failed')::bigint as failed
FROM processing_jobs
WHERE created_at >= NOW() - INTERVAL '24 hours';

-- name: GetTopUsersByUsage :many
SELECT 
    u.email,
    u.subscription_tier::text as plan,
    COUNT(pj.id)::bigint as transforms,
    CASE u.subscription_tier 
        WHEN 'pro' THEN 19.00 
        WHEN 'enterprise' THEN 99.00 
        ELSE 0 
    END::float8 as monthly_rate
FROM users u
JOIN files f ON f.user_id = u.id AND f.deleted_at IS NULL
JOIN processing_jobs pj ON pj.file_id = f.id
WHERE pj.created_at >= DATE_TRUNC('month', NOW())
    AND u.deleted_at IS NULL
GROUP BY u.id, u.email, u.subscription_tier
ORDER BY transforms DESC
LIMIT 10;

-- name: GetRecentSignups :many
SELECT 
    email,
    subscription_tier::text as plan,
    created_at
FROM users
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 10;

-- name: GetFailedJobs24h :many
SELECT 
    pj.id,
    pj.file_id,
    pj.job_type::text as job_type,
    COALESCE(pj.error_message, 'Unknown error') as error,
    COALESCE(pj.completed_at, pj.created_at) as failed_at,
    (pj.attempts < 3) as can_retry
FROM processing_jobs pj
WHERE pj.status = 'failed'
    AND pj.created_at >= NOW() - INTERVAL '24 hours'
ORDER BY failed_at DESC
LIMIT 20;

-- name: GetWorkerQueueSize :one
SELECT COUNT(*)::bigint as size
FROM processing_jobs
WHERE status IN ('pending', 'running');

-- name: GetFailedJobsLastHour :one
SELECT COUNT(*)::bigint as count
FROM processing_jobs
WHERE status = 'failed'
    AND created_at >= NOW() - INTERVAL '1 hour';

-- name: RetryFailedJob :exec
UPDATE processing_jobs
SET status = 'pending', error_message = NULL, attempts = 0
WHERE id = $1 AND status = 'failed';

-- name: ListJobsAdmin :many
SELECT 
    pj.id,
    pj.file_id,
    pj.job_type::text as job_type,
    pj.status::text as status,
    pj.priority,
    pj.attempts,
    COALESCE(pj.error_message, '') as error_message,
    pj.created_at,
    pj.started_at,
    pj.completed_at,
    f.filename
FROM processing_jobs pj
LEFT JOIN files f ON f.id = pj.file_id
WHERE (sqlc.narg('status')::text IS NULL OR pj.status::text = sqlc.narg('status'))
ORDER BY pj.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountJobsAdmin :one
SELECT COUNT(*)::bigint as total
FROM processing_jobs
WHERE (sqlc.narg('status')::text IS NULL OR status::text = sqlc.narg('status'));
