-- name: CreateWebhook :one
INSERT INTO webhooks (user_id, url, secret, events)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetWebhook :one
SELECT * FROM webhooks
WHERE id = $1 AND user_id = $2;

-- name: GetWebhookByID :one
SELECT * FROM webhooks
WHERE id = $1;

-- name: ListWebhooksByUser :many
SELECT * FROM webhooks
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountWebhooksByUser :one
SELECT COUNT(*) FROM webhooks
WHERE user_id = $1;

-- name: UpdateWebhook :one
UPDATE webhooks
SET url = $3, events = $4, active = $5, updated_at = NOW()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteWebhook :exec
DELETE FROM webhooks
WHERE id = $1 AND user_id = $2;

-- name: ListActiveWebhooksByUserAndEvent :many
SELECT * FROM webhooks
WHERE user_id = @user_id AND active = true AND @event_type::text = ANY(events);

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (webhook_id, event_type, payload)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetWebhookDelivery :one
SELECT * FROM webhook_deliveries
WHERE id = $1;

-- name: GetWebhookByDeliveryID :one
SELECT w.* FROM webhooks w
JOIN webhook_deliveries wd ON wd.webhook_id = w.id
WHERE wd.id = $1;

-- name: ListDeliveriesByWebhook :many
SELECT * FROM webhook_deliveries
WHERE webhook_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDeliveriesByWebhook :one
SELECT COUNT(*) FROM webhook_deliveries
WHERE webhook_id = $1;

-- name: MarkDeliverySuccess :exec
UPDATE webhook_deliveries
SET status = 'success', attempts = attempts + 1, last_attempt_at = NOW(),
    response_code = $2, response_body = $3
WHERE id = $1;

-- name: MarkDeliveryFailed :exec
UPDATE webhook_deliveries
SET status = 'failed', attempts = attempts + 1, last_attempt_at = NOW(),
    response_code = $2, response_body = $3
WHERE id = $1;

-- name: UpdateDeliveryRetry :exec
UPDATE webhook_deliveries
SET status = 'retrying', attempts = attempts + 1, last_attempt_at = NOW(),
    next_retry_at = $2, response_code = $3, response_body = $4
WHERE id = $1;

-- name: ListPendingDeliveries :many
SELECT * FROM webhook_deliveries
WHERE status IN ('pending', 'retrying')
  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
ORDER BY created_at ASC
LIMIT $1;

-- name: IncrementWebhookFailures :exec
UPDATE webhooks
SET consecutive_failures = consecutive_failures + 1,
    last_failure_at = NOW(),
    circuit_state = CASE WHEN consecutive_failures >= 9 THEN 'open' ELSE circuit_state END
WHERE id = $1;

-- name: ResetWebhookFailures :exec
UPDATE webhooks
SET consecutive_failures = 0, circuit_state = 'closed'
WHERE id = $1;

-- name: GetWebhookCircuitState :one
SELECT id, url, consecutive_failures, last_failure_at, circuit_state
FROM webhooks
WHERE id = $1;

-- name: ListOpenCircuitWebhooks :many
SELECT id, url, consecutive_failures, last_failure_at, circuit_state
FROM webhooks
WHERE circuit_state = 'open'
  AND last_failure_at < NOW() - INTERVAL '1 hour';

-- name: SetWebhookCircuitState :exec
UPDATE webhooks
SET circuit_state = $2
WHERE id = $1;
