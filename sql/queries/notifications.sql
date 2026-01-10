-- name: CreateNotification :one
INSERT INTO notifications (user_id, type, title, message, link)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetNotification :one
SELECT * FROM notifications WHERE id = $1;

-- name: ListNotificationsByUser :many
SELECT * FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListUnreadNotificationsByUser :many
SELECT * FROM notifications
WHERE user_id = $1 AND read_at IS NULL
ORDER BY created_at DESC
LIMIT $2;

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications
WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications
SET read_at = NOW()
WHERE id = $1 AND user_id = $2;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications
SET read_at = NOW()
WHERE user_id = $1 AND read_at IS NULL;

-- name: DeleteNotification :exec
DELETE FROM notifications WHERE id = $1 AND user_id = $2;

-- name: DeleteOldNotifications :exec
DELETE FROM notifications
WHERE created_at < NOW() - INTERVAL '30 days';

-- name: GetUserOnboardingProgress :one
SELECT
    onboarding_completed_at,
    onboarding_steps
FROM users
WHERE id = $1;

-- name: UpdateOnboardingStep :exec
UPDATE users
SET onboarding_steps = onboarding_steps || $2::jsonb
WHERE id = $1;

-- name: CompleteOnboarding :exec
UPDATE users
SET onboarding_completed_at = NOW()
WHERE id = $1 AND onboarding_completed_at IS NULL;
