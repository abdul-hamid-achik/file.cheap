package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type StorageHealthChecker interface {
	HealthCheck(ctx context.Context) error
}

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
)

type ComponentHealth struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Latency int64  `json:"latency_ms"`
	Error   string `json:"error,omitempty"`
}

type HealthResponse struct {
	Status     Status            `json:"status"`
	Components []ComponentHealth `json:"components,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}

type Checker struct {
	pool    *pgxpool.Pool
	redis   *redis.Client
	storage StorageHealthChecker
}

func NewChecker(pool *pgxpool.Pool, redisClient *redis.Client) *Checker {
	return &Checker{pool: pool, redis: redisClient}
}

func (c *Checker) WithStorage(s StorageHealthChecker) *Checker {
	c.storage = s
	return c
}

func (c *Checker) CheckAll(ctx context.Context) HealthResponse {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	components := make([]ComponentHealth, 0, 3)
	mu := sync.Mutex{}

	if c.pool != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			comp := c.checkDatabase(ctx)
			mu.Lock()
			components = append(components, comp)
			mu.Unlock()
		}()
	}

	if c.redis != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			comp := c.checkRedis(ctx)
			mu.Lock()
			components = append(components, comp)
			mu.Unlock()
		}()
	}

	if c.storage != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			comp := c.checkStorage(ctx)
			mu.Lock()
			components = append(components, comp)
			mu.Unlock()
		}()
	}

	wg.Wait()

	status := StatusHealthy
	for _, comp := range components {
		if comp.Status == StatusUnhealthy {
			status = StatusUnhealthy
			break
		}
	}

	return HealthResponse{
		Status:     status,
		Components: components,
		Timestamp:  time.Now(),
	}
}

func (c *Checker) checkDatabase(ctx context.Context) ComponentHealth {
	start := time.Now()
	err := c.pool.Ping(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ComponentHealth{
			Name:    "database",
			Status:  StatusUnhealthy,
			Latency: latency,
			Error:   err.Error(),
		}
	}
	return ComponentHealth{
		Name:    "database",
		Status:  StatusHealthy,
		Latency: latency,
	}
}

func (c *Checker) checkRedis(ctx context.Context) ComponentHealth {
	start := time.Now()
	err := c.redis.Ping(ctx).Err()
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ComponentHealth{
			Name:    "redis",
			Status:  StatusUnhealthy,
			Latency: latency,
			Error:   err.Error(),
		}
	}
	return ComponentHealth{
		Name:    "redis",
		Status:  StatusHealthy,
		Latency: latency,
	}
}

func (c *Checker) checkStorage(ctx context.Context) ComponentHealth {
	start := time.Now()
	err := c.storage.HealthCheck(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ComponentHealth{
			Name:    "storage",
			Status:  StatusUnhealthy,
			Latency: latency,
			Error:   err.Error(),
		}
	}
	return ComponentHealth{
		Name:    "storage",
		Status:  StatusHealthy,
		Latency: latency,
	}
}

func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
}

func ReadinessHandler(checker *Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := checker.CheckAll(r.Context())

		w.Header().Set("Content-Type", "application/json")
		if resp.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func HealthHandler(checker *Checker) http.HandlerFunc {
	return ReadinessHandler(checker)
}
