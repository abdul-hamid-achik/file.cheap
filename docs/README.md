# file.cheap Documentation

Documentation for AI agents and developers working on this codebase.

## Quick Start

### For AI Agents

If you're an AI agent working on this codebase, read these files in order:

1. **[ARCHITECTURE.md](./ARCHITECTURE.md)** - System architecture and component overview
2. **[DEVELOPMENT.md](./DEVELOPMENT.md)** - Common development tasks and patterns
3. **[API.md](./API.md)** - API endpoints and web routes reference
4. **[../AGENTS.md](../AGENTS.md)** - Code style guidelines and conventions

### For Humans

Start with ARCHITECTURE.md to understand the system, then refer to DEVELOPMENT.md for implementation patterns.

## Documentation Files

### [ARCHITECTURE.md](./ARCHITECTURE.md)
Complete system architecture documentation including:
- System diagrams
- Request/processing flows
- Component descriptions
- Authentication mechanisms
- Deployment guide
- Scaling considerations

**Read this first** to understand how everything fits together.

### [DEVELOPMENT.md](./DEVELOPMENT.md)
Practical guide for common development tasks:
- Adding processors
- Creating API endpoints
- Database migrations
- Adding templ components
- Debugging techniques
- Testing patterns
- Troubleshooting

**Use this** when implementing new features.

### [API.md](./API.md)
Complete API reference:
- REST API endpoints (v1)
- Web UI routes
- Authentication methods
- Request/response formats
- Error codes

**Refer to this** when working with endpoints.

### [../AGENTS.md](../AGENTS.md)
Code style and project conventions:
- Code style (Go, Templ, comments)
- Project structure
- Tech stack
- Development workflow
- Testing guidelines
- Git workflow

**Follow these guidelines** for all code changes.

### [STRIPE_SETUP.md](./STRIPE_SETUP.md)
Complete Stripe billing integration guide:
- API key configuration
- Product and price creation
- Customer Portal setup
- Webhook configuration
- Local testing with Stripe CLI
- Production checklist
- Troubleshooting

**Use this** when deploying billing features.

## Project Overview

file.cheap is a Go web application for uploading, storing, and processing files (primarily images and PDFs).

### Key Features

- **File Upload**: Multipart form upload via API or web UI
- **Object Storage**: MinIO (S3-compatible) for file storage
- **Background Processing**: Redis Streams job queue for async processing
- **Image Processing**: Thumbnails, resizing, WebP conversion, watermarking
- **Dual Interface**: REST API (JWT) and Web UI (sessions)
- **Authentication**: Email/password, Google OAuth, GitHub OAuth
- **Database**: PostgreSQL with type-safe sqlc queries
- **Billing**: Stripe integration with Pro plan ($19/mo), 7-day trial

### Tech Stack

- **Language**: Go 1.25+
- **Database**: PostgreSQL 16+ with sqlc
- **Storage**: MinIO (S3-compatible)
- **Queue**: Redis Streams via job-queue library
- **Templates**: templ (type-safe HTML templates)
- **Frontend**: HTMX + Alpine.js + Tailwind CSS 4
- **Logging**: slog (structured logging)

### Architecture

```
Client → API Server → MinIO (storage)
                    → PostgreSQL (metadata)
                    → Redis (job queue)

Worker Pool ← Redis (job queue)
            → Processor Registry
            → MinIO (variants)
            → PostgreSQL (variant metadata)
```

## Quick Reference

### Project Structure

```
cmd/
  api/          # API server entry point
  worker/       # Background worker entry point
internal/
  api/          # REST API handlers
  web/          # Web UI handlers and templates
  auth/         # Authentication (sessions, OAuth, JWT)
  storage/      # MinIO client
  processor/    # File processors
  worker/       # Job handlers and payloads
  db/           # sqlc generated queries
  config/       # Configuration loading
  logger/       # Structured logging (slog)
  apperror/     # Error handling
sql/
  schema.sql    # Database schema
  queries/      # SQL queries for sqlc
docs/           # This directory
```

### Common Commands

```bash
task setup              # Install tools (sqlc, templ, tailwindcss)
task docker:up          # Start postgres, redis, minio
task migrate            # Run database migrations
task generate           # Generate code (sqlc, templ, css)
task run:api            # Start API server
task run:worker         # Start background worker
task test               # Run all tests
```

### Entry Points

- **API Server**: `cmd/api/main.go`
- **Worker Pool**: `cmd/worker/main.go`

### Key Packages

- `internal/worker/handlers.go` - Job processing logic
- `internal/processor/registry.go` - File processor registry
- `internal/api/routes.go` - API endpoints
- `internal/web/routes.go` - Web UI routes
- `internal/auth/middleware.go` - Authentication
- `internal/storage/minio.go` - Storage implementation

## Development Workflow

1. Make code changes
2. Run `task generate` if you modified SQL, templ, or CSS
3. Run `task test` to verify changes
4. Run `task fmt` before committing

## Need Help?

### Adding a Feature
1. Read ARCHITECTURE.md to understand where it fits
2. Check DEVELOPMENT.md for similar implementation patterns
3. Follow code style in AGENTS.md
4. Write tests

### Fixing a Bug
1. Write a failing test that reproduces the bug
2. Fix the code
3. Verify test passes
4. Check for similar issues in codebase

### Understanding Existing Code
1. Read ARCHITECTURE.md for high-level overview
2. Trace code from entry point (cmd/api or cmd/worker)
3. Check database schema in sql/schema.sql
4. Review tests for expected behavior

## Additional Resources

- **sqlc**: https://docs.sqlc.dev/
- **templ**: https://templ.guide/
- **job-queue library**: https://github.com/abdul-hamid-achik/job-queue
- **HTMX**: https://htmx.org/
- **Tailwind CSS**: https://tailwindcss.com/
- **Nord Theme**: https://www.nordtheme.com/
- **Stripe**: https://stripe.com/docs

## Contributing

See [../AGENTS.md](../AGENTS.md) for code style guidelines.

Key points:
- No unnecessary comments
- Self-documenting code with clear naming
- Table-driven tests
- No HTML comments in templ files
- Follow standard Go conventions
