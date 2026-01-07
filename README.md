# file.cheap

File processing that doesn't cost a fortune. A production-ready Go application for image and document processing.

## What You'll Learn

- Go interfaces and design patterns
- Object storage with MinIO (S3-compatible)
- Type-safe SQL with sqlc and pgx
- Background job processing with Redis Streams
- Image processing with the imaging library
- HTTP APIs with Go 1.22+ net/http patterns
- Modern frontend with templ + htmx + Tailwind 4

## Quick Start

```bash
# Install tools
brew install go-task sqlc templ tailwindcss
asdf install  # or install Go 1.25.5 manually

# Setup project
task setup
task deps

# Start infrastructure
task docker:up

# Run tests
task test
```

## Project Structure

```
file-processor/
├── cmd/api/          # API server entry point
├── cmd/worker/       # Background worker entry point
├── internal/         # Application code (you implement this)
├── sql/              # SQL schema and queries (you write this)
├── docs/             # Learning documentation
├── testdata/         # Test fixtures
└── Taskfile.yml      # Task runner commands
```

## Documentation

Start with the [Learning Path](./docs/README.md) for a guided walkthrough.

| Phase | Topic |
|-------|-------|
| 0 | [Getting Started](./docs/00-getting-started.md) |
| 1 | [Storage Layer](./docs/01-phase1-storage.md) |
| 2 | [Database Layer](./docs/02-phase2-database.md) |
| 3 | [Queue Integration](./docs/03-phase3-queue.md) |
| 4 | [Image Processing](./docs/04-phase4-processing.md) |
| 5 | [Worker Layer](./docs/05-phase5-worker.md) |
| 6 | [API Layer](./docs/06-phase6-api.md) |
| 7 | [Frontend](./docs/07-phase7-frontend.md) |

## Tech Stack

- **Language:** Go 1.25
- **Database:** PostgreSQL 17
- **Cache/Queue:** Redis 7
- **Object Storage:** MinIO
- **SQL Codegen:** sqlc
- **Templates:** templ
- **Frontend:** htmx + Tailwind 4
- **Task Runner:** Task

## License

MIT
