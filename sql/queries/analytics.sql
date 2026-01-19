-- name: GetUserUsageStats :one
SELECT 
    COALESCE(COUNT(DISTINCT f.id) FILTER (WHERE f.deleted_at IS NULL), 0)::bigint as files_used,
    u.files_limit as files_limit,
    COALESCE(u.transformations_count, 0) as transforms_used,
    u.transformations_limit as transforms_limit,
    COALESCE(SUM(f.size_bytes) FILTER (WHERE f.deleted_at IS NULL), 0)::bigint as storage_used_bytes,
    u.subscription_tier as plan_name,
    COALESCE(u.subscription_period_end, u.transformations_reset_at) as plan_renews_at
FROM users u
LEFT JOIN files f ON f.user_id = u.id
WHERE u.id = $1 AND u.deleted_at IS NULL
GROUP BY u.id;

-- name: GetDailyUsage :many
SELECT 
    dates.date::date as date,
    COALESCE(file_counts.uploads, 0)::bigint as uploads,
    COALESCE(job_counts.transforms, 0)::bigint as transforms
FROM (
    SELECT generate_series($2::date, CURRENT_DATE, '1 day'::interval)::date as date
) dates
LEFT JOIN (
    SELECT DATE(f2.created_at) as day, COUNT(*) as uploads
    FROM files f2
    WHERE f2.user_id = $1 AND f2.created_at >= $2 AND f2.deleted_at IS NULL
    GROUP BY DATE(f2.created_at)
) file_counts ON dates.date = file_counts.day
LEFT JOIN (
    SELECT DATE(pj.created_at) as day, COUNT(*) as transforms
    FROM processing_jobs pj
    JOIN files f3 ON f3.id = pj.file_id
    WHERE f3.user_id = $1 AND pj.created_at >= $2 AND f3.deleted_at IS NULL
    GROUP BY DATE(pj.created_at)
) job_counts ON dates.date = job_counts.day
ORDER BY dates.date;

-- name: GetTransformBreakdown :many
SELECT 
    pj.job_type::text as type,
    COUNT(*)::bigint as count
FROM processing_jobs pj
JOIN files f ON f.id = pj.file_id
WHERE f.user_id = $1 
    AND pj.created_at >= DATE_TRUNC('month', NOW())
    AND pj.status = 'completed'
    AND f.deleted_at IS NULL
GROUP BY pj.job_type
ORDER BY count DESC;

-- name: GetTopFilesByTransforms :many
SELECT 
    f.id as file_id,
    f.filename,
    COUNT(pj.id)::bigint as transforms
FROM files f
JOIN processing_jobs pj ON pj.file_id = f.id
WHERE f.user_id = $1 
    AND pj.created_at >= DATE_TRUNC('month', NOW())
    AND f.deleted_at IS NULL
GROUP BY f.id, f.filename
ORDER BY transforms DESC
LIMIT 10;

-- name: GetRecentActivity :many
SELECT * FROM (
    SELECT 
        f.id::text as id,
        'upload' as type,
        f.filename || ' uploaded' as message,
        'success' as status,
        f.created_at as created_at
    FROM files f
    WHERE f.user_id = $1 AND f.deleted_at IS NULL
    
    UNION ALL
    
    SELECT 
        pj.id::text as id,
        'transform' as type,
        f.filename || ' ' || pj.job_type::text || 
            CASE pj.status 
                WHEN 'completed' THEN ' completed'
                WHEN 'failed' THEN ' failed'
                WHEN 'running' THEN ' processing'
                ELSE ' queued'
            END as message,
        CASE pj.status
            WHEN 'completed' THEN 'success'
            WHEN 'failed' THEN 'error'
            ELSE 'warning'
        END as status,
        COALESCE(pj.completed_at, pj.created_at) as created_at
    FROM processing_jobs pj
    JOIN files f ON f.id = pj.file_id
    WHERE f.user_id = $1 AND f.deleted_at IS NULL
    
    UNION ALL
    
    SELECT 
        b.id::text as id,
        'batch' as type,
        'Batch ' || LEFT(b.id::text, 8) || 
            CASE b.status
                WHEN 'completed' THEN ' completed (' || b.completed_files || ' files)'
                WHEN 'failed' THEN ' failed'
                WHEN 'partial' THEN ' partially completed'
                ELSE ' processing'
            END as message,
        CASE b.status
            WHEN 'completed' THEN 'success'
            WHEN 'failed' THEN 'error'
            ELSE 'warning'
        END as status,
        COALESCE(b.completed_at, b.created_at) as created_at
    FROM batch_operations b
    WHERE b.user_id = $1
) activity
ORDER BY created_at DESC
LIMIT 20;

-- name: GetTotalStorageByUser :one
SELECT COALESCE(SUM(size_bytes), 0)::bigint as total_bytes
FROM files
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: GetStorageBreakdownByType :many
SELECT
    CASE
        WHEN content_type LIKE 'image/%' THEN 'image'
        WHEN content_type LIKE 'video/%' THEN 'video'
        WHEN content_type LIKE 'audio/%' THEN 'audio'
        WHEN content_type = 'application/pdf' THEN 'pdf'
        ELSE 'other'
    END as file_type,
    COUNT(*) as file_count,
    COALESCE(SUM(size_bytes), 0)::bigint as total_bytes
FROM files
WHERE user_id = $1 AND deleted_at IS NULL
GROUP BY 1
ORDER BY total_bytes DESC;

-- name: GetStorageBreakdownByVariant :many
SELECT
    variant_type::text,
    COUNT(*) as variant_count,
    COALESCE(SUM(size_bytes), 0)::bigint as total_bytes
FROM file_variants fv
JOIN files f ON f.id = fv.file_id
WHERE f.user_id = $1 AND f.deleted_at IS NULL
GROUP BY 1
ORDER BY total_bytes DESC;

-- name: GetLargestFiles :many
SELECT id, filename, content_type, size_bytes, created_at
FROM files
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY size_bytes DESC
LIMIT $2;

-- name: GetAdminStorageBreakdown :many
SELECT
    CASE
        WHEN content_type LIKE 'image/%' THEN 'image'
        WHEN content_type LIKE 'video/%' THEN 'video'
        WHEN content_type LIKE 'audio/%' THEN 'audio'
        WHEN content_type = 'application/pdf' THEN 'pdf'
        ELSE 'other'
    END as file_type,
    COUNT(*) as file_count,
    COALESCE(SUM(size_bytes), 0)::bigint as total_bytes
FROM files
WHERE deleted_at IS NULL
GROUP BY 1
ORDER BY total_bytes DESC;
