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

-- name: GetThumbnailsForFiles :many
SELECT file_id, storage_key, content_type, size_bytes
FROM file_variants
WHERE file_id = ANY($1::uuid[]) AND variant_type = 'thumbnail';

-- name: GetVariantTypes :many
SELECT variant_type FROM file_variants
WHERE file_id = $1;

-- name: HasVariant :one
SELECT EXISTS(
    SELECT 1 FROM file_variants
    WHERE file_id = $1 AND variant_type = $2
) AS exists;

-- name: CreateVideoVariant :one
INSERT INTO file_variants (
    file_id, variant_type, content_type, size_bytes, storage_key,
    width, height, duration_seconds, bitrate_bps, video_codec, audio_codec, frame_rate, resolution
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetVideoDurationByFile :one
SELECT duration_seconds FROM file_variants
WHERE file_id = $1 AND duration_seconds IS NOT NULL
LIMIT 1;
