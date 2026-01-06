-- name: CreateAPIToken :one
INSERT INTO api_tokens (user_id, name, token_hash, token_prefix)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAPITokenByHash :one
SELECT t.*, u.id as uid, u.email, u.name as user_name, u.role
FROM api_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1
  AND (t.expires_at IS NULL OR t.expires_at > NOW())
  AND u.deleted_at IS NULL;

-- name: ListAPITokensByUser :many
SELECT * FROM api_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: DeleteAPIToken :exec
DELETE FROM api_tokens
WHERE id = $1 AND user_id = $2;

-- name: UpdateAPITokenLastUsed :exec
UPDATE api_tokens
SET last_used_at = NOW()
WHERE id = $1;
