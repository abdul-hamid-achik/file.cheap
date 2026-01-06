# Development Guide

## Common Development Tasks

### Adding a New Processor

1. Create processor implementation in `internal/processor/{type}/`
```go
package myprocessor

import (
    "context"
    "io"
)

type MyProcessor struct{}

func New() *MyProcessor {
    return &MyProcessor{}
}

func (p *MyProcessor) Process(ctx context.Context, input io.Reader, output io.Writer, params map[string]interface{}) error {
    // Implementation here
    return nil
}
```

2. Register in `internal/processor/registry.go`
```go
func NewRegistry() *Registry {
    r := &Registry{processors: make(map[string]Processor)}
    r.Register("thumbnail", thumbnail.New())
    r.Register("resize", resize.New())
    r.Register("myprocessor", myprocessor.New())  // Add here
    return r
}
```

3. Create job payload in `internal/worker/payloads.go`
```go
type MyProcessorPayload struct {
    FileID uuid.UUID `json:"file_id"`
    // Add parameters
}

func NewMyProcessorPayload(fileID uuid.UUID) MyProcessorPayload {
    return MyProcessorPayload{FileID: fileID}
}
```

4. Create job handler in `internal/worker/handlers.go`
```go
func MyProcessorHandler(deps *Dependencies) func(context.Context, *job.Job) error {
    return func(ctx context.Context, j *job.Job) error {
        // Implementation similar to ThumbnailHandler
    }
}
```

5. Register handler in `cmd/worker/main.go`
```go
registry.Register("myprocessor", worker.MyProcessorHandler(deps))
```

6. Enqueue job in `internal/api/routes.go` or wherever needed
```go
payload := worker.NewMyProcessorPayload(fileUUID)
jobID, err := cfg.Broker.Enqueue("myprocessor", payload)
```

### Adding a New API Endpoint

1. Define handler in `internal/api/routes.go` or new file
```go
func (cfg *Config) handleNewEndpoint(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    log := logger.FromContext(ctx)
    
    // Extract user ID from context (if auth required)
    userID := auth.UserIDFromContext(ctx)
    if userID == uuid.Nil {
        respondError(w, apperror.ErrUnauthorized)
        return
    }
    
    // Your logic here
    
    respondJSON(w, http.StatusOK, map[string]interface{}{
        "message": "success",
    })
}
```

2. Register route in `internal/api/routes.go`
```go
func (cfg *Config) routes() http.Handler {
    mux := http.NewServeMux()
    
    // Existing routes...
    
    mux.HandleFunc("GET /api/new-endpoint", cfg.handleNewEndpoint)
    
    return mux
}
```

3. Add authentication if needed
```go
// Use middleware in routes() or in handler
protected := auth.RequireAuth(cfg.handleNewEndpoint)
mux.HandleFunc("GET /api/new-endpoint", protected)
```

### Adding a Database Table

1. Create migration in `sql/schema.sql`
```sql
CREATE TABLE my_table (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_my_table_user_id ON my_table(user_id);
```

2. Create queries in `sql/queries/my_table.sql`
```sql
-- name: GetMyTable :one
SELECT * FROM my_table WHERE id = $1;

-- name: ListMyTablesByUserID :many
SELECT * FROM my_table WHERE user_id = $1 ORDER BY created_at DESC;

-- name: CreateMyTable :one
INSERT INTO my_table (user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: UpdateMyTable :one
UPDATE my_table SET name = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteMyTable :exec
DELETE FROM my_table WHERE id = $1;
```

3. Generate code
```bash
task sqlc
```

4. Use in handlers
```go
import "your-project/internal/db"

func (cfg *Config) handleCreateMyTable(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    userID := auth.UserIDFromContext(ctx)
    
    var req struct {
        Name string `json:"name"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, apperror.ErrBadRequest)
        return
    }
    
    item, err := cfg.Queries.CreateMyTable(ctx, db.CreateMyTableParams{
        UserID: userID,
        Name:   req.Name,
    })
    if err != nil {
        log.Error("failed to create item", "error", err)
        respondError(w, apperror.ErrInternal)
        return
    }
    
    respondJSON(w, http.StatusCreated, item)
}
```

### Adding a Templ Component

1. Create templ file in `internal/web/templates/components/`
```templ
package components

templ MyComponent(title string, content string) {
    <div class="bg-nord-1 rounded-lg p-4">
        <h3 class="text-nord-5 font-semibold mb-2">{title}</h3>
        <p class="text-nord-4">{content}</p>
    </div>
}
```

2. Generate Go code
```bash
task templ
```

3. Use in pages
```templ
package pages

import "your-project/internal/web/templates/components"

templ MyPage() {
    @components.MyComponent("Title", "Content")
}
```

### Adding OAuth Provider

1. Add credentials to `.env`
```env
NEWPROVIDER_CLIENT_ID=your_client_id
NEWPROVIDER_CLIENT_SECRET=your_client_secret
```

2. Add to config in `internal/config/config.go`
```go
type Config struct {
    // Existing fields...
    NewProviderClientID     string
    NewProviderClientSecret string
}

func Load() (*Config, error) {
    // Existing code...
    NewProviderClientID:     os.Getenv("NEWPROVIDER_CLIENT_ID"),
    NewProviderClientSecret: os.Getenv("NEWPROVIDER_CLIENT_SECRET"),
}
```

3. Implement in `internal/auth/oauth.go`
```go
func (c *Client) NewProviderLogin(w http.ResponseWriter, r *http.Request) {
    // OAuth flow implementation
}

func (c *Client) NewProviderCallback(w http.ResponseWriter, r *http.Request) {
    // Callback handling
}
```

4. Register routes in `internal/web/routes.go`
```go
mux.HandleFunc("GET /auth/newprovider", oauthClient.NewProviderLogin)
mux.HandleFunc("GET /auth/newprovider/callback", oauthClient.NewProviderCallback)
```

## Debugging

### Debug Logging
Set `LOG_LEVEL=debug` in `.env`
```bash
LOG_LEVEL=debug task run:api
```

### Inspect Redis Queue
```bash
docker exec -it file-processor-redis redis-cli
KEYS *
XINFO STREAM stream:default:medium
XRANGE stream:default:medium - +
```

### Check Database
```bash
docker exec -it file-processor-db psql -U postgres -d fileprocessor
\dt                    # List tables
\d files              # Describe table
SELECT * FROM files;  # Query data
```

### Check MinIO
Access MinIO Console: http://localhost:9001
- Username: minioadmin
- Password: minioadmin

Or use CLI:
```bash
docker exec -it file-processor-minio mc ls local/fileprocessor
```

### View Logs
```bash
# API logs
docker compose logs api -f

# Worker logs
docker compose logs worker -f

# All logs
docker compose logs -f
```

## Testing

### Run Specific Tests
```bash
# Single test
go test -v -run TestUploadFile ./internal/api

# Single package
go test -v ./internal/auth

# With coverage
go test -cover ./...

# Integration tests only
go test -v ./tests/integration
```

### Write Tests
```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "hello", "HELLO", false},
        {"empty input", "", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFunction() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("MyFunction() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Mock Dependencies
```go
type MockStorage struct {
    UploadFunc func(ctx context.Context, path string, reader io.Reader, contentType string, size int64) error
}

func (m *MockStorage) Upload(ctx context.Context, path string, reader io.Reader, contentType string, size int64) error {
    if m.UploadFunc != nil {
        return m.UploadFunc(ctx, path, reader, contentType, size)
    }
    return nil
}
```

## Code Generation

### After SQL Changes
```bash
task sqlc  # Generate from sql/queries/
```

### After Template Changes
```bash
task templ  # Generate from *.templ files
```

### After CSS Changes
```bash
task css  # Build Tailwind CSS
```

### All at Once
```bash
task generate  # Runs sqlc, templ, and css
```

## Database Migrations

### Apply Migrations
```bash
task migrate  # Run sql/schema.sql
```

### Reset Database
```bash
task docker:down -v  # Remove volumes
task docker:up       # Recreate database
task migrate         # Apply schema
```

## Troubleshooting

### Port Already in Use
```bash
lsof -i :8080
kill -9 <PID>
```

### Docker Issues
```bash
task docker:clean  # Remove containers and volumes
task docker:up     # Start fresh
```

### Go Module Issues
```bash
go clean -modcache
go mod tidy
go mod download
```

### Build Errors
```bash
task clean      # Remove build artifacts
task generate   # Regenerate code
task build      # Build fresh
```

## Performance Profiling

### CPU Profile
```go
import _ "net/http/pprof"

// Already enabled in cmd/api/main.go
```

Visit: http://localhost:8080/debug/pprof/

### Memory Profile
```bash
go tool pprof http://localhost:8080/debug/pprof/heap
```

### Trace
```bash
curl http://localhost:8080/debug/pprof/trace?seconds=10 > trace.out
go tool trace trace.out
```

## Environment Variables Reference

See `.env.example` for complete list and defaults.

Required:
- `DATABASE_URL`
- `REDIS_URL`
- `MINIO_ENDPOINT`
- `MINIO_ACCESS_KEY`
- `MINIO_SECRET_KEY`
- `JWT_SECRET`

Optional:
- `PORT` (default: 8080)
- `ENVIRONMENT` (development, production)
- `LOG_LEVEL` (debug, info, warn, error)
- `WORKER_CONCURRENCY` (default: 4)
- `JOB_TIMEOUT` (default: 5m)
- `MAX_RETRIES` (default: 3)
