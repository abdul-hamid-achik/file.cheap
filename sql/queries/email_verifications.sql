-- name: CreateEmailVerification :one
INSERT INTO email_verifications (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetEmailVerificationByTokenHash :one
SELECT ev.*, u.email, u.name
FROM email_verifications ev
JOIN users u ON ev.user_id = u.id
WHERE ev.token_hash = $1 
  AND ev.expires_at > NOW() 
  AND ev.verified_at IS NULL
  AND u.deleted_at IS NULL;

-- name: MarkEmailVerified :exec
UPDATE email_verifications
SET verified_at = NOW()
WHERE id = $1;

-- name: DeleteExpiredEmailVerifications :exec
DELETE FROM email_verifications
WHERE expires_at < NOW() OR verified_at IS NOT NULL;

-- name: DeleteUserEmailVerifications :exec
DELETE FROM email_verifications WHERE user_id = $1;
