-- name: CreateStash :exec
INSERT INTO stashes (
    id, name, source_path, tool, created_at,
    file_count, total_size, content_hash,
    compression, compressed_size, bundle_type, indexed
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: AddTag :exec
INSERT INTO tags (stash_id, tag) VALUES (?, ?);

-- name: GetStash :one
SELECT * FROM stashes WHERE id = ?;

-- name: ListStashes :many
SELECT * FROM stashes ORDER BY created_at DESC;

-- name: ListStashesByTag :many
SELECT s.* FROM stashes s
JOIN tags t ON s.id = t.stash_id
WHERE t.tag = ?
ORDER BY s.created_at DESC;

-- name: DeleteStash :exec
DELETE FROM stashes WHERE id = ?;

-- name: DeleteTagsForStash :exec
DELETE FROM tags WHERE stash_id = ?;

-- name: MarkIndexed :exec
UPDATE stashes SET indexed = 1 WHERE id = ?;

-- name: UpdateCompression :exec
UPDATE stashes SET compression = ?, compressed_size = ? WHERE id = ?;

-- name: GetStashTags :many
SELECT tag FROM tags WHERE stash_id = ?;

-- name: SearchStashes :many
SELECT s.* FROM stashes s
WHERE s.name LIKE '%' || ? || '%'
   OR s.tool LIKE '%' || ? || '%'
   OR s.id LIKE '%' || ? || '%'
ORDER BY s.created_at DESC;