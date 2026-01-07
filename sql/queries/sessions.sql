-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, user_agent, ip_address, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT s.*, u.email, u.name, u.avatar_url, u.role, u.subscription_tier, u.email_verified_at
FROM sessions s
JOIN users u ON s.user_id = u.id
WHERE s.token_hash = $1 
  AND s.expires_at > NOW()
  AND u.deleted_at IS NULL;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < NOW();

-- name: ListUserSessions :many
SELECT * FROM sessions
WHERE user_id = $1
ORDER BY created_at DESC;
