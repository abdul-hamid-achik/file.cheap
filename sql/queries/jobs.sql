-- name: GetJob :one
SELECT * FROM processing_jobs
WHERE id = $1;

-- name: ListPendingJobs :many
SELECT * FROM processing_jobs
WHERE status = 'pending'
ORDER BY priority DESC, created_at ASC
LIMIT $1;

-- name: CreateJob :one
INSERT INTO processing_jobs (
    file_id,
    job_type,
    priority
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: MarkJobRunning :exec
UPDATE processing_jobs
SET status = 'running',
    started_at = NOW(),
    attempts = attempts + 1
WHERE id = $1;

-- name: MarkJobCompleted :exec
UPDATE processing_jobs
SET status = 'completed',
    completed_at = NOW()
WHERE id = $1;

-- name: MarkJobFailed :exec
UPDATE processing_jobs
SET status = 'failed',
    completed_at = NOW(),
    error_message = $2
WHERE id = $1;

-- name: ListJobsByFileID :many
SELECT * FROM processing_jobs
WHERE file_id = $1
ORDER BY created_at DESC;

-- name: DeleteJobsByFileID :exec
DELETE FROM processing_jobs
WHERE file_id = $1;

-- name: RetryJob :exec
UPDATE processing_jobs
SET status = 'pending',
    error_message = NULL,
    started_at = NULL,
    completed_at = NULL
WHERE id = $1 AND status = 'failed';

-- name: CancelJob :exec
UPDATE processing_jobs
SET status = 'failed',
    error_message = 'Cancelled by user',
    completed_at = NOW()
WHERE id = $1 AND status IN ('pending', 'running');

-- name: BulkRetryFailedJobs :exec
UPDATE processing_jobs
SET status = 'pending',
    error_message = NULL,
    started_at = NULL,
    completed_at = NULL
WHERE file_id IN (
    SELECT id FROM files WHERE user_id = $1 AND deleted_at IS NULL
) AND status = 'failed';

-- name: ListJobsByUserWithStatus :many
SELECT pj.*, f.filename, f.content_type
FROM processing_jobs pj
JOIN files f ON f.id = pj.file_id
WHERE f.user_id = $1
  AND f.deleted_at IS NULL
  AND ($2::job_status IS NULL OR pj.status = $2)
ORDER BY pj.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountJobsByUser :one
SELECT COUNT(*)
FROM processing_jobs pj
JOIN files f ON f.id = pj.file_id
WHERE f.user_id = $1
  AND f.deleted_at IS NULL
  AND ($2::job_status IS NULL OR pj.status = $2);

-- name: GetJobByUser :one
SELECT pj.*, f.filename, f.content_type, f.user_id
FROM processing_jobs pj
JOIN files f ON f.id = pj.file_id
WHERE pj.id = $1
  AND f.user_id = $2
  AND f.deleted_at IS NULL;
