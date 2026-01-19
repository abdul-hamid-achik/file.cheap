-- name: CreateFolder :one
INSERT INTO folders (user_id, parent_id, name, path)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetFolder :one
SELECT * FROM folders
WHERE id = $1 AND user_id = $2;

-- name: GetFolderByPath :one
SELECT * FROM folders
WHERE user_id = $1 AND path = $2;

-- name: ListRootFolders :many
SELECT * FROM folders
WHERE user_id = $1 AND parent_id IS NULL
ORDER BY name ASC;

-- name: ListFolderChildren :many
SELECT * FROM folders
WHERE user_id = $1 AND parent_id = $2
ORDER BY name ASC;

-- name: ListFilesInFolder :many
SELECT * FROM files
WHERE user_id = $1 AND folder_id = $2 AND deleted_at IS NULL
ORDER BY filename ASC;

-- name: ListFilesInRoot :many
SELECT * FROM files
WHERE user_id = $1 AND folder_id IS NULL AND deleted_at IS NULL
ORDER BY filename ASC;

-- name: UpdateFolder :one
UPDATE folders
SET name = $3, path = $4, parent_id = $5, updated_at = NOW()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteFolder :exec
DELETE FROM folders
WHERE id = $1 AND user_id = $2;

-- name: DeleteFolderRecursive :exec
WITH RECURSIVE folder_tree AS (
    SELECT folders.id FROM folders WHERE folders.id = $1 AND folders.user_id = $2
    UNION ALL
    SELECT f.id FROM folders f
    INNER JOIN folder_tree ft ON f.parent_id = ft.id
)
DELETE FROM folders WHERE folders.id IN (SELECT folder_tree.id FROM folder_tree);

-- name: MoveFileToFolder :exec
UPDATE files
SET folder_id = $3, updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: MoveFileToRoot :exec
UPDATE files
SET folder_id = NULL, updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: CountFolderContents :one
SELECT
    (SELECT COUNT(*) FROM folders WHERE parent_id = $1) AS folder_count,
    (SELECT COUNT(*) FROM files WHERE folder_id = $1 AND deleted_at IS NULL) AS file_count;

-- name: GetFolderPath :many
WITH RECURSIVE folder_path AS (
    SELECT folders.id, folders.parent_id, folders.name, folders.path, 1 as depth
    FROM folders
    WHERE folders.id = $1 AND folders.user_id = $2
    UNION ALL
    SELECT f.id, f.parent_id, f.name, f.path, fp.depth + 1
    FROM folders f
    INNER JOIN folder_path fp ON f.id = fp.parent_id
)
SELECT folder_path.id, folder_path.name, folder_path.path FROM folder_path
ORDER BY depth DESC;
