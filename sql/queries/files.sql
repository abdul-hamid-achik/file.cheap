-- name: GetFile :one
SELECT * FROM files 
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListFilesByUser :many
SELECT * FROM files 
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountFilesByUser :one
SELECT COUNT(*) FROM files 
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: CreateFile :one
INSERT INTO files (
    user_id,
    filename,
    content_type,
    size_bytes,
    storage_key,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: UpdateFileStatus :exec
UPDATE files 
SET status = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteFile :exec
UPDATE files
SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetFilesByIDs :many
SELECT * FROM files
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;

-- name: ListFilesByUserWithCount :many
SELECT *, COUNT(*) OVER() AS total_count FROM files
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetUserStorageUsage :one
SELECT COALESCE(SUM(size_bytes), 0)::bigint as total_bytes
FROM files
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: GetUserVideoStorageUsage :one
SELECT COALESCE(SUM(size_bytes), 0)::bigint as total_bytes
FROM files
WHERE user_id = $1
  AND deleted_at IS NULL
  AND content_type LIKE 'video/%';

-- name: ListExpiredSoftDeletedFiles :many
SELECT id, storage_key, user_id
FROM files
WHERE deleted_at IS NOT NULL
  AND deleted_at < NOW() - INTERVAL '7 days'
LIMIT $1;

-- name: ListRetentionExpiredFiles :many
SELECT f.id, f.storage_key, f.user_id
FROM files f
JOIN user_settings us ON us.user_id = f.user_id
WHERE f.deleted_at IS NULL
  AND f.created_at < NOW() - (us.default_retention_days || ' days')::INTERVAL
LIMIT $1;

-- name: HardDeleteFile :exec
DELETE FROM files WHERE id = $1;

-- name: MarkOriginalDeleted :exec
UPDATE files
SET storage_key = '', updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SearchFilesByUser :many
SELECT *, COUNT(*) OVER() AS total_count FROM files
WHERE user_id = $1
  AND deleted_at IS NULL
  AND ($2::text = '' OR filename ILIKE '%' || $2 || '%')
  AND ($3::text = '' OR content_type LIKE $3 || '%')
  AND ($4::timestamptz IS NULL OR created_at >= $4)
  AND ($5::timestamptz IS NULL OR created_at <= $5)
  AND ($6::text = '' OR status = $6::file_status)
ORDER BY created_at DESC
LIMIT $7 OFFSET $8;
