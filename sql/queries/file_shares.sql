-- name: CreateFileShare :one
INSERT INTO file_shares (file_id, token, expires_at, allowed_transforms)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetFileShareByToken :one
SELECT s.*, f.storage_key, f.content_type, f.user_id, f.filename
FROM file_shares s
JOIN files f ON f.id = s.file_id
WHERE s.token = $1
  AND (s.expires_at IS NULL OR s.expires_at > NOW())
  AND f.deleted_at IS NULL;

-- name: IncrementShareAccessCount :exec
UPDATE file_shares
SET access_count = access_count + 1
WHERE id = $1;

-- name: ListFileSharesByFile :many
SELECT * FROM file_shares
WHERE file_id = $1
ORDER BY created_at DESC;

-- name: DeleteFileShare :exec
DELETE FROM file_shares
WHERE file_shares.id = $1 AND file_id IN (SELECT files.id FROM files WHERE files.user_id = $2);

-- name: DeleteExpiredShares :exec
DELETE FROM file_shares
WHERE expires_at IS NOT NULL AND expires_at < NOW();

-- name: CreateTransformCache :one
INSERT INTO transform_cache (file_id, cache_key, transform_params, storage_key, content_type, size_bytes, width, height)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (file_id, cache_key) DO UPDATE SET
    request_count = transform_cache.request_count + 1,
    last_accessed_at = NOW()
RETURNING *;

-- name: GetTransformCache :one
SELECT * FROM transform_cache
WHERE file_id = $1 AND cache_key = $2;

-- name: IncrementTransformCacheCount :exec
UPDATE transform_cache
SET request_count = request_count + 1, last_accessed_at = NOW()
WHERE file_id = $1 AND cache_key = $2;

-- name: GetTransformRequestCount :one
SELECT COALESCE(
    (SELECT request_count FROM transform_cache WHERE file_id = $1 AND cache_key = $2),
    0
)::int AS count;

-- name: DeleteOldTransformCache :exec
DELETE FROM transform_cache
WHERE last_accessed_at < NOW() - INTERVAL '30 days'
  AND request_count < 10;
