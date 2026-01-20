-- name: CreateZipDownload :one
INSERT INTO zip_downloads (user_id, file_ids, status)
VALUES ($1, $2, 'pending')
RETURNING *;

-- name: GetZipDownload :one
SELECT * FROM zip_downloads
WHERE id = $1;

-- name: GetZipDownloadByUser :one
SELECT * FROM zip_downloads
WHERE id = $1 AND user_id = $2;

-- name: UpdateZipDownloadCompleted :exec
UPDATE zip_downloads
SET status = 'completed',
    storage_key = $2,
    size_bytes = $3,
    download_url = $4,
    expires_at = $5,
    completed_at = NOW()
WHERE id = $1;

-- name: UpdateZipDownloadFailed :exec
UPDATE zip_downloads
SET status = 'failed',
    error_message = $2,
    completed_at = NOW()
WHERE id = $1;

-- name: UpdateZipDownloadRunning :exec
UPDATE zip_downloads
SET status = 'running'
WHERE id = $1;

-- name: ListZipDownloadsByUser :many
SELECT * FROM zip_downloads
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: DeleteExpiredZipDownloads :exec
DELETE FROM zip_downloads
WHERE expires_at IS NOT NULL AND expires_at < NOW();

-- name: CountPendingZipDownloadsByUser :one
SELECT COUNT(*) FROM zip_downloads
WHERE user_id = $1 AND status IN ('pending', 'running');
