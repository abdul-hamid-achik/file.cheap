-- name: GetUserSettings :one
SELECT * FROM user_settings
WHERE user_id = $1;

-- name: CreateUserSettings :one
INSERT INTO user_settings (user_id)
VALUES ($1)
RETURNING *;

-- name: UpsertUserSettings :one
INSERT INTO user_settings (user_id)
VALUES ($1)
ON CONFLICT (user_id) DO UPDATE SET updated_at = NOW()
RETURNING *;

-- name: UpdateNotificationSettings :one
UPDATE user_settings
SET email_notifications = $2,
    processing_alerts = $3,
    marketing_emails = $4,
    updated_at = NOW()
WHERE user_id = $1
RETURNING *;

-- name: UpdateFileSettings :one
UPDATE user_settings
SET default_retention_days = $2,
    auto_delete_originals = $3,
    updated_at = NOW()
WHERE user_id = $1
RETURNING *;
