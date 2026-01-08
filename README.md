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

| Document | Description |
|----------|-------------|
| [ARCHITECTURE.md](./docs/ARCHITECTURE.md) | System architecture, components, and data flow |
| [API.md](./docs/API.md) | REST API reference and endpoint documentation |
| [DEVELOPMENT.md](./docs/DEVELOPMENT.md) | Development guide and common tasks |
| [DEPLOYMENT.md](./docs/DEPLOYMENT.md) | Production deployment with Kubernetes |
| [MONITORING.md](./docs/MONITORING.md) | Prometheus, Grafana, and observability setup |
| [AGENTS.md](./AGENTS.md) | Code style guidelines for AI agents |

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
