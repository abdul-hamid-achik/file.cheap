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

-- name: GetChurnMetrics :one
WITH monthly_stats AS (
    SELECT
        COUNT(*) FILTER (
            WHERE subscription_status = 'canceled'
            AND updated_at >= DATE_TRUNC('month', NOW())
        )::bigint as churned_this_month,
        COUNT(*) FILTER (
            WHERE subscription_status IN ('active', 'trialing')
            AND created_at < DATE_TRUNC('month', NOW())
        )::bigint as active_start_of_month,
        COUNT(*) FILTER (
            WHERE subscription_status IN ('active', 'trialing')
        )::bigint as current_active,
        COUNT(*) FILTER (
            WHERE subscription_status = 'canceled'
            AND updated_at >= NOW() - INTERVAL '30 days'
        )::bigint as churned_30d
    FROM users
    WHERE deleted_at IS NULL
)
SELECT
    churned_this_month,
    churned_30d,
    current_active,
    CASE
        WHEN active_start_of_month > 0
        THEN (churned_this_month::float8 / active_start_of_month::float8 * 100)
        ELSE 0
    END as monthly_churn_rate,
    CASE
        WHEN active_start_of_month > 0
        THEN ((active_start_of_month - churned_this_month)::float8 / active_start_of_month::float8 * 100)
        ELSE 100
    END as retention_rate
FROM monthly_stats;

-- name: GetRevenueMetrics :one
WITH revenue_stats AS (
    SELECT
        COUNT(*) FILTER (WHERE subscription_status IN ('active', 'trialing'))::bigint as paying_users,
        COALESCE(SUM(
            CASE subscription_tier
                WHEN 'pro' THEN 19.00
                WHEN 'enterprise' THEN 99.00
                ELSE 0
            END
        ) FILTER (WHERE subscription_status IN ('active', 'trialing')), 0)::float8 as mrr
    FROM users
    WHERE deleted_at IS NULL
),
churn_stats AS (
    SELECT
        COUNT(*) FILTER (
            WHERE subscription_status = 'canceled'
            AND updated_at >= DATE_TRUNC('month', NOW())
        )::bigint as churned,
        COUNT(*) FILTER (
            WHERE subscription_status IN ('active', 'trialing')
            AND created_at < DATE_TRUNC('month', NOW())
        )::bigint as start_count
    FROM users
    WHERE deleted_at IS NULL
)
SELECT
    r.mrr,
    CASE
        WHEN r.paying_users > 0
        THEN r.mrr / r.paying_users::float8
        ELSE 0
    END as arpu,
    CASE
        WHEN c.start_count > 0 AND c.churned > 0
        THEN (r.mrr / r.paying_users::float8) / (c.churned::float8 / c.start_count::float8)
        ELSE r.mrr * 24
    END as estimated_ltv,
    r.paying_users,
    CASE r.paying_users
        WHEN 0 THEN 0
        ELSE (r.mrr * 12)::float8
    END as arr
FROM revenue_stats r, churn_stats c;

-- name: GetCohortRetention :many
WITH cohorts AS (
    SELECT
        id,
        DATE_TRUNC('month', created_at)::date as cohort_month,
        created_at,
        CASE
            WHEN subscription_status IN ('active', 'trialing') THEN true
            ELSE false
        END as is_active
    FROM users
    WHERE deleted_at IS NULL
        AND created_at >= NOW() - INTERVAL '6 months'
),
cohort_sizes AS (
    SELECT
        cohort_month,
        COUNT(*) as cohort_size
    FROM cohorts
    GROUP BY cohort_month
),
retention AS (
    SELECT
        c.cohort_month,
        EXTRACT(MONTH FROM AGE(DATE_TRUNC('month', NOW()), c.cohort_month))::int as months_since,
        COUNT(*) FILTER (WHERE c.is_active) as retained
    FROM cohorts c
    GROUP BY c.cohort_month, months_since
)
SELECT
    r.cohort_month,
    cs.cohort_size::bigint,
    r.months_since::int,
    r.retained::bigint,
    CASE
        WHEN cs.cohort_size > 0
        THEN (r.retained::float8 / cs.cohort_size::float8 * 100)
        ELSE 0
    END as retention_pct
FROM retention r
JOIN cohort_sizes cs ON cs.cohort_month = r.cohort_month
ORDER BY r.cohort_month DESC, r.months_since;

-- name: GetNRR :one
WITH previous_month AS (
    SELECT COALESCE(SUM(
        CASE subscription_tier
            WHEN 'pro' THEN 19.00
            WHEN 'enterprise' THEN 99.00
            ELSE 0
        END
    ), 0)::float8 as mrr
    FROM users
    WHERE deleted_at IS NULL
        AND subscription_status IN ('active', 'trialing')
        AND created_at < DATE_TRUNC('month', NOW())
),
current_month AS (
    SELECT COALESCE(SUM(
        CASE subscription_tier
            WHEN 'pro' THEN 19.00
            WHEN 'enterprise' THEN 99.00
            ELSE 0
        END
    ), 0)::float8 as mrr
    FROM users
    WHERE deleted_at IS NULL
        AND subscription_status IN ('active', 'trialing')
)
SELECT
    p.mrr as previous_mrr,
    c.mrr as current_mrr,
    CASE
        WHEN p.mrr > 0
        THEN (c.mrr / p.mrr * 100)
        ELSE 100
    END as nrr_percent
FROM previous_month p, current_month c;

-- name: GetAlertConfig :many
SELECT * FROM admin_alert_config ORDER BY metric_name;

-- name: UpdateAlertThreshold :exec
UPDATE admin_alert_config
SET threshold_value = $2, enabled = $3, updated_at = NOW()
WHERE metric_name = $1;
