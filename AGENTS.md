# Agent Guidelines

This document contains guidelines for AI agents working on this codebase.

## Code Style

### No Unnecessary Comments
- Do not add comments unless absolutely necessary
- Code should be self-documenting through clear naming
- Do not add TODO comments - implement features directly or leave them out
- Do not add section dividers like `// --- Section ---`
- Do not add comments explaining what code does - the code should be clear

### Go Style
- Follow standard Go conventions
- Use short, clear variable names
- No doc comments on unexported functions unless complex
- Prefer table-driven tests
- Error messages should be lowercase without punctuation

### Templ Style
- No HTML comments in templ files
- Keep templates focused and composable
- Use consistent indentation
- Extract repeated patterns into components

## Project Structure

```
cmd/
  api/          # API server entry point
  worker/       # Background worker entry point
internal/
  api/          # REST API handlers and middleware
  apperror/     # Application error types and responses
  auth/         # Authentication (sessions, OAuth, passwords)
  config/       # Configuration loading
  db/           # Database queries (sqlc generated)
  email/        # Email templates and sending
  logger/       # Logging utilities (slog)
  processor/    # File processing (image, pdf)
  storage/      # Object storage (MinIO/S3)
  web/          # Web UI handlers and templates
  worker/       # Background job handlers
sql/
  queries/      # SQL queries for sqlc
  schema/       # Database migrations
static/
  css/          # Tailwind CSS input/output
  js/           # HTMX and Alpine.js
```

## Tech Stack

- Go 1.25+ with standard library HTTP routing
- PostgreSQL with sqlc for type-safe queries
- MinIO for object storage
- Redis for job queue (job-queue library with Redis Streams)
- templ for HTML templates
- HTMX + Alpine.js for interactivity
- Tailwind CSS 4 with Nord theme
- slog for structured logging

## Architecture

### API vs Web UI
- `/internal/api` - JSON REST API with JWT authentication
- `/internal/web` - Server-rendered HTML UI with session authentication
- Both can coexist and share business logic through internal packages

### Authentication Flows
- Web UI: session-based with httpOnly cookies
- API: JWT tokens via `Authorization: Bearer <token>` header
- OAuth: Google and GitHub (web UI only, creates session)
- Email verification required for new registrations
- Password reset via email token

### Background Jobs
- Powered by custom job-queue library (v0.3.0) built on Redis Streams
- Library: `github.com/abdul-hamid-achik/job-queue`
- Broker pattern: API server enqueues jobs, worker pool processes them
- Job payloads defined in `internal/worker/payloads.go` (ThumbnailPayload, ResizePayload)
- Worker pool configuration:
  - Concurrency: configurable via `WORKER_CONCURRENCY` (default: 4)
  - Poll interval: 1 second
  - Graceful shutdown: 30-second timeout
  - Queues: "default" queue for all jobs
- Middleware support:
  - Recovery middleware (panic handling)
  - Logging middleware (structured logs with zerolog)
  - Timeout middleware (configurable via `JOB_TIMEOUT`, default: 5m)
- Error handling:
  - Permanent errors (non-retryable): invalid payload, file not found
  - Transient errors (retryable): download/upload/processing failures
  - Max retries: configurable via `MAX_RETRIES` (default: 3)
- Job handlers in `internal/worker/handlers.go`:
  - ThumbnailHandler: creates 200x200 thumbnails (configurable dimensions)
  - ResizeHandler: creates multiple size variants (small, medium, large)
- Broker adapter in `cmd/api/main.go` wraps library for enqueueing
- Worker initialization in `cmd/worker/main.go` with full lifecycle management

## Development Workflow

### First Time Setup
```bash
task setup              # Install tools (sqlc, templ, tailwindcss)
task docker:up          # Start postgres, redis, minio
task migrate            # Run database migrations
task generate           # Generate sqlc, templ, css
```

### Daily Development
```bash
# Terminal 1: Watch and regenerate templates
task templ:watch

# Terminal 2: Watch and rebuild CSS
task css:watch

# Terminal 3: Run API server
task run:api

# Terminal 4: Run background worker
task run:worker
```

### Before Committing
```bash
task test               # Run all tests
task fmt                # Format Go and templ files
task lint               # Run golangci-lint (if available)
```

## Commands

Use `task` for all common operations. See `Taskfile.yml` for complete list.

```bash
task setup              # Install required tools
task generate           # Run sqlc, templ, and css generation
task build              # Build api and worker binaries to bin/
task build:api          # Build only API binary
task build:worker       # Build only worker binary
task test               # Run all tests
task test:unit          # Run unit tests (fast)
task test:coverage      # Generate coverage report
task docker:up          # Start all services
task docker:down        # Stop all services
task migrate            # Run database migrations
task fmt                # Format code
task clean              # Remove build artifacts
```

## Build Artifacts

- Binaries built to `bin/` directory (api, worker)
- Generated files: `*_templ.go`, `static/css/output.css`
- Never commit binaries or generated files to the repository
- `.gitignore` handles exclusions automatically

## Configuration

Configuration loaded from environment variables via `.env` file. See `.env.example` for all options.

### Required Variables
- `DATABASE_URL` - PostgreSQL connection string
- `REDIS_URL` - Redis connection string
- `MINIO_ENDPOINT` - MinIO/S3 endpoint
- `MINIO_ACCESS_KEY` - MinIO/S3 access key
- `MINIO_SECRET_KEY` - MinIO/S3 secret key
- `JWT_SECRET` - Secret for signing JWT tokens

### Optional Variables
- `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` - Google OAuth
- `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` - GitHub OAuth
- `SMTP_*` - Email configuration (defaults to Mailhog on localhost)
- `LOG_LEVEL` - Logging level (debug, info, warn, error)
- `LOG_FORMAT` - Log format (text, json - auto-detected from ENVIRONMENT)
- `WORKER_CONCURRENCY` - Number of concurrent workers (default: 4)

## Logging

Structured logging using Go's `log/slog` via `internal/logger`.

### Usage
```go
// Initialize logger (done in main)
logger.Init(cfg.LogLevel, cfg.LogFormat, cfg.Environment)

// Use logger from context (preferred)
log := logger.FromContext(ctx)
log.Info("processing file", "file_id", fileID, "size", size)

// Add request ID and user ID to context
ctx = logger.WithRequestID(ctx, requestID)
ctx = logger.WithUserID(ctx, userID)

// Log levels
log.Debug("debug message")
log.Info("info message")
log.Warn("warning message")
log.Error("error occurred", "error", err)
```

### Guidelines
- Always use structured logging (key-value pairs)
- Include relevant context (user_id, request_id, file_id, etc.)
- Don't log sensitive data (passwords, tokens, full credentials)
- Use `logger.FromContext(ctx)` to get logger with context
- Production uses JSON format, development uses text

## Error Handling

Use `internal/apperror` for consistent error responses.

### Predefined Errors
- `ErrNotFound` - 404 resource not found
- `ErrUnauthorized` - 401 authentication required
- `ErrForbidden` - 403 permission denied
- `ErrBadRequest` - 400 invalid request
- `ErrInvalidCredentials` - 401 wrong email/password
- `ErrEmailTaken` - 409 email already exists
- `ErrFileTooLarge` - 413 file exceeds max size
- `ErrInternal` - 500 internal server error

### Usage
```go
// Return predefined error
return apperror.ErrNotFound

// Wrap internal error
return apperror.Wrap(err, apperror.ErrInternal)

// Create custom error
return apperror.New("custom_code", "Custom message", http.StatusBadRequest)

// Extract status code and safe message
statusCode := apperror.StatusCode(err)
message := apperror.SafeMessage(err)
```

### Guidelines
- Never expose internal errors to API responses
- Use predefined errors when possible
- Log internal errors with full context
- Return user-friendly messages in responses
- HTTP status codes should match error semantics

## Frontend Guidelines

### Nord Theme Colors
- Background: nord-0 (#2E3440), nord-1, nord-2
- Text: nord-4 (secondary), nord-5 (primary), nord-6 (bright)
- Accent: nord-8 (cyan), nord-7 (teal)
- Status: nord-11 (red/error), nord-13 (yellow/warning), nord-14 (green/success)

### Contrast Requirements
- Primary text: use `text-nord-5` on dark backgrounds
- Secondary text: use `text-nord-4` (never nord-3 on dark backgrounds)
- Placeholders: use `placeholder-nord-4`
- Interactive elements need focus states with `focus:ring-2 focus:ring-nord-8`

### Accessibility
- Include skip-to-content links
- Use semantic HTML and ARIA roles
- All interactive elements must have focus states
- Images need alt text, decorative icons use aria-hidden

### HTMX Patterns
- Use `hx-boost` for progressive enhancement
- Return HTML fragments from endpoints
- Leverage `hx-target` and `hx-swap` for partial updates
- Use `hx-indicator` for loading states
- Handle errors with `hx-error` responses

## Database

Schema managed via migrations in `sql/schema/`. Apply with `task migrate`.

### Key Tables
- `users` - User accounts and profiles
- `sessions` - Web UI sessions
- `oauth_accounts` - Linked OAuth providers
- `api_tokens` - JWT tokens for API access
- `files` - Uploaded files metadata
- `file_variants` - Processed file variants (thumbnails, resized, etc.)
- `processing_jobs` - Background job tracking
- `password_resets` - Password reset tokens
- `email_verifications` - Email verification tokens

### Queries
- Queries defined in `sql/queries/`
- Generated to `internal/db/` via sqlc
- Type-safe Go code from SQL
- Run `task sqlc` after modifying queries

## File Processing

Processors registered in `internal/processor/registry.go`.

### Available Processors
- `thumbnail` - Generates 200x200 thumbnails (configurable)
- `resize` - Creates small/medium/large variants
- `webp` - Converts images to WebP format
- `watermark` - Adds text/image watermarks
- `metadata` - Extracts EXIF/document properties

### Storage Structure
- Original files: `{user_id}/{file_id}/original.{ext}`
- Variants: `{user_id}/{file_id}/{variant_type}.{ext}`
- Stored in MinIO bucket configured via `MINIO_BUCKET`

### Job Flow
1. File uploaded to MinIO
2. Job payload created (e.g., worker.NewThumbnailPayload)
3. Job enqueued to Redis Streams via broker.Enqueue
4. Worker pool polls Redis every 1 second
5. Worker picks up job and executes handler
6. Handler downloads original from storage
7. Handler processes file using processor registry
8. Variant uploaded to MinIO
9. Variant metadata saved to database
10. Job completion logged with duration

## Testing

### Test Organization
- Unit tests: `*_test.go` in same package
- Integration tests: `tests/integration/`
- Table-driven tests preferred for multiple cases
- Mock interfaces for external dependencies

### Running Tests
```bash
task test               # All tests
task test:unit          # Unit tests only (-short flag)
task test:coverage      # With coverage report
task test:v             # Verbose output
```

### Guidelines
- Test files live alongside implementation
- Use testify for assertions when helpful
- Mock external services (storage, email, etc.)
- Test error cases and edge conditions
- Keep tests focused and independent

### Test Utilities
- `logger.NewTestLogger()` - Silent logger for tests
- `internal/processor/image/testutil.go` - Image test helpers
- `internal/storage/mock.go` - Mock storage implementation
- `internal/api/mock_test.go` - Mock API dependencies
- `internal/worker/mock_test.go` - Mock worker dependencies

## Docker

Services defined in `docker-compose.yml`: postgres, redis, minio, grafana, loki, promtail.

```bash
docker compose up -d                    # Start all services
docker compose down                     # Stop services
docker compose down -v                  # Stop and remove volumes
docker compose logs api --tail 50       # View logs
docker compose build api                # Rebuild API image
docker compose build api && docker compose up -d api  # Rebuild and restart
```

## Git Workflow

- Create feature branches from main
- Keep commits focused and atomic
- Write clear commit messages describing the "why"
- Run `task test` before committing
- Use `task fmt` to format code before committing
