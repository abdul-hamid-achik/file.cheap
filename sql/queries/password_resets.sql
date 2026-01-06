-- name: CreatePasswordReset :one
INSERT INTO password_resets (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordResetByTokenHash :one
SELECT pr.*, u.email, u.name
FROM password_resets pr
JOIN users u ON pr.user_id = u.id
WHERE pr.token_hash = $1 
  AND pr.expires_at > NOW() 
  AND pr.used_at IS NULL
  AND u.deleted_at IS NULL;

-- name: MarkPasswordResetUsed :exec
UPDATE password_resets
SET used_at = NOW()
WHERE id = $1;

-- name: DeleteExpiredPasswordResets :exec
DELETE FROM password_resets
WHERE expires_at < NOW() OR used_at IS NOT NULL;

-- name: DeleteUserPasswordResets :exec
DELETE FROM password_resets WHERE user_id = $1;
