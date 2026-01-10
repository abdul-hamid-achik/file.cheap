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

PDFs:
- PDF (application/pdf)

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

### Automatic Processing

On upload:
- **Images**: `thumbnail` job enqueued
- **PDFs**: `pdf_thumbnail` job enqueued
- **Other**: No automatic processing
