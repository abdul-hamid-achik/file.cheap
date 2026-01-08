package metrics

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	latencyWindow     []int64
	latencyMu         sync.Mutex
	maxLatencyRecords = 1000
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

func HTTPMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		path := NormalizePath(r.URL.Path)

		HTTPRequestsInFlight.WithLabelValues(r.Method).Inc()
		defer HTTPRequestsInFlight.WithLabelValues(r.Method).Dec()

		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		durationMs := time.Since(start).Milliseconds()
		status := strconv.Itoa(rw.statusCode)

		HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, path, status).Observe(duration)
		HTTPResponseSize.WithLabelValues(r.Method, path, status).Observe(float64(rw.size))

		recordLatency(durationMs)
	})
}

func recordLatency(ms int64) {
	latencyMu.Lock()
	defer latencyMu.Unlock()

	latencyWindow = append(latencyWindow, ms)
	if len(latencyWindow) > maxLatencyRecords {
		latencyWindow = latencyWindow[len(latencyWindow)-maxLatencyRecords:]
	}
}

func GetLatencyP95() int64 {
	latencyMu.Lock()
	defer latencyMu.Unlock()

	if len(latencyWindow) == 0 {
		return 0
	}

	sorted := make([]int64, len(latencyWindow))
	copy(sorted, latencyWindow)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	p95Index := int(float64(len(sorted)) * 0.95)
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	return sorted[p95Index]
}

type RedisSetFunc func(ctx context.Context, key string, value interface{}, expiration time.Duration) error

func UpdateLatencyMetrics(ctx context.Context, setFunc RedisSetFunc) {
	if setFunc == nil {
		return
	}
	p95 := GetLatencyP95()
	_ = setFunc(ctx, "metrics:api_latency_p95", strconv.FormatInt(p95, 10), 5*time.Minute)
}
