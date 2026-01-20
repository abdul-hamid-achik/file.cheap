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

-- ============================================================================
-- ENHANCED ANALYTICS - Processing Volume & Trends
-- ============================================================================

-- name: GetProcessingVolumeByTypeOverTime :many
-- Get processing volume by job type over the last N days
SELECT
    dates.date::date as date,
    pj.job_type::text as job_type,
    COUNT(pj.id)::bigint as count,
    COALESCE(SUM(EXTRACT(EPOCH FROM (pj.completed_at - pj.started_at)))::bigint, 0) as total_duration_seconds
FROM (
    SELECT generate_series($1::date, CURRENT_DATE, '1 day'::interval)::date as date
) dates
LEFT JOIN processing_jobs pj ON DATE(pj.created_at) = dates.date
    AND pj.status = 'completed'
GROUP BY dates.date, pj.job_type
ORDER BY dates.date, pj.job_type;

-- name: GetStorageGrowthTrend :many
-- Get storage growth over time (cumulative by day)
SELECT
    dates.date::date as date,
    COALESCE(SUM(daily_added.bytes_added) OVER (ORDER BY dates.date), 0)::bigint as cumulative_bytes,
    COALESCE(daily_added.bytes_added, 0)::bigint as bytes_added,
    COALESCE(daily_added.files_added, 0)::bigint as files_added
FROM (
    SELECT generate_series($1::date, CURRENT_DATE, '1 day'::interval)::date as date
) dates
LEFT JOIN (
    SELECT
        DATE(created_at) as day,
        SUM(size_bytes) as bytes_added,
        COUNT(*) as files_added
    FROM files
    WHERE created_at >= $1 AND deleted_at IS NULL
    GROUP BY DATE(created_at)
) daily_added ON dates.date = daily_added.day
ORDER BY dates.date;

-- name: GetVideoProcessingStats :one
-- Get video processing statistics for cost forecasting
SELECT
    COUNT(DISTINCT f.id)::bigint as total_video_files,
    COALESCE(SUM(f.size_bytes), 0)::bigint as total_video_bytes,
    COUNT(pj.id) FILTER (WHERE pj.job_type IN ('video_transcode', 'video_hls', 'video_thumbnail', 'video_watermark'))::bigint as total_video_jobs,
    COUNT(pj.id) FILTER (WHERE pj.job_type = 'video_transcode')::bigint as transcode_jobs,
    COUNT(pj.id) FILTER (WHERE pj.job_type = 'video_hls')::bigint as hls_jobs,
    COUNT(pj.id) FILTER (WHERE pj.job_type = 'video_thumbnail')::bigint as video_thumbnail_jobs,
    COALESCE(AVG(EXTRACT(EPOCH FROM (pj.completed_at - pj.started_at))) FILTER (WHERE pj.job_type = 'video_transcode'), 0)::float as avg_transcode_duration_seconds,
    COALESCE(SUM(EXTRACT(EPOCH FROM (pj.completed_at - pj.started_at))) FILTER (WHERE pj.job_type IN ('video_transcode', 'video_hls')), 0)::bigint as total_video_processing_seconds
FROM files f
LEFT JOIN processing_jobs pj ON pj.file_id = f.id AND pj.status = 'completed'
WHERE f.content_type LIKE 'video/%' AND f.deleted_at IS NULL;

-- name: GetVideoProcessingStatsByUser :one
-- Get video processing statistics for a specific user
SELECT
    COUNT(DISTINCT f.id)::bigint as total_video_files,
    COALESCE(SUM(f.size_bytes), 0)::bigint as total_video_bytes,
    COUNT(pj.id) FILTER (WHERE pj.job_type IN ('video_transcode', 'video_hls', 'video_thumbnail', 'video_watermark'))::bigint as total_video_jobs,
    COUNT(pj.id) FILTER (WHERE pj.job_type = 'video_transcode')::bigint as transcode_jobs,
    COUNT(pj.id) FILTER (WHERE pj.job_type = 'video_hls')::bigint as hls_jobs,
    COALESCE(SUM(EXTRACT(EPOCH FROM (pj.completed_at - pj.started_at))) FILTER (WHERE pj.job_type IN ('video_transcode', 'video_hls')), 0)::bigint as total_video_processing_seconds
FROM files f
LEFT JOIN processing_jobs pj ON pj.file_id = f.id AND pj.status = 'completed'
WHERE f.user_id = $1 AND f.content_type LIKE 'video/%' AND f.deleted_at IS NULL;

-- name: GetProcessingVolumeByTier :many
-- Get processing volume grouped by subscription tier
SELECT
    u.subscription_tier::text as tier,
    pj.job_type::text as job_type,
    COUNT(pj.id)::bigint as count,
    COALESCE(SUM(EXTRACT(EPOCH FROM (pj.completed_at - pj.started_at)))::bigint, 0) as total_duration_seconds
FROM processing_jobs pj
JOIN files f ON f.id = pj.file_id
JOIN users u ON u.id = f.user_id
WHERE pj.created_at >= $1 AND pj.status = 'completed' AND f.deleted_at IS NULL
GROUP BY u.subscription_tier, pj.job_type
ORDER BY tier, count DESC;

-- name: GetBandwidthStats :one
-- Get CDN/download bandwidth statistics (approximation based on share downloads)
SELECT
    COALESCE(SUM(fs.download_count), 0)::bigint as total_downloads,
    COALESCE(SUM(fs.download_count * f.size_bytes), 0)::bigint as estimated_bandwidth_bytes
FROM file_shares fs
JOIN files f ON f.id = fs.file_id
WHERE fs.created_at >= $1 AND f.deleted_at IS NULL;

-- name: GetBandwidthStatsByUser :one
-- Get bandwidth statistics for a specific user
SELECT
    COALESCE(SUM(fs.download_count), 0)::bigint as total_downloads,
    COALESCE(SUM(fs.download_count * f.size_bytes), 0)::bigint as estimated_bandwidth_bytes
FROM file_shares fs
JOIN files f ON f.id = fs.file_id
WHERE f.user_id = $1 AND fs.created_at >= $2 AND f.deleted_at IS NULL;

-- name: GetCostForecast :one
-- Get data for cost forecasting (storage, processing, bandwidth)
SELECT
    -- Storage costs
    COALESCE(SUM(f.size_bytes), 0)::bigint as total_storage_bytes,
    COUNT(DISTINCT f.id)::bigint as total_files,

    -- Processing costs (last 30 days extrapolated)
    (SELECT COUNT(*) FROM processing_jobs WHERE created_at >= NOW() - INTERVAL '30 days' AND status = 'completed')::bigint as jobs_last_30_days,
    (SELECT COUNT(*) FROM processing_jobs WHERE created_at >= NOW() - INTERVAL '30 days' AND status = 'completed' AND job_type IN ('video_transcode', 'video_hls'))::bigint as video_jobs_last_30_days,
    (SELECT COALESCE(SUM(EXTRACT(EPOCH FROM (completed_at - started_at))), 0) FROM processing_jobs WHERE created_at >= NOW() - INTERVAL '30 days' AND status = 'completed' AND job_type IN ('video_transcode', 'video_hls'))::bigint as video_processing_seconds_30_days,

    -- Bandwidth (share downloads)
    (SELECT COALESCE(SUM(download_count), 0) FROM file_shares WHERE created_at >= NOW() - INTERVAL '30 days')::bigint as downloads_30_days
FROM files f
WHERE f.deleted_at IS NULL;
