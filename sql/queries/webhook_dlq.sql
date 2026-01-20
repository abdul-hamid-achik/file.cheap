-- name: CreateWebhookDLQEntry :one
INSERT INTO webhook_dlq (
    webhook_id,
    delivery_id,
    event_type,
    payload,
    final_error,
    attempts,
    last_response_code,
    last_response_body
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetWebhookDLQEntry :one
SELECT * FROM webhook_dlq
WHERE id = $1;

-- name: ListWebhookDLQByWebhook :many
SELECT * FROM webhook_dlq
WHERE webhook_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListWebhookDLQByUser :many
SELECT dlq.*
FROM webhook_dlq dlq
JOIN webhooks w ON w.id = dlq.webhook_id
WHERE w.user_id = $1
ORDER BY dlq.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountWebhookDLQByUser :one
SELECT COUNT(*)
FROM webhook_dlq dlq
JOIN webhooks w ON w.id = dlq.webhook_id
WHERE w.user_id = $1;

-- name: MarkWebhookDLQRetried :exec
UPDATE webhook_dlq
SET can_retry = false, retried_at = NOW()
WHERE id = $1;

-- name: DeleteWebhookDLQEntry :exec
DELETE FROM webhook_dlq
WHERE id = $1;

-- name: ListRetryableWebhookDLQ :many
SELECT dlq.*
FROM webhook_dlq dlq
JOIN webhooks w ON w.id = dlq.webhook_id
WHERE w.user_id = $1 AND dlq.can_retry = true
ORDER BY dlq.created_at DESC
LIMIT $2;

-- name: DeleteOldDLQEntries :exec
DELETE FROM webhook_dlq
WHERE created_at < NOW() - INTERVAL '30 days';
