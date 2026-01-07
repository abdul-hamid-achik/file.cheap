-- name: CreateBatchOperation :one
INSERT INTO batch_operations (user_id, total_files, presets, webp, quality, watermark)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBatchOperation :one
SELECT * FROM batch_operations WHERE id = $1;

-- name: GetBatchOperationByUser :one
SELECT * FROM batch_operations WHERE id = $1 AND user_id = $2;

-- name: ListBatchOperationsByUser :many
SELECT * FROM batch_operations WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: UpdateBatchOperationStatus :exec
UPDATE batch_operations 
SET status = $2, started_at = COALESCE(started_at, NOW()), updated_at = NOW()
WHERE id = $1;

-- name: UpdateBatchOperationCompleted :exec
UPDATE batch_operations 
SET status = $2, completed_at = NOW(), error_message = $3
WHERE id = $1;

-- name: IncrementBatchCompletedFiles :exec
UPDATE batch_operations 
SET completed_files = completed_files + 1
WHERE id = $1;

-- name: IncrementBatchFailedFiles :exec
UPDATE batch_operations 
SET failed_files = failed_files + 1
WHERE id = $1;

-- name: CreateBatchItem :one
INSERT INTO batch_items (batch_id, file_id, job_ids)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetBatchItem :one
SELECT * FROM batch_items WHERE id = $1;

-- name: ListBatchItems :many
SELECT * FROM batch_items WHERE batch_id = $1 ORDER BY created_at;

-- name: UpdateBatchItemStatus :exec
UPDATE batch_items 
SET status = $2, error_message = $3, completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
WHERE id = $1;

-- name: CountBatchItemsByStatus :one
SELECT 
    COUNT(*) FILTER (WHERE status = 'pending') AS pending,
    COUNT(*) FILTER (WHERE status = 'processing') AS processing,
    COUNT(*) FILTER (WHERE status = 'completed') AS completed,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed
FROM batch_items WHERE batch_id = $1;
