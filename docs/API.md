# API Reference

## Base URL

`https://api.file.cheap`

## Authentication

### API Keys

API keys can be obtained through the device authentication flow or created via the web UI settings page.

Include API key in Authorization header with `fp_` prefix:
```
Authorization: Bearer fp_your_api_key_here
```

### JWT Tokens

Include JWT token in Authorization header:
```
Authorization: Bearer <token>
```

JWT tokens are issued through the web UI session system and can be used for programmatic API access.

### Device Authentication Flow

For CLI tools and applications that cannot open a web browser directly.

#### Initiate Device Flow

**POST** `/v1/auth/device`

No authentication required.

**Response:** `200 OK`
```json
{
  "device_code": "device_code_abc123",
  "user_code": "ABCD-1234",
  "verification_uri": "https://file.cheap/auth/device",
  "expires_in": 900,
  "interval": 5
}
```

The user should visit `verification_uri` and enter the `user_code` to authorize the device.

#### Poll for Token

**POST** `/v1/auth/device/token`

**Request Body:**
```json
{
  "device_code": "device_code_abc123"
}
```

**Response (Pending):** `400 Bad Request`
```json
{
  "error": "authorization_pending",
  "error_description": "User has not yet authorized this device"
}
```

**Response (Approved):** `200 OK`
```json
{
  "api_key": "fp_base64_encoded_token_here"
}
```

Poll this endpoint every `interval` seconds (from initiate response) until approved or expired.

#### Approve Device (Web UI)

**POST** `/v1/auth/device/approve`

Authentication: JWT required (web session)

**Request Body:**
```json
{
  "user_code": "ABCD-1234"
}
```

**Response:** `200 OK`

This endpoint is called by the web UI when a user authorizes a device.

### Session (Web UI)

Web UI uses httpOnly cookies for session management. No explicit authentication required in requests.

## API Endpoints (v1)

### Health Check

**GET** `/health`

No authentication required.

**Response:**
```json
{
  "status": "ok"
}
```

### Upload File

**POST** `/v1/upload`

Authentication: API key or JWT required

**Request:**
- Content-Type: `multipart/form-data`
- Body:
  - `file` (file): File to upload

**Response:** `202 Accepted`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "filename": "example.jpg",
  "status": "pending"
}
```

The upload returns immediately. Processing jobs are enqueued in the background.

**Error Responses:**
- `400 Bad Request` - Invalid file or missing file
- `401 Unauthorized` - Missing or invalid token
- `403 Forbidden` - File limit reached or file too large for tier
- `413 Payload Too Large` - File exceeds maximum size limit
- `500 Internal Server Error` - Server error

### List Files

**GET** `/v1/files`

Authentication: API key or JWT required

**Query Parameters:**
- `limit` (int, optional): Number of files to return (default: 20, max: 100)
- `offset` (int, optional): Pagination offset (default: 0)

**Response:** `200 OK`
```json
{
  "files": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "filename": "example.jpg",
      "content_type": "image/jpeg",
      "size_bytes": 1024000,
      "status": "completed",
      "created_at": "2026-01-06T12:00:00Z"
    }
  ],
  "total": 42,
  "has_more": false
}
```

**Notes:**
- Variants are not included in list responses. Use the Get File endpoint to retrieve variant information.
- `has_more` indicates if there are additional pages of results.

### Get File

**GET** `/v1/files/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Response:** `200 OK`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "filename": "example.jpg",
  "content_type": "image/jpeg",
  "size_bytes": 1024000,
  "status": "completed",
  "created_at": "2026-01-06T12:00:00Z",
  "variants": [
    {
      "id": "223e4567-e89b-12d3-a456-426614174000",
      "variant_type": "thumbnail",
      "content_type": "image/jpeg",
      "size_bytes": 10240,
      "width": 200,
      "height": 200
    },
    {
      "id": "323e4567-e89b-12d3-a456-426614174000",
      "variant_type": "large",
      "content_type": "image/jpeg",
      "size_bytes": 512000,
      "width": 1920,
      "height": 1080
    }
  ]
}
```

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - File not found or not owned by user

### Download File

**GET** `/v1/files/{id}/download`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Query Parameters:**
- `variant` (string, optional): Variant type (thumbnail, small, medium, large). Omit for original.

**Response:** `307 Temporary Redirect`

Redirects to presigned storage URL valid for 1 hour.

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - File or variant not found

### Transform File

**POST** `/v1/files/{id}/transform`

Authentication: API key or JWT required

Apply transformations to an existing file. Jobs are enqueued and processed asynchronously.

**Path Parameters:**
- `id` (uuid): File ID

**Request Body:**
```json
{
  "presets": ["thumbnail", "sm", "md", "lg"],
  "webp": true,
  "quality": 85,
  "watermark": "© 2026 Your Company"
}
```

**Request Parameters:**
- `presets` (array[string], optional): Array of preset names to apply
- `webp` (boolean, optional): Convert to WebP format
- `quality` (int, optional): JPEG/WebP quality 1-100 (default: 85)
- `watermark` (string, optional): Watermark text to overlay (Pro/Enterprise only)

**Response:** `202 Accepted`
```json
{
  "file_id": "123e4567-e89b-12d3-a456-426614174000",
  "jobs": ["job_001", "job_002", "job_003"]
}
```

**Error Responses:**
- `400 Bad Request` - Invalid request or no transformations specified
- `401 Unauthorized` - Missing or invalid token
- `403 Forbidden` - Transformation limit reached or feature not available on tier
- `404 Not Found` - File not found
- `503 Service Unavailable` - Job queue unavailable

**Notes:**
- At least one transformation (preset, webp, or watermark) is required
- Watermark feature requires Pro or Enterprise tier
- Some presets may be restricted by subscription tier
- Each transformation counts against your monthly limit

### Delete File

**DELETE** `/v1/files/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Response:** `204 No Content`

Soft deletes the file, all variants, and associated database records.

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - File not found or not owned by user
- `500 Internal Server Error` - Deletion failed

## Batch Transformations

Process multiple files with the same transformations in a single request.

### Create Batch Transform

**POST** `/v1/batch/transform`

Authentication: API key or JWT required

**Request Body:**
```json
{
  "file_ids": [
    "123e4567-e89b-12d3-a456-426614174000",
    "223e4567-e89b-12d3-a456-426614174001"
  ],
  "presets": ["thumbnail", "md"],
  "webp": true,
  "quality": 85,
  "watermark": "© 2026 Your Company"
}
```

**Request Parameters:**
- `file_ids` (array[string], required): Array of file UUIDs (max: 100 files)
- `presets` (array[string], optional): Array of preset names to apply
- `webp` (boolean, optional): Convert to WebP format
- `quality` (int, optional): JPEG/WebP quality 1-100 (default: 85)
- `watermark` (string, optional): Watermark text to overlay (Pro/Enterprise only)

**Response:** `202 Accepted`
```json
{
  "batch_id": "b23e4567-e89b-12d3-a456-426614174000",
  "total_files": 2,
  "total_jobs": 6,
  "status": "pending",
  "status_url": "/v1/batch/b23e4567-e89b-12d3-a456-426614174000"
}
```

**Error Responses:**
- `400 Bad Request` - Invalid request, no files, or no transformations specified
- `401 Unauthorized` - Missing or invalid token
- `403 Forbidden` - Transformation limit reached or feature not available on tier

**Notes:**
- Maximum 100 files per batch
- At least one transformation is required
- Only files owned by the authenticated user will be processed
- Invalid file IDs are silently skipped

### Get Batch Status

**GET** `/v1/batch/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): Batch operation ID

**Query Parameters:**
- `include_items` (boolean, optional): Include detailed item status (default: false)

**Response:** `200 OK`
```json
{
  "id": "b23e4567-e89b-12d3-a456-426614174000",
  "status": "processing",
  "total_files": 2,
  "completed_files": 1,
  "failed_files": 0,
  "presets": ["thumbnail", "md"],
  "webp": true,
  "quality": 85,
  "watermark": "© 2026 Your Company",
  "created_at": "2026-01-06T12:00:00Z",
  "started_at": "2026-01-06T12:00:01Z",
  "completed_at": null
}
```

**With `include_items=true`:**
```json
{
  "id": "b23e4567-e89b-12d3-a456-426614174000",
  "status": "processing",
  "total_files": 2,
  "completed_files": 1,
  "failed_files": 0,
  "presets": ["thumbnail", "md"],
  "webp": true,
  "quality": 85,
  "created_at": "2026-01-06T12:00:00Z",
  "started_at": "2026-01-06T12:00:01Z",
  "completed_at": null,
  "items": [
    {
      "file_id": "123e4567-e89b-12d3-a456-426614174000",
      "status": "completed",
      "job_ids": ["job_001", "job_002", "job_003"],
      "error_message": null
    },
    {
      "file_id": "223e4567-e89b-12d3-a456-426614174001",
      "status": "processing",
      "job_ids": ["job_004", "job_005", "job_006"],
      "error_message": null
    }
  ]
}
```

**Batch Statuses:**
- `pending` - Batch created, jobs not yet started
- `processing` - Jobs are being processed
- `completed` - All jobs completed successfully
- `failed` - One or more jobs failed

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - Batch not found or not owned by user

## Bulk Downloads (ZIP Export)

Download multiple files as a single ZIP archive.

### Create Bulk Download

**POST** `/v1/downloads/zip`

Authentication: API key or JWT required

Request multiple files to be packaged into a ZIP archive for download.

**Request Body:**
```json
{
  "file_ids": [
    "123e4567-e89b-12d3-a456-426614174000",
    "223e4567-e89b-12d3-a456-426614174001",
    "323e4567-e89b-12d3-a456-426614174002"
  ]
}
```

**Request Parameters:**
- `file_ids` (array[string], required): Array of file UUIDs to include (max: 100 files)

**Response:** `202 Accepted`
```json
{
  "id": "z23e4567-e89b-12d3-a456-426614174000",
  "status": "pending",
  "status_url": "/v1/downloads/z23e4567-e89b-12d3-a456-426614174000"
}
```

**Error Responses:**
- `400 Bad Request` - No files provided, too many files (>100), or no valid file IDs
- `401 Unauthorized` - Missing or invalid token
- `429 Too Many Requests` - Too many pending downloads (max 3 concurrent)

**Notes:**
- Maximum 100 files per ZIP download
- Only files owned by the authenticated user are included
- Invalid file IDs are silently skipped
- ZIP archives expire after 24 hours (configurable via `ZIP_DOWNLOAD_EXPIRY`)

### Get Bulk Download Status

**GET** `/v1/downloads/{id}`

Authentication: API key or JWT required

Check the status of a ZIP download request.

**Path Parameters:**
- `id` (uuid): Bulk download ID

**Response:** `200 OK`
```json
{
  "id": "z23e4567-e89b-12d3-a456-426614174000",
  "status": "completed",
  "file_count": 3,
  "size_bytes": 15728640,
  "download_url": "https://storage.file.cheap/downloads/z23e4567...?signature=...",
  "expires_at": "2026-01-07T12:00:00Z",
  "created_at": "2026-01-06T12:00:00Z",
  "completed_at": "2026-01-06T12:00:30Z"
}
```

**Status Values:**
- `pending` - ZIP creation queued
- `running` - ZIP is being created
- `completed` - ZIP ready for download
- `failed` - ZIP creation failed (see `error_message`)

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - Download not found or not owned by user

### List Bulk Downloads

**GET** `/v1/downloads`

Authentication: API key or JWT required

List your ZIP download history.

**Query Parameters:**
- `limit` (int, optional): Number of downloads to return (default: 20, max: 100)
- `offset` (int, optional): Pagination offset (default: 0)

**Response:** `200 OK`
```json
{
  "downloads": [
    {
      "id": "z23e4567-e89b-12d3-a456-426614174000",
      "status": "completed",
      "file_count": 3,
      "size_bytes": 15728640,
      "download_url": "https://...",
      "expires_at": "2026-01-07T12:00:00Z",
      "created_at": "2026-01-06T12:00:00Z",
      "completed_at": "2026-01-06T12:00:30Z"
    }
  ]
}
```

## Share Links & CDN

### Create Share Link

**POST** `/v1/files/{id}/share`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Query Parameters:**
- `expires` (duration, optional): Expiration time (e.g., `24h`, `7d`, `30d`)

**Response:** `201 Created`
```json
{
  "id": "s23e4567-e89b-12d3-a456-426614174000",
  "token": "abc123def456",
  "share_url": "https://file.cheap/cdn/abc123def456/_/example.jpg",
  "expires_at": "2026-01-07T12:00:00Z"
}
```

**Notes:**
- `expires_at` is only included if an expiration was set
- Shares without expiration are valid indefinitely
- The share URL can be used with CDN transforms (see CDN Transform API section)

### List Shares

**GET** `/v1/files/{id}/shares`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Response:** `200 OK`
```json
{
  "shares": [
    {
      "id": "s23e4567-e89b-12d3-a456-426614174000",
      "token": "abc123def456",
      "access_count": 42,
      "created_at": "2026-01-06T12:00:00Z",
      "expires_at": "2026-01-13T12:00:00Z"
    }
  ]
}
```

### Delete Share

**DELETE** `/v1/shares/{shareId}`

Authentication: API key or JWT required

**Path Parameters:**
- `shareId` (uuid): Share ID (not the share token)

**Response:** `204 No Content`

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - Share not found or not owned by user

## CDN Transform API

The CDN provides on-demand image and PDF transformations via shareable URLs.

### CDN URL Format

```
GET /cdn/{share_token}/{transforms}/{filename}
```

**Path Parameters:**
- `share_token` - File share token (obtained via share creation endpoint)
- `transforms` - Comma-separated transformation parameters (use `_` or `original` for no transforms)
- `filename` - Original filename

### Transform Parameters

| Key | Description | Range | Example |
|-----|-------------|-------|---------|
| `w` | Width in pixels | 1-10000 | `w_800` |
| `h` | Height in pixels | 1-10000 | `h_600` |
| `q` | Quality (JPEG) | 1-100 | `q_85` |
| `f` | Format | webp, jpg, png, gif | `f_webp` |
| `c` | Crop mode | thumb, fit, fill, cover, contain | `c_cover` |
| `wm` | Watermark text | max 100 chars | `wm_copyright` |
| `p` | Page (PDF only) | 1-9999 | `p_1` |

### Examples

**Original file:**
```
GET /cdn/abc123/_/document.pdf
GET /cdn/abc123/original/image.jpg
```

**Resize image:**
```
GET /cdn/abc123/w_800,h_600/image.jpg
GET /cdn/abc123/w_1200/photo.png
```

**Convert to WebP:**
```
GET /cdn/abc123/f_webp,q_80/image.jpg
```

**PDF page to image:**
```
GET /cdn/abc123/p_1/document.pdf           # First page as PNG
GET /cdn/abc123/p_3,w_800/document.pdf     # Page 3, 800px wide
GET /cdn/abc123/p_1,f_jpeg,q_90/doc.pdf    # First page as JPEG
```

**Combination:**
```
GET /cdn/abc123/w_300,h_300,c_thumb,q_85/image.jpg
```

### CDN Caching Behavior

To optimize performance and reduce processing costs, the CDN automatically caches frequently requested transforms:

- **Automatic Caching**: Transforms requested 3 or more times are automatically cached
- **Cache Headers**:
  - Cached transforms: `Cache-Control: public, max-age=31536000, immutable`
  - Non-cached transforms: `Cache-Control: public, max-age=3600`
- **Storage**: Cached variants are stored separately and don't count against variant limits
- **Performance**: Cached transforms are served directly from storage without reprocessing

**Notes:**
- First 2 requests for a transform are processed on-demand
- 3rd and subsequent requests serve from cache
- Different transform parameters create different cache entries
- Original files are always served from cache

## Analytics API

### User Analytics

**GET** `/v1/analytics`

Returns comprehensive analytics for the authenticated user.

**Response:** `200 OK`
```json
{
  "files_used": 45,
  "files_limit": 100,
  "transforms_used": 230,
  "transforms_limit": 1000,
  "storage_used_bytes": 1073741824,
  "storage_limit_bytes": 5368709120,
  "plan_name": "pro",
  "plan_renews_at": "2024-02-15T00:00:00Z",
  "days_until_renewal": 15,
  "daily_usage": [...],
  "transform_breakdown": [...],
  "top_files": [...],
  "recent_activity": [...]
}
```

### Usage Summary

**GET** `/v1/analytics/usage`

Returns usage limits and current consumption.

**Response:** `200 OK`
```json
{
  "files_used": 45,
  "files_limit": 100,
  "transforms_used": 230,
  "transforms_limit": 1000,
  "storage_used_bytes": 1073741824,
  "storage_limit_bytes": 5368709120,
  "plan_name": "pro",
  "plan_renews_at": "2024-02-15T00:00:00Z",
  "days_until_renewal": 15
}
```

### Storage Analytics

**GET** `/v1/analytics/storage`

Returns storage breakdown by file type and variant type.

**Response:** `200 OK`
```json
{
  "breakdown_by_type": [
    {"file_type": "image", "file_count": 100, "total_bytes": 524288000},
    {"file_type": "video", "file_count": 10, "total_bytes": 1073741824}
  ],
  "breakdown_by_variant": [
    {"variant_type": "thumbnail", "variant_count": 100, "total_bytes": 10485760},
    {"variant_type": "webp", "variant_count": 50, "total_bytes": 52428800}
  ],
  "largest_files": [...],
  "total_bytes": 1598029824
}
```

### Activity Feed

**GET** `/v1/analytics/activity`

Returns recent activity feed for the user.

**Response:** `200 OK`
```json
{
  "recent_activity": [
    {
      "id": "uuid",
      "type": "upload",
      "message": "image.jpg uploaded",
      "status": "success",
      "created_at": "2024-01-20T10:30:00Z",
      "time_ago": "5 minutes ago"
    }
  ]
}
```

### Enhanced Analytics

**GET** `/v1/analytics/enhanced`

Returns enhanced analytics with processing volume trends, storage growth, bandwidth stats, and cost forecasting.

**Response:** `200 OK`
```json
{
  "processing_volume": [
    {"date": "2024-01-20", "job_type": "thumbnail", "count": 25, "total_duration_seconds": 45}
  ],
  "storage_growth": [
    {"date": "2024-01-20", "cumulative_bytes": 1073741824, "bytes_added": 52428800, "files_added": 5}
  ],
  "video_stats": {
    "total_video_files": 10,
    "total_video_bytes": 536870912,
    "total_video_jobs": 15,
    "transcode_jobs": 8,
    "hls_jobs": 5,
    "video_thumbnail_jobs": 2,
    "avg_transcode_duration_seconds": 45.5,
    "total_video_processing_seconds": 364,
    "estimated_video_minutes": 6.07,
    "estimated_monthly_cost": 0.91
  },
  "bandwidth_stats": {
    "total_downloads": 150,
    "estimated_bandwidth_bytes": 1610612736,
    "estimated_bandwidth_gb": 1.5
  },
  "cost_forecast": {
    "total_storage_bytes": 1073741824,
    "total_storage_gb": 1.0,
    "total_files": 50,
    "estimated_storage_cost": 0.02,
    "jobs_last_30_days": 100,
    "video_jobs_last_30_days": 15,
    "video_processing_seconds_30_days": 364,
    "video_minutes_30_days": 6.07,
    "estimated_processing_cost": 0.91,
    "downloads_30_days": 150,
    "estimated_bandwidth_cost": 0.14,
    "total_estimated_monthly_cost": 1.07
  },
  "processing_by_tier": [...]
}
```

### Admin Analytics (Admin Only)

**GET** `/v1/admin/analytics`

Returns platform-wide analytics dashboard. Requires admin role.

**Response:** `200 OK`
```json
{
  "mrr": 4500.00,
  "mrr_growth": 12.5,
  "total_users": 1250,
  "new_users_this_week": 35,
  "total_files": 125000,
  "total_storage_bytes": 536870912000,
  "jobs_last_24h": 3500,
  "job_success_rate": 99.2,
  "churn": {...},
  "revenue": {...},
  "nrr": {...},
  "cohort_data": [...],
  "revenue_history": [...],
  "users_by_plan": [...],
  "health": {...},
  "top_users": [...],
  "recent_signups": [...],
  "failed_jobs": [...]
}
```

**GET** `/v1/admin/analytics/users`

Returns user-focused admin analytics.

**GET** `/v1/admin/analytics/health`

Returns system health metrics.

**GET** `/v1/admin/analytics/enhanced`

Returns platform-wide enhanced analytics with processing volume by tier.

## Web UI Routes

### Public Pages

**GET** `/`
- Landing page
- No authentication required

**GET** `/login`
- Login form
- Redirects to `/dashboard` if already authenticated

**GET** `/register`
- Registration form
- Redirects to `/dashboard` if already authenticated

**GET** `/forgot-password`
- Password reset request form

### Authentication Actions

**POST** `/login`
- Process login form
- Creates session on success
- Redirects to `/dashboard`

**POST** `/register`
- Process registration form
- Sends verification email
- Redirects to verification pending page

**POST** `/logout`
- Destroys session
- Redirects to `/`

**POST** `/forgot-password`
- Sends password reset email
- Redirects to confirmation page

### OAuth

**GET** `/auth/google`
- Initiates Google OAuth flow

**GET** `/auth/google/callback`
- Google OAuth callback handler

**GET** `/auth/github`
- Initiates GitHub OAuth flow

**GET** `/auth/github/callback`
- GitHub OAuth callback handler

### Protected Pages (Session Required)

**GET** `/dashboard`
- User dashboard
- Shows recent files and statistics

**GET** `/upload`
- File upload form

**POST** `/upload`
- Process file upload (multipart form)
- Enqueues processing jobs
- Redirects to `/files`

**POST** `/files/upload`
- AJAX file upload endpoint
- Returns JSON response

**GET** `/files`
- List user's files
- Paginated view

**GET** `/files/{id}`
- File detail page
- Shows variants and processing status

**POST** `/files/{id}/delete`
- Delete file
- Redirects to `/files`

**GET** `/profile`
- User profile page

**POST** `/profile`
- Update profile information

**POST** `/profile/avatar`
- Upload profile avatar

**POST** `/profile/delete`
- Delete user account
- Destroys session

**GET** `/settings`
- User settings page

## Subscription Limits

The API enforces limits based on your subscription tier.

### Tier Limits

| Feature | Free | Pro | Enterprise |
|---------|------|-----|------------|
| Files | 100 | 10,000 | Unlimited |
| File Size | 10 MB | 100 MB | 500 MB |
| Transformations/mo | 1,000 | 100,000 | Unlimited |
| Watermarks | ❌ | ✅ | ✅ |
| Advanced Presets | ❌ | ✅ | ✅ |
| CDN Bandwidth | 10 GB | 1 TB | Unlimited |

### Limit-Related Error Codes

When you exceed a limit, the API returns a 403 Forbidden response with one of these error codes:

- `file_limit_reached` - You've reached your plan's file limit
  ```json
  {
    "error": {
      "code": "file_limit_reached",
      "message": "File limit of 100 reached. Upgrade to Pro for 10,000 files."
    }
  }
  ```

- `transformation_limit_reached` - You've reached your monthly transformation limit
  ```json
  {
    "error": {
      "code": "transformation_limit_reached",
      "message": "Not enough transformations remaining. Need 3, have 1."
    }
  }
  ```

- `feature_not_available` - Feature requires a higher tier
  ```json
  {
    "error": {
      "code": "feature_not_available",
      "message": "Custom watermarks are not available on your plan. Upgrade to Pro for access."
    }
  }
  ```

### Checking Your Usage

Use the web UI dashboard or API endpoints to monitor your current usage against limits.

## Available Presets

Presets provide predefined transformation settings optimized for common use cases.

### Image Presets

#### Responsive Sizes
- `thumbnail` - 300x300px thumbnail (Free tier)
- `sm` - 640px width, auto height (Free tier)
- `md` - 1024px width, auto height (Pro/Enterprise)
- `lg` - 1920px width, auto height (Pro/Enterprise)
- `xl` - 2560px width, auto height (Enterprise)

#### Social Media
- `og` - 1200x630px (Open Graph) - Pro/Enterprise
- `twitter` - 1200x675px (Twitter Card) - Pro/Enterprise
- `instagram_square` - 1080x1080px - Pro/Enterprise
- `instagram_portrait` - 1080x1350px - Pro/Enterprise
- `instagram_story` - 1080x1920px - Pro/Enterprise

### PDF Presets

- `pdf_thumbnail` - 300x300px thumbnail (Free tier)
- `pdf_sm` - 640px width, auto height (Free tier)
- `pdf_md` - 1024px width, auto height (Pro/Enterprise)
- `pdf_lg` - 1920px width, auto height (Pro/Enterprise)

### Using Presets

**Via Transform API:**
```json
{
  "presets": ["thumbnail", "md", "og"]
}
```

**Via CDN (using share links):**

Presets are applied during upload and transformation operations. CDN transforms use the `w`, `h`, and other parameters directly instead of preset names.

## Error Responses

All endpoints return consistent error format:

```json
{
  "error": {
    "code": "not_found",
    "message": "Resource not found"
  }
}
```

### Error Codes

- `bad_request` - Invalid request format or parameters
- `unauthorized` - Authentication required or invalid
- `forbidden` - Insufficient permissions or quota exceeded
- `not_found` - Resource not found
- `conflict` - Resource conflict (e.g., email already taken)
- `payload_too_large` - File exceeds size limit
- `rate_limit_exceeded` - Too many requests
- `internal_error` - Server error
- `file_limit_reached` - File storage limit reached
- `transformation_limit_reached` - Transformation quota exceeded
- `feature_not_available` - Feature requires higher tier

## Rate Limiting

API requests are rate limited to ensure fair usage and system stability.

### Limits

- **Authenticated requests**: 100 requests per second per user
- **Burst allowance**: 200 requests
- **Unauthenticated requests**: Shared rate limit by IP address

### Rate Limit Headers

Responses include rate limit information:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1672531200
```

### Rate Limit Exceeded Response

When rate limited, the API returns `429 Too Many Requests`:

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Rate limit exceeded. Please try again later."
  }
}
```

**Note**: CDN endpoints (`/cdn/*`) have separate, more generous rate limits optimized for content delivery.

## File Size Limits

Default: 100MB per file

Configure via `MAX_UPLOAD_SIZE` environment variable. Subscription tier may impose lower limits (see Subscription Limits section).

## Supported File Types

Images:
- JPEG (image/jpeg)
- PNG (image/png)
- GIF (image/gif)
- WebP (image/webp)

Videos:
- MP4 (video/mp4)
- WebM (video/webm)
- MOV (video/quicktime)
- AVI (video/x-msvideo)
- MKV (video/x-matroska)

PDFs:
- PDF (application/pdf)

## Video Processing

### Transcode Video

**POST** `/v1/files/{id}/video/transcode`

Authentication: API key or JWT required (Pro tier)

Transcode a video file to different resolutions and formats.

**Path Parameters:**
- `id` (uuid): Video file ID

**Request Body:**
```json
{
  "resolutions": [360, 720, 1080],
  "format": "mp4",
  "thumbnail": true
}
```

**Request Parameters:**
- `resolutions` (array[int], required): Target resolutions (height in pixels)
- `format` (string, optional): Output format (`mp4` or `webm`, default: `mp4`)
- `thumbnail` (boolean, optional): Generate video thumbnail (default: false)

**Response:** `202 Accepted`
```json
{
  "file_id": "123e4567-e89b-12d3-a456-426614174000",
  "jobs": ["job_001", "job_002", "job_003", "job_004"]
}
```

**Error Responses:**
- `400 Bad Request` - Invalid request, not a video file, or invalid format
- `401 Unauthorized` - Missing or invalid token
- `403 Forbidden` - Feature requires Pro tier or resolution limit exceeded
- `404 Not Found` - File not found

### Generate HLS Stream

**POST** `/v1/files/{id}/video/hls`

Authentication: API key or JWT required (Pro tier)

Generate HLS (HTTP Live Streaming) package for adaptive streaming.

**Path Parameters:**
- `id` (uuid): Video file ID

**Request Body:**
```json
{
  "segment_duration": 10,
  "resolutions": [360, 720, 1080]
}
```

**Request Parameters:**
- `segment_duration` (int, optional): Segment length in seconds (default: 10)
- `resolutions` (array[int], optional): Rendition resolutions (default: [360, 720])

**Response:** `202 Accepted`
```json
{
  "file_id": "123e4567-e89b-12d3-a456-426614174000",
  "jobs": ["job_001"]
}
```

**Error Responses:**
- `400 Bad Request` - Not a video file
- `401 Unauthorized` - Missing or invalid token
- `403 Forbidden` - HLS requires Pro tier

### Stream HLS Content

**GET** `/v1/files/{id}/hls/{segment}`

Authentication: API key or JWT required

Stream HLS manifest or segment files.

**Path Parameters:**
- `id` (uuid): Video file ID
- `segment` (string): Segment filename (e.g., `master.m3u8`, `720p.m3u8`, `segment001.ts`)

**Response:** `307 Temporary Redirect`

Redirects to presigned storage URL valid for 1 hour.

### Chunked Upload

For large video files, use chunked upload to upload in parts.

#### Initialize Chunked Upload

**POST** `/v1/upload/chunked/init`

Authentication: API key or JWT required

**Request Body:**
```json
{
  "filename": "video.mp4",
  "content_type": "video/mp4",
  "total_size": 104857600
}
```

**Response:** `200 OK`
```json
{
  "upload_id": "abc123def456",
  "chunk_size": 5242880,
  "chunks_total": 20
}
```

#### Upload Chunk

**POST** `/v1/upload/chunked/{uploadId}/chunk?chunk={index}`

Authentication: API key or JWT required

**Path Parameters:**
- `uploadId` (string): Upload session ID

**Query Parameters:**
- `chunk` (int): Chunk index (0-based)

**Request:** Binary chunk data

**Response:** `200 OK`
```json
{
  "chunk_index": 0,
  "chunks_loaded": 1,
  "chunks_total": 20
}
```

#### Complete Chunked Upload

**POST** `/v1/upload/chunked/{uploadId}/complete`

Authentication: API key or JWT required

**Response:** `200 OK`
```json
{
  "file_id": "123e4567-e89b-12d3-a456-426614174000",
  "filename": "video.mp4",
  "status": "pending"
}
```

### Video Embed

**GET** `/embed/{id}`

No authentication required (public).

Embeddable video player page for sharing videos.

**Path Parameters:**
- `id` (uuid): Video file ID

**Response:** HTML page with video player

## Job Processing

File processing is asynchronous. Upload and transform endpoints return immediately with job IDs or status URLs.

Check file detail endpoint to see variant status:
- Variants appear as they're processed
- Processing typically completes within seconds
- Failed jobs are retried up to 3 times

### Job Lifecycle

1. **Upload** - File uploaded to storage
2. **Enqueue** - Processing jobs created and added to queue
3. **Process** - Worker picks up job and processes file
4. **Complete** - Variant saved to storage and database
5. **Retry** - Failed jobs retried up to 3 times

## Presigned URLs

Download URLs expire after 1 hour.

Request new URL via download endpoint if expired.

## Pagination

List endpoints support pagination:
- `limit` - Items per page (default: 20, max: 100)
- `offset` - Skip N items

Response includes:
```json
{
  "total": 150,
  "has_more": true
}
```

Calculate if more pages exist using `has_more` field.

## PDF Processing

PDF files are automatically processed when uploaded:
- A thumbnail of the first page is generated (300x300 PNG)
- The thumbnail is stored as a `pdf_preview` variant

### PDF-Specific Features

**Automatic Processing:**
- When a PDF is uploaded, a `pdf_thumbnail` job is enqueued
- First page is rendered as a 300x300 PNG thumbnail
- Result is stored in the `pdf_preview` variant

**On-Demand Page Rendering:**
- Use CDN transform API with `p_X` parameter
- Render any page as PNG or JPEG
- Combine with resize transforms

**Manual Processing:**
- Use `pdf_preview` action via web UI file detail page
- Or enqueue `pdf_thumbnail` job via transform API

### PDF Error Handling

| Error | Description |
|-------|-------------|
| `ErrPDFEncrypted` | PDF is password-protected |
| `ErrPDFEmpty` | PDF has no pages |
| `ErrPageOutOfRange` | Requested page doesn't exist |

Encrypted and corrupted PDFs return permanent errors (no retry).

## Processing Jobs

### Job Types

| Type | Description | Supported Files |
|------|-------------|-----------------|
| `thumbnail` | 300x300 thumbnail | Images |
| `resize` | Custom dimensions | Images |
| `webp` | WebP conversion | Images |
| `watermark` | Add text watermark | Images |
| `pdf_thumbnail` | First page thumbnail | PDFs |
| `video_thumbnail` | Extract frame as thumbnail | Videos |
| `video_transcode` | Transcode to different resolution/format | Videos |
| `video_hls` | Generate HLS streaming package | Videos |
| `video_watermark` | Add text watermark overlay | Videos |

### Automatic Processing

On upload:
- **Images**: `thumbnail` job enqueued
- **PDFs**: `pdf_thumbnail` job enqueued
- **Videos**: `video_thumbnail` job enqueued (extracts frame at 10%)
- **Other**: No automatic processing

## Job Management

### List Jobs

**GET** `/v1/jobs`

Authentication: API key or JWT required

List processing jobs for the authenticated user.

**Query Parameters:**
- `status` (string, optional): Filter by status (`pending`, `processing`, `completed`, `failed`)
- `limit` (int, optional): Number of jobs to return (default: 20, max: 100)
- `offset` (int, optional): Pagination offset (default: 0)

**Response:** `200 OK`
```json
{
  "jobs": [
    {
      "id": "j23e4567-e89b-12d3-a456-426614174000",
      "file_id": "123e4567-e89b-12d3-a456-426614174000",
      "job_type": "thumbnail",
      "status": "completed",
      "priority": 0,
      "attempts": 1,
      "error_message": null,
      "created_at": "2026-01-06T12:00:00Z",
      "started_at": "2026-01-06T12:00:01Z",
      "completed_at": "2026-01-06T12:00:05Z"
    }
  ],
  "total": 42,
  "has_more": true
}
```

### Retry Failed Job

**POST** `/v1/jobs/{id}/retry`

Authentication: API key or JWT required

Retry a failed job.

**Path Parameters:**
- `id` (uuid): Job ID

**Response:** `200 OK`
```json
{
  "message": "Job queued for retry",
  "job_id": "j23e4567-e89b-12d3-a456-426614174000"
}
```

**Error Responses:**
- `400 Bad Request` - Job not in failed status
- `404 Not Found` - Job not found or not owned by user

### Cancel Job

**POST** `/v1/jobs/{id}/cancel`

Authentication: API key or JWT required

Cancel a pending or running job.

**Path Parameters:**
- `id` (uuid): Job ID

**Response:** `200 OK`
```json
{
  "message": "Job cancelled",
  "job_id": "j23e4567-e89b-12d3-a456-426614174000"
}
```

**Error Responses:**
- `400 Bad Request` - Job already completed or failed
- `404 Not Found` - Job not found or not owned by user

### Bulk Retry Failed Jobs

**POST** `/v1/jobs/retry-all`

Authentication: API key or JWT required

Retry all failed jobs for the authenticated user.

**Response:** `200 OK`
```json
{
  "message": "Jobs queued for retry",
  "count": 5
}
```

## Folder Management

### Create Folder

**POST** `/v1/folders`

Authentication: API key or JWT required

**Request Body:**
```json
{
  "name": "My Folder",
  "parent_id": null
}
```

**Request Parameters:**
- `name` (string, required): Folder name
- `parent_id` (uuid, optional): Parent folder ID (null for root)

**Response:** `201 Created`
```json
{
  "id": "f23e4567-e89b-12d3-a456-426614174000",
  "name": "My Folder",
  "parent_id": null,
  "created_at": "2026-01-06T12:00:00Z"
}
```

### List Folders

**GET** `/v1/folders`

Authentication: API key or JWT required

List root-level folders.

**Response:** `200 OK`
```json
{
  "folders": [
    {
      "id": "f23e4567-e89b-12d3-a456-426614174000",
      "name": "My Folder",
      "parent_id": null,
      "file_count": 10,
      "created_at": "2026-01-06T12:00:00Z"
    }
  ]
}
```

### Get Folder Contents

**GET** `/v1/folders/{id}`

Authentication: API key or JWT required

Get folder details and contents.

**Path Parameters:**
- `id` (uuid): Folder ID

**Response:** `200 OK`
```json
{
  "id": "f23e4567-e89b-12d3-a456-426614174000",
  "name": "My Folder",
  "parent_id": null,
  "created_at": "2026-01-06T12:00:00Z",
  "subfolders": [],
  "files": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "filename": "example.jpg",
      "content_type": "image/jpeg",
      "size_bytes": 1024000,
      "created_at": "2026-01-06T12:00:00Z"
    }
  ]
}
```

### Update Folder

**PUT** `/v1/folders/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): Folder ID

**Request Body:**
```json
{
  "name": "Renamed Folder"
}
```

**Response:** `200 OK`
```json
{
  "id": "f23e4567-e89b-12d3-a456-426614174000",
  "name": "Renamed Folder",
  "parent_id": null,
  "created_at": "2026-01-06T12:00:00Z"
}
```

### Delete Folder

**DELETE** `/v1/folders/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): Folder ID

**Query Parameters:**
- `recursive` (boolean, optional): Delete folder and all contents (default: false)

**Response:** `204 No Content`

**Error Responses:**
- `400 Bad Request` - Folder not empty and recursive=false
- `404 Not Found` - Folder not found

### Move File to Folder

**POST** `/v1/files/{id}/move`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Request Body:**
```json
{
  "folder_id": "f23e4567-e89b-12d3-a456-426614174000"
}
```

**Request Parameters:**
- `folder_id` (uuid, optional): Target folder ID (null to move to root)

**Response:** `200 OK`
```json
{
  "message": "File moved successfully"
}
```

## File Tags

Organize files with custom labels/tags for better organization beyond folders.

### Add Tags to File

**POST** `/v1/files/{id}/tags`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Request Body:**
```json
{
  "tags": ["project-alpha", "review-needed", "2026"]
}
```

**Request Parameters:**
- `tags` (array[string], required): Tags to add (max 20 per request, each max 100 chars)

**Response:** `201 Created`
```json
{
  "tags": [
    {
      "file_id": "123e4567-e89b-12d3-a456-426614174000",
      "tag_name": "project-alpha",
      "created_at": "2026-01-06T12:00:00Z"
    },
    {
      "file_id": "123e4567-e89b-12d3-a456-426614174000",
      "tag_name": "review-needed",
      "created_at": "2026-01-06T12:00:00Z"
    }
  ]
}
```

**Error Responses:**
- `400 Bad Request` - No tags provided or too many tags (>20)
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - File not found or not owned by user

**Notes:**
- Duplicate tags are silently ignored
- Empty or oversized tags are skipped

### Remove Tag from File

**DELETE** `/v1/files/{id}/tags/{tag}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): File ID
- `tag` (string): Tag name to remove

**Response:** `204 No Content`

### List File Tags

**GET** `/v1/files/{id}/tags`

Authentication: API key or JWT required

Get all tags for a specific file.

**Path Parameters:**
- `id` (uuid): File ID

**Response:** `200 OK`
```json
{
  "tags": ["project-alpha", "review-needed", "2026"]
}
```

### List User Tags

**GET** `/v1/tags`

Authentication: API key or JWT required

Get all tags used by the authenticated user with file counts.

**Response:** `200 OK`
```json
{
  "tags": [
    {
      "tag_name": "project-alpha",
      "file_count": 15
    },
    {
      "tag_name": "review-needed",
      "file_count": 8
    }
  ]
}
```

### List Files by Tag

**GET** `/v1/tags/{tag}/files`

Authentication: API key or JWT required

Get all files with a specific tag.

**Path Parameters:**
- `tag` (string): Tag name

**Query Parameters:**
- `limit` (int, optional): Number of files to return (default: 20, max: 100)
- `offset` (int, optional): Pagination offset (default: 0)

**Response:** `200 OK`
```json
{
  "files": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "filename": "document.pdf",
      "content_type": "application/pdf",
      "size_bytes": 1024000,
      "status": "completed",
      "created_at": "2026-01-06T12:00:00Z"
    }
  ],
  "total": 15,
  "has_more": true
}
```

### Rename Tag

**PUT** `/v1/tags/{tag}`

Authentication: API key or JWT required

Rename a tag across all files.

**Path Parameters:**
- `tag` (string): Current tag name

**Request Body:**
```json
{
  "new_name": "project-beta"
}
```

**Response:** `200 OK`
```json
{
  "old_name": "project-alpha",
  "new_name": "project-beta"
}
```

**Error Responses:**
- `400 Bad Request` - Invalid new name (empty or >100 chars)

### Delete Tag

**DELETE** `/v1/tags/{tag}`

Authentication: API key or JWT required

Remove a tag from all files.

**Path Parameters:**
- `tag` (string): Tag name to delete

**Response:** `204 No Content`

## Webhooks

Webhooks allow you to receive real-time notifications when events occur.

### Create Webhook

**POST** `/v1/webhooks`

Authentication: API key or JWT required

**Request Body:**
```json
{
  "url": "https://example.com/webhook",
  "events": ["file.uploaded", "processing.completed", "processing.failed"],
  "secret": "your_webhook_secret"
}
```

**Request Parameters:**
- `url` (string, required): Webhook endpoint URL
- `events` (array[string], required): Events to subscribe to
- `secret` (string, optional): Secret for signing webhook payloads

**Response:** `201 Created`
```json
{
  "id": "w23e4567-e89b-12d3-a456-426614174000",
  "url": "https://example.com/webhook",
  "events": ["file.uploaded", "processing.completed", "processing.failed"],
  "enabled": true,
  "created_at": "2026-01-06T12:00:00Z"
}
```

### List Webhooks

**GET** `/v1/webhooks`

Authentication: API key or JWT required

**Response:** `200 OK`
```json
{
  "webhooks": [
    {
      "id": "w23e4567-e89b-12d3-a456-426614174000",
      "url": "https://example.com/webhook",
      "events": ["file.uploaded", "processing.completed", "processing.failed"],
      "enabled": true,
      "created_at": "2026-01-06T12:00:00Z",
      "last_triggered_at": "2026-01-06T14:30:00Z"
    }
  ]
}
```

### Get Webhook

**GET** `/v1/webhooks/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): Webhook ID

**Response:** `200 OK`
```json
{
  "id": "w23e4567-e89b-12d3-a456-426614174000",
  "url": "https://example.com/webhook",
  "events": ["file.uploaded", "processing.completed", "processing.failed"],
  "enabled": true,
  "created_at": "2026-01-06T12:00:00Z",
  "last_triggered_at": "2026-01-06T14:30:00Z"
}
```

### Update Webhook

**PUT** `/v1/webhooks/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): Webhook ID

**Request Body:**
```json
{
  "url": "https://example.com/new-webhook",
  "events": ["file.uploaded"],
  "enabled": false
}
```

**Response:** `200 OK`

### Delete Webhook

**DELETE** `/v1/webhooks/{id}`

Authentication: API key or JWT required

**Path Parameters:**
- `id` (uuid): Webhook ID

**Response:** `204 No Content`

### List Webhook Deliveries

**GET** `/v1/webhooks/{id}/deliveries`

Authentication: API key or JWT required

Get delivery history for a webhook.

**Path Parameters:**
- `id` (uuid): Webhook ID

**Query Parameters:**
- `limit` (int, optional): Number of deliveries to return (default: 20)
- `offset` (int, optional): Pagination offset

**Response:** `200 OK`
```json
{
  "deliveries": [
    {
      "id": "d23e4567-e89b-12d3-a456-426614174000",
      "event": "file.uploaded",
      "status_code": 200,
      "success": true,
      "delivered_at": "2026-01-06T14:30:00Z",
      "response_time_ms": 150
    }
  ]
}
```

### Test Webhook

**POST** `/v1/webhooks/{id}/test`

Authentication: API key or JWT required

Send a test event to the webhook.

**Path Parameters:**
- `id` (uuid): Webhook ID

**Response:** `200 OK`
```json
{
  "success": true,
  "status_code": 200,
  "response_time_ms": 120
}
```

### Webhook Event Types

| Event | Description |
|-------|-------------|
| `file.uploaded` | File has been uploaded |
| `processing.started` | Processing job has started |
| `processing.completed` | Processing job completed successfully |
| `processing.failed` | Processing job failed |

### Webhook Payload Example

```json
{
  "event": "processing.completed",
  "timestamp": "2026-01-06T14:30:00Z",
  "data": {
    "file_id": "123e4567-e89b-12d3-a456-426614174000",
    "filename": "example.jpg",
    "job_type": "thumbnail",
    "variant_id": "223e4567-e89b-12d3-a456-426614174000"
  }
}
```

### Webhook Security

When a webhook secret is configured, payloads are signed with HMAC-SHA256:

```
X-Webhook-Signature: sha256=abc123...
```

Verify the signature by computing HMAC-SHA256 of the raw payload body with your secret.

### Webhook Dead Letter Queue (DLQ)

View and retry permanently failed webhook deliveries.

#### List Failed Deliveries

**GET** `/v1/webhooks/dlq`

Authentication: API key or JWT required

List all failed webhook deliveries that exhausted retry attempts.

**Query Parameters:**
- `limit` (int, optional): Number of entries to return (default: 20, max: 100)
- `offset` (int, optional): Pagination offset (default: 0)

**Response:** `200 OK`
```json
{
  "entries": [
    {
      "id": "dlq23e4567-e89b-12d3-a456-426614174000",
      "webhook_id": "w23e4567-e89b-12d3-a456-426614174000",
      "delivery_id": "d23e4567-e89b-12d3-a456-426614174000",
      "event_type": "file.uploaded",
      "final_error": "connection refused",
      "attempts": 5,
      "last_response_code": null,
      "can_retry": true,
      "retried_at": null,
      "created_at": "2026-01-06T14:30:00Z"
    }
  ],
  "total": 3,
  "has_more": false
}
```

**Fields:**
- `can_retry` - Whether the entry can be manually retried
- `retried_at` - Timestamp when the entry was last retried (null if never retried)
- `last_response_code` - HTTP status code from last attempt (null if connection failed)

#### Retry Failed Delivery

**POST** `/v1/webhooks/dlq/{id}/retry`

Authentication: API key or JWT required

Manually retry a failed webhook delivery.

**Path Parameters:**
- `id` (uuid): DLQ entry ID

**Response:** `200 OK`
```json
{
  "retried": true,
  "new_delivery_id": "d33e4567-e89b-12d3-a456-426614174000"
}
```

**Error Responses:**
- `400 Bad Request` - Entry has already been retried (`can_retry: false`)
- `404 Not Found` - Entry not found or webhook not owned by user

**Notes:**
- Each DLQ entry can only be retried once
- A new delivery is created and queued for the retry
- The original DLQ entry is marked as retried

#### Delete DLQ Entry

**DELETE** `/v1/webhooks/dlq/{id}`

Authentication: API key or JWT required

Remove a failed delivery from the dead letter queue.

**Path Parameters:**
- `id` (uuid): DLQ entry ID

**Response:** `204 No Content`

## User Profile

### Get Current User

**GET** `/v1/me`

Authentication: API key or JWT required

**Response:** `200 OK`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "email": "user@example.com",
  "name": "John Doe",
  "avatar_url": "https://...",
  "subscription_tier": "pro",
  "subscription_status": "active",
  "created_at": "2026-01-01T00:00:00Z"
}
```

### Get Usage Statistics

**GET** `/v1/me/usage`

Authentication: API key or JWT required

**Response:** `200 OK`
```json
{
  "files": {
    "used": 150,
    "limit": 10000
  },
  "storage": {
    "used_bytes": 1073741824,
    "limit_bytes": 107374182400
  },
  "transformations": {
    "used": 500,
    "limit": 100000,
    "resets_at": "2026-02-01T00:00:00Z"
  }
}
```

## Real-time Updates

### File Status Polling

**GET** `/v1/files/{id}/status`

Authentication: API key or JWT required

Lightweight polling endpoint for file processing status.

**Path Parameters:**
- `id` (uuid): File ID

**Response:** `200 OK`
```json
{
  "status": "processing",
  "progress": 0.75,
  "variants_completed": 3,
  "variants_total": 4
}
```

### File Events (SSE)

**GET** `/v1/files/{id}/events`

Authentication: API key or JWT required

Server-Sent Events stream for real-time file updates.

**Path Parameters:**
- `id` (uuid): File ID

**Event Types:**
- `status` - File status changed
- `variant` - Variant processing completed
- `error` - Processing error occurred
- `complete` - All processing completed

**Example Events:**
```
event: status
data: {"status": "processing", "progress": 0.5}

event: variant
data: {"variant_type": "thumbnail", "variant_id": "223e4567-..."}

event: complete
data: {"status": "completed", "variants_count": 4}
```

### Upload Progress (SSE)

**GET** `/v1/upload/progress`

Authentication: API key or JWT required

Server-Sent Events stream for upload progress tracking.

**Event Types:**
- `progress` - Upload progress update
- `complete` - Upload completed
- `error` - Upload error

**Example Events:**
```
event: progress
data: {"file_id": "123e4567-...", "progress": 0.75, "bytes_uploaded": 75000000}

event: complete
data: {"file_id": "123e4567-...", "status": "pending"}
```

### JavaScript SSE Example

```javascript
const eventSource = new EventSource('/v1/files/123e4567-.../events', {
  headers: {
    'Authorization': 'Bearer fp_your_api_key'
  }
});

eventSource.addEventListener('status', (event) => {
  const data = JSON.parse(event.data);
  console.log('Status:', data.status, 'Progress:', data.progress);
});

eventSource.addEventListener('variant', (event) => {
  const data = JSON.parse(event.data);
  console.log('Variant ready:', data.variant_type);
});

eventSource.addEventListener('complete', (event) => {
  console.log('Processing complete');
  eventSource.close();
});

eventSource.addEventListener('error', (event) => {
  console.error('SSE error:', event);
  eventSource.close();
});
```

## API Token Permissions

API tokens can be created with specific permissions to limit access.

### Available Permissions

| Permission | Description |
|------------|-------------|
| `files:read` | View and download files |
| `files:write` | Upload files |
| `files:delete` | Delete files |
| `transform` | Create transformations |
| `shares:read` | View share links |
| `shares:write` | Create and delete share links |
| `webhooks:read` | View webhooks |
| `webhooks:write` | Create, update, and delete webhooks |

### Permission Presets

| Preset | Permissions |
|--------|-------------|
| Read-only | `files:read`, `shares:read` |
| Standard | `files:read`, `files:write`, `transform`, `shares:read`, `shares:write` |
| Full | All permissions |

### Token Expiration

API tokens can be created with an expiration date. Expired tokens will be rejected with a `401 Unauthorized` response.

Tokens without expiration are valid indefinitely (not recommended for production use).

## Configuration

### Environment Variables

The following environment variables can be used to configure the API:

#### Timeouts

| Variable | Description | Default |
|----------|-------------|---------|
| `UPLOAD_TIMEOUT` | Maximum time for file uploads | `5m` |
| `CDN_TRANSFORM_TIMEOUT` | Maximum time for on-demand CDN transforms | `30s` |
| `WEBHOOK_DELIVERY_TIMEOUT` | Maximum time for webhook HTTP requests | `30s` |
| `PRESIGNED_URL_EXPIRY` | How long presigned download URLs are valid | `1h` |
| `ZIP_DOWNLOAD_EXPIRY` | How long ZIP download links are valid | `24h` |

**Format:** Duration strings like `30s`, `5m`, `1h`, `24h`

**Examples:**
```bash
# Quick timeouts for development
UPLOAD_TIMEOUT=1m
CDN_TRANSFORM_TIMEOUT=10s

# Extended timeouts for large files
UPLOAD_TIMEOUT=15m
ZIP_DOWNLOAD_EXPIRY=72h
```

#### Other Configuration

See the deployment documentation for a complete list of environment variables including database, storage, and authentication configuration.
