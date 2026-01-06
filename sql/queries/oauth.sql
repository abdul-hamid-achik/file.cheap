-- name: CreateOAuthAccount :one
INSERT INTO oauth_accounts (user_id, provider, provider_user_id, access_token, refresh_token, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetOAuthAccount :one
SELECT * FROM oauth_accounts
WHERE provider = $1 AND provider_user_id = $2;

-- name: GetOAuthAccountWithUser :one
SELECT 
    oa.*,
    u.email, u.name, u.avatar_url, u.role, u.email_verified_at, u.created_at as user_created_at
FROM oauth_accounts oa
JOIN users u ON oa.user_id = u.id
WHERE oa.provider = $1 AND oa.provider_user_id = $2 AND u.deleted_at IS NULL;

-- name: UpdateOAuthTokens :exec
UPDATE oauth_accounts
SET access_token = $3, refresh_token = $4, expires_at = $5
WHERE provider = $1 AND provider_user_id = $2;

-- name: ListUserOAuthAccounts :many
SELECT * FROM oauth_accounts
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: DeleteOAuthAccount :exec
DELETE FROM oauth_accounts
WHERE user_id = $1 AND provider = $2;
