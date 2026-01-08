# API Reference

## Base URL

Development: `http://localhost:8080`

## Authentication

### JWT (API Endpoints)

Include JWT token in Authorization header:
```
Authorization: Bearer <token>
```

Obtain token via login endpoint (to be implemented).

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

**POST** `/api/v1/upload`

Authentication: JWT required

**Request:**
- Content-Type: `multipart/form-data`
- Body:
  - `file` (file): File to upload

**Response:** `202 Accepted`
```json
{
  "file": {
    "id": "uuid",
    "user_id": "uuid",
    "filename": "example.jpg",
    "content_type": "image/jpeg",
    "size": 1024000,
    "storage_path": "user_id/file_id/original.jpg",
    "created_at": "2026-01-06T12:00:00Z"
  },
  "jobs": [
    {
      "id": "job_uuid_1",
      "type": "thumbnail"
    },
    {
      "id": "job_uuid_2",
      "type": "resize"
    }
  ]
}
```

**Error Responses:**
- `400 Bad Request` - Invalid file or missing file
- `401 Unauthorized` - Missing or invalid token
- `413 Payload Too Large` - File exceeds size limit
- `500 Internal Server Error` - Server error

### List Files

**GET** `/api/v1/files`

Authentication: JWT required

**Query Parameters:**
- `limit` (int, optional): Number of files to return (default: 50, max: 100)
- `offset` (int, optional): Pagination offset (default: 0)

**Response:** `200 OK`
```json
{
  "files": [
    {
      "id": "uuid",
      "filename": "example.jpg",
      "content_type": "image/jpeg",
      "size": 1024000,
      "created_at": "2026-01-06T12:00:00Z",
      "variants": [
        {
          "id": "uuid",
          "variant_type": "thumbnail",
          "storage_path": "user_id/file_id/thumbnail.jpg",
          "size": 10240
        }
      ]
    }
  ],
  "total": 42,
  "limit": 50,
  "offset": 0
}
```

### Get File

**GET** `/api/v1/files/{id}`

Authentication: JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "filename": "example.jpg",
  "content_type": "image/jpeg",
  "size": 1024000,
  "storage_path": "user_id/file_id/original.jpg",
  "created_at": "2026-01-06T12:00:00Z",
  "variants": [
    {
      "id": "uuid",
      "file_id": "uuid",
      "variant_type": "thumbnail",
      "storage_path": "user_id/file_id/thumbnail.jpg",
      "width": 200,
      "height": 200,
      "size": 10240,
      "created_at": "2026-01-06T12:00:30Z"
    },
    {
      "id": "uuid",
      "file_id": "uuid",
      "variant_type": "large",
      "storage_path": "user_id/file_id/large.jpg",
      "width": 1920,
      "height": 1080,
      "size": 512000,
      "created_at": "2026-01-06T12:00:35Z"
    }
  ]
}
```

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - File not found or not owned by user

### Download File

**GET** `/api/v1/files/{id}/download`

Authentication: JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Query Parameters:**
- `variant` (string, optional): Variant type (thumbnail, small, medium, large). Omit for original.

**Response:** `302 Found`

Redirects to presigned MinIO URL valid for 1 hour.

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - File or variant not found

### Delete File

**DELETE** `/api/v1/files/{id}`

Authentication: JWT required

**Path Parameters:**
- `id` (uuid): File ID

**Response:** `204 No Content`

Deletes file, all variants, and associated database records.

**Error Responses:**
- `401 Unauthorized` - Missing or invalid token
- `404 Not Found` - File not found or not owned by user
- `500 Internal Server Error` - Deletion failed

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
- `forbidden` - Insufficient permissions
- `not_found` - Resource not found
- `conflict` - Resource conflict (e.g., email already taken)
- `payload_too_large` - File exceeds size limit
- `internal_error` - Server error

## Rate Limiting

Not currently implemented.

## File Size Limits

Default: 10MB per file

Configure via `MAX_FILE_SIZE` environment variable.

## Supported File Types

Images:
- JPEG (image/jpeg)
- PNG (image/png)
- GIF (image/gif)
- WebP (image/webp)

PDFs:
- PDF (application/pdf)

## Job Processing

File processing is asynchronous. Upload returns immediately with job IDs.

Check file detail endpoint to see variant status:
- Variants appear as they're processed
- Processing typically completes within seconds
- Failed jobs are retried up to 3 times

## Presigned URLs

Download URLs expire after 1 hour.

Request new URL via download endpoint if expired.

## Pagination

List endpoints support pagination:
- `limit` - Items per page (max: 100)
- `offset` - Skip N items

Response includes:
```json
{
  "total": 150,
  "limit": 50,
  "offset": 0
}
```

Calculate pages: `total / limit`

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

### Create Share Link

**POST** `/api/v1/files/{id}/share`

Authentication: JWT required

**Query Parameters:**
- `expires` (duration, optional): Expiration time (e.g., `24h`, `7d`)

**Response:** `201 Created`
```json
{
  "id": "share_uuid",
  "token": "abc123...",
  "share_url": "https://example.com/cdn/abc123/_/filename.jpg",
  "expires_at": "2026-01-07T12:00:00Z"
}
```

### List Shares

**GET** `/api/v1/files/{id}/shares`

Authentication: JWT required

**Response:** `200 OK`
```json
{
  "shares": [
    {
      "id": "share_uuid",
      "token": "abc123...",
      "access_count": 42,
      "created_at": "2026-01-06T12:00:00Z",
      "expires_at": "2026-01-13T12:00:00Z"
    }
  ]
}
```

### Delete Share

**DELETE** `/api/v1/files/{id}/shares/{shareId}`

Authentication: JWT required

**Response:** `204 No Content`

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
- Or enqueue `pdf_thumbnail` job via API

### PDF Error Handling

| Error | Description |
|-------|-------------|
| `ErrPDFEncrypted` | PDF is password-protected |
| `ErrPDFEmpty` | PDF has no pages |
| `ErrPageOutOfRange` | Requested page doesn't exist |

Encrypted and corrupted PDFs return permanent errors (no retry).

### PDF Presets

| Preset | Dimensions | Quality | Description |
|--------|------------|---------|-------------|
| `pdf_thumbnail` | 300x300 | 85 | Default thumbnail |
| `pdf_sm` | 640xauto | 85 | Small preview |
| `pdf_md` | 1024xauto | 85 | Medium preview |
| `pdf_lg` | 1920xauto | 85 | Large preview |

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
