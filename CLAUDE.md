# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

All commands use `task` runner (install: `brew install go-task`):

```bash
# Code generation (required after schema/query/template changes)
task gen              # Generate sqlc, templ, and CSS

# Build
task build            # Build api, worker, fc binaries to bin/
task build:api        # API binary only
task build:worker     # Worker binary only

# Testing
task test             # All tests
task test:short       # Unit tests only (-short flag)
task test:int         # Integration tests
go test -v ./internal/processor/image/...  # Single package

# Development
task up               # Start Docker services (postgres, redis, minio)
task down             # Stop services
task migrate          # Run database migrations
task run:api          # Run API server
task worker           # Run background worker
task watch:templ      # Watch templates
task watch:css        # Watch CSS

# Quality
task fmt              # Format Go and templ files
task lint             # Run golangci-lint
```

**Before committing:** Run `task test && task fmt && golangci-lint run` - CI rejects lint errors.

## Architecture

### Subdomain-Based Routing

| Domain | Purpose | Auth |
|--------|---------|------|
| `file.cheap` | Web UI (HTML) | Session cookies |
| `api.file.cheap` | REST API (JSON) | JWT Bearer tokens |

**API routes use `/v1/` prefix, NOT `/api/v1/`:**
```
https://api.file.cheap/v1/files     ✓
https://api.file.cheap/api/v1/files ✗
```

### Request Flow

1. File uploaded → MinIO storage
2. Job enqueued → Redis Streams (`github.com/abdul-hamid-achik/job-queue`)
3. Worker processes → Downloads from storage, processes, uploads variant
4. Variant metadata saved → PostgreSQL

### Key Packages

- `internal/api` - REST API handlers (JSON responses, JWT auth)
- `internal/web` - Web UI handlers (HTML templates, session auth)
- `internal/processor/{image,pdf,video}` - File processors (strategy pattern)
- `internal/worker` - Background job handlers
- `internal/storage` - MinIO/S3 abstraction
- `internal/db` - sqlc-generated database code
- `internal/apperror` - Error types with HTTP status codes

### File Processing

Processors implement a common interface and register in `internal/processor/registry.go`:
- Image: thumbnail, resize, webp, watermark
- PDF: thumbnail (uses poppler-utils)
- Video: transcode, thumbnail, HLS (uses ffmpeg)

Storage path: `{user_id}/{file_id}/{variant_type}.{ext}`

## Code Style

From AGENTS.md:
- No unnecessary comments - code should be self-documenting
- No TODO comments - implement features or leave them out
- Standard Go conventions with short, clear variable names
- Table-driven tests preferred
- Error messages lowercase without punctuation

### Error Handling

```go
// Use predefined errors from internal/apperror
return apperror.ErrNotFound
return apperror.Wrap(err, apperror.ErrInternal)
```

### Logging

```go
log := logger.FromContext(ctx)
log.Info("processing file", "file_id", fileID, "size", size)
```

## Database

- Schema: `sql/schema.sql`
- Queries: `sql/queries/*.sql`
- Generated code: `internal/db/`
- Run `task gen` after modifying queries

**When adding migrations:** Update both `migrations/NNN_*.sql` AND the `migrate` task in `Taskfile.yml`.

## Frontend

- Templates: templ (`internal/web/templates/`)
- Styling: Tailwind CSS 4 with Nord theme
- Interactivity: HTMX
- Primary text: `text-nord-5`, Secondary: `text-nord-4`
- Never use `text-nord-3` on dark backgrounds (poor contrast)

## Testing

```bash
task test                                    # All tests
task test:short                              # Unit only
go test -v ./internal/api/...                # Single package
go test -v -run TestUpload ./internal/api/   # Single test
```

Test utilities:
- `logger.NewTestLogger()` - Silent logger
- `internal/storage/mock.go` - Mock storage

## Required Environment Variables

```
DATABASE_URL     # PostgreSQL connection
REDIS_URL        # Redis connection
MINIO_ENDPOINT   # MinIO/S3 endpoint
MINIO_ACCESS_KEY # S3 access key
MINIO_SECRET_KEY # S3 secret key
JWT_SECRET       # JWT signing key
```

See `.env.example` for all options.
