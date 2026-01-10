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
