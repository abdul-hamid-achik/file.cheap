-- name: ListVariantsByFile :many
SELECT * FROM file_variants
WHERE file_id = $1
ORDER BY created_at DESC;

-- name: GetVariant :one
SELECT * FROM file_variants
WHERE file_id = $1 AND variant_type = $2;

-- name: CreateVariant :one
INSERT INTO file_variants (
    file_id,
    variant_type,
    content_type,
    size_bytes,
    storage_key,
    width,
    height
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: DeleteVariantsByFile :exec
DELETE FROM file_variants
WHERE file_id = $1;

-- name: DeleteVariant :exec
DELETE FROM file_variants
WHERE id = $1;
