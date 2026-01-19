-- name: CreateEnterpriseInquiry :one
INSERT INTO enterprise_inquiries (
    user_id, company_name, contact_name, email, phone,
    company_size, estimated_usage, message
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetEnterpriseInquiry :one
SELECT * FROM enterprise_inquiries
WHERE id = $1;

-- name: ListEnterpriseInquiries :many
SELECT * FROM enterprise_inquiries
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListEnterpriseInquiriesByStatus :many
SELECT * FROM enterprise_inquiries
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountEnterpriseInquiries :one
SELECT COUNT(*) FROM enterprise_inquiries;

-- name: CountEnterpriseInquiriesByStatus :one
SELECT COUNT(*) FROM enterprise_inquiries
WHERE status = $1;

-- name: UpdateEnterpriseInquiryStatus :one
UPDATE enterprise_inquiries
SET
    status = $2,
    admin_notes = $3,
    processed_at = NOW(),
    processed_by = $4
WHERE id = $1
RETURNING *;

-- name: GetUserEnterpriseInquiry :one
SELECT * FROM enterprise_inquiries
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: HasPendingEnterpriseInquiry :one
SELECT EXISTS(
    SELECT 1 FROM enterprise_inquiries
    WHERE user_id = $1 AND status = 'pending'
) AS has_pending;
