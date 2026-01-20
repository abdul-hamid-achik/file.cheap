-- name: CreateFileTag :one
INSERT INTO file_tags (file_id, user_id, tag_name)
VALUES ($1, $2, $3)
ON CONFLICT (file_id, tag_name) DO NOTHING
RETURNING *;

-- name: DeleteFileTag :exec
DELETE FROM file_tags
WHERE file_id = $1 AND tag_name = $2 AND user_id = $3;

-- name: ListTagsByFile :many
SELECT * FROM file_tags
WHERE file_id = $1
ORDER BY tag_name;

-- name: ListTagsByUser :many
SELECT DISTINCT tag_name, COUNT(*) as file_count
FROM file_tags
WHERE user_id = $1
GROUP BY tag_name
ORDER BY tag_name;

-- name: ListFilesByTag :many
SELECT f.*, COUNT(*) OVER() AS total_count
FROM files f
JOIN file_tags ft ON ft.file_id = f.id
WHERE ft.user_id = $1
  AND ft.tag_name = $2
  AND f.deleted_at IS NULL
ORDER BY f.created_at DESC
LIMIT $3 OFFSET $4;

-- name: BulkCreateFileTags :copyfrom
INSERT INTO file_tags (file_id, user_id, tag_name)
VALUES ($1, $2, $3);

-- name: DeleteAllTagsFromFile :exec
DELETE FROM file_tags
WHERE file_id = $1 AND user_id = $2;

-- name: RenameTag :exec
UPDATE file_tags
SET tag_name = $3
WHERE user_id = $1 AND tag_name = $2;

-- name: DeleteTagByName :exec
DELETE FROM file_tags
WHERE user_id = $1 AND tag_name = $2;

-- name: CountFilesByTag :one
SELECT COUNT(DISTINCT file_id)
FROM file_tags
WHERE user_id = $1 AND tag_name = $2;
