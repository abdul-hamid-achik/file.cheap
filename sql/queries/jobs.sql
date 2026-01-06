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
