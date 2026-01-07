# Monitoring Guide

This guide covers the monitoring and observability setup for the file.cheap application.

## Overview

The monitoring stack consists of:

- **Prometheus** - Metrics collection and storage
- **Grafana** - Visualization and dashboards
- **Loki** - Log aggregation
- **Promtail** - Log shipping from containers

## Quick Start

```bash
# Start monitoring stack
task monitoring:up

# Or start everything
task docker:up
```

## Access

| Service | URL | Credentials |
|---------|-----|-------------|
| Grafana | http://localhost:3000 | admin / admin |
| Prometheus | http://localhost:9090 | - |
| API Metrics | http://localhost:8080/metrics | - |
| Worker Metrics | http://localhost:9091/metrics | - |

## Dashboards

Four pre-configured dashboards are available in Grafana:

### 1. Application Overview

Main dashboard showing:
- Service health status
- Request rate and error rate
- Request duration percentiles (p50, p95, p99)
- File uploads and storage throughput
- Recent error logs

### 2. Worker Performance

Background job processing metrics:
- Queue depth and active jobs
- Job throughput by type
- Job duration by type and stage
- Job success/failure rates
- Worker error logs

### 3. System Health

Infrastructure health:
- Service status (up/down)
- Storage operations and latency
- In-flight requests
- Error and warning logs

### 4. User Activity

User-facing metrics:
- Active sessions
- Login/registration activity
- File uploads and deletions
- Upload file size distribution
- User activity logs

## Metrics Reference

### HTTP Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `http_requests_total` | Counter | method, path, status | Total HTTP requests |
| `http_request_duration_seconds` | Histogram | method, path, status | Request duration |
| `http_requests_in_flight` | Gauge | method | Currently processing requests |
| `http_response_size_bytes` | Histogram | method, path, status | Response size |

### Authentication Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `auth_operations_total` | Counter | operation, status | Auth operations (login, register, etc.) |
| `auth_sessions_active` | Gauge | - | Active user sessions |

### File Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `file_uploads_total` | Counter | status | File upload attempts |
| `file_upload_bytes` | Histogram | - | Upload file sizes |
| `file_upload_duration_seconds` | Histogram | - | Upload duration |
| `file_deletions_total` | Counter | status | File deletions |

### Storage Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `storage_operations_total` | Counter | operation, status | Storage operations |
| `storage_operation_duration_seconds` | Histogram | operation | Operation duration |
| `storage_bytes_total` | Counter | operation | Bytes transferred |

### Job Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `jobs_enqueued_total` | Counter | type | Jobs added to queue |
| `jobs_processed_total` | Counter | type, status | Jobs processed |
| `jobs_processing_duration_seconds` | Histogram | type, stage | Processing duration |
| `jobs_in_queue` | Gauge | queue | Pending jobs |
| `worker_pool_active_jobs` | Gauge | - | Currently processing |
| `worker_pool_size` | Gauge | - | Worker pool size |

### Application Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `app_info` | Gauge | version, environment, service | Application info |
| `app_up` | Gauge | - | Application health |

## Common Queries

### Request Rate

```promql
sum(rate(http_requests_total[5m]))
```

### Error Rate Percentage

```promql
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100
```

### P95 Request Duration

```promql
histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))
```

### Jobs per Second by Type

```promql
sum by (type) (rate(jobs_processed_total[5m]))
```

### Job Queue Depth

```promql
sum(jobs_in_queue)
```

### Storage Throughput

```promql
sum by (operation) (rate(storage_bytes_total[5m]))
```

## Log Queries (Loki)

### All Errors

```logql
{service=~"api|worker"} |= "error" | json
```

### Login Failures

```logql
{service="api"} |= "login" |= "error" | json
```

### Job Processing

```logql
{service="worker"} |= "job" | json
```

### Slow Requests (>1s)

```logql
{service="api"} | json | duration_ms > 1000
```

## Alerting

Alerts are configured in Grafana and will appear in the Alerting section. Current alerts:

### Critical

- **High Error Rate** - >5% HTTP errors in 5 minutes
- **Service Down** - No metrics for 2 minutes
- **Job Queue Backup** - >100 pending jobs for 5 minutes
- **High Job Failure Rate** - >10% job failures in 10 minutes

### Warning

- **Elevated Error Rate** - >2% HTTP errors in 10 minutes
- **Slow Requests** - p95 latency >2 seconds
- **Queue Growing** - Queue depth increasing rapidly

## Troubleshooting

### No Metrics in Grafana

1. Check Prometheus is running: `docker compose ps prometheus`
2. Verify targets: http://localhost:9090/targets
3. Check API metrics: `curl http://localhost:8080/metrics`
4. Check datasource in Grafana: Settings > Data Sources > Prometheus

### No Logs in Grafana

1. Check Loki is running: `docker compose ps loki`
2. Check Promtail: `docker compose logs promtail`
3. Verify Loki datasource in Grafana

### High Memory Usage

Prometheus stores metrics locally. To reduce retention:

```yaml
# prometheus/prometheus.yml
storage:
  tsdb:
    retention.time: 7d  # Reduce from 15d
```

### Missing Dashboard Panels

1. Check datasource UIDs match in dashboard JSON
2. Refresh dashboard (top right menu > Refresh)
3. Check time range selector

## Adding Custom Metrics

To add new metrics:

1. Define in `internal/metrics/metrics.go`:

```go
var MyCustomMetric = promauto.NewCounter(
    prometheus.CounterOpts{
        Name: "my_custom_metric_total",
        Help: "Description of the metric",
    },
)
```

2. Record metrics where needed:

```go
metrics.MyCustomMetric.Inc()
```

3. Add to dashboards via Grafana UI or update JSON files.

## Architecture

```
┌─────────────┐    scrape    ┌────────────┐
│    API      │◄─────────────│ Prometheus │
│  :8080      │              │   :9090    │
└─────────────┘              └─────┬──────┘
                                   │
┌─────────────┐    scrape          │    query
│   Worker    │◄───────────────────┤◄──────────┐
│   :9090     │                    │           │
└─────────────┘              ┌─────▼──────┐    │
                             │  Grafana   │────┘
┌─────────────┐    push      │   :3000    │
│  Promtail   │────────────► │            │◄───┐
└─────────────┘              └────────────┘    │
       │                                       │
       │ scrape logs         ┌────────────┐    │
       └────────────────────►│    Loki    │────┘
                             │   :3100    │ query
                             └────────────┘
```
