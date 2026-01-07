package metrics

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var uuidRegex = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
		[]string{"method"},
	)

	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path", "status"},
	)

	AuthOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_operations_total",
			Help: "Total number of authentication operations",
		},
		[]string{"operation", "status"},
	)

	AuthSessionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "auth_sessions_active",
			Help: "Number of active user sessions",
		},
	)

	FileUploadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "file_uploads_total",
			Help: "Total number of file uploads",
		},
		[]string{"status"},
	)

	FileUploadBytes = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "file_upload_bytes",
			Help:    "Size of uploaded files in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 4, 10),
		},
	)

	FileUploadDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "file_upload_duration_seconds",
			Help:    "Duration of file uploads in seconds",
			Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
		},
	)

	FileDeletionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "file_deletions_total",
			Help: "Total number of file deletions",
		},
		[]string{"status"},
	)

	FilesStoredTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "files_stored_total",
			Help: "Total number of files stored",
		},
	)

	StorageOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_operations_total",
			Help: "Total number of storage operations",
		},
		[]string{"operation", "status"},
	)

	StorageOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "storage_operation_duration_seconds",
			Help:    "Duration of storage operations in seconds",
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		},
		[]string{"operation"},
	)

	StorageBytesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_bytes_total",
			Help: "Total bytes transferred to/from storage",
		},
		[]string{"operation"},
	)

	JobsEnqueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobs_enqueued_total",
			Help: "Total number of jobs enqueued",
		},
		[]string{"type"},
	)

	JobsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobs_processed_total",
			Help: "Total number of jobs processed",
		},
		[]string{"type", "status"},
	)

	JobsProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "jobs_processing_duration_seconds",
			Help:    "Duration of job processing in seconds",
			Buckets: []float64{.1, .5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		},
		[]string{"type", "stage"},
	)

	JobsInQueue = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "jobs_in_queue",
			Help: "Number of jobs currently in queue",
		},
		[]string{"queue"},
	)

	WorkerPoolActiveJobs = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "worker_pool_active_jobs",
			Help: "Number of jobs currently being processed by workers",
		},
	)

	WorkerPoolSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "worker_pool_size",
			Help: "Size of the worker pool",
		},
	)

	AppInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_info",
			Help: "Application information",
		},
		[]string{"version", "environment", "service"},
	)

	AppUp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "app_up",
			Help: "Application is up and running",
		},
	)
)

func NormalizePath(path string) string {
	return uuidRegex.ReplaceAllString(path, ":id")
}

func RecordAuthOperation(operation, status string) {
	AuthOperationsTotal.WithLabelValues(operation, status).Inc()
}

func RecordFileUpload(status string, sizeBytes int64, durationSeconds float64) {
	FileUploadsTotal.WithLabelValues(status).Inc()
	if status == "success" {
		FileUploadBytes.Observe(float64(sizeBytes))
		FileUploadDuration.Observe(durationSeconds)
	}
}

func RecordFileDeletion(status string) {
	FileDeletionsTotal.WithLabelValues(status).Inc()
}

func RecordJobEnqueued(jobType string) {
	JobsEnqueuedTotal.WithLabelValues(jobType).Inc()
}

func RecordJobProcessed(jobType, status string, durationSeconds float64) {
	JobsProcessedTotal.WithLabelValues(jobType, status).Inc()
	JobsProcessingDuration.WithLabelValues(jobType, "total").Observe(durationSeconds)
}

func RecordJobStage(jobType, stage string, durationSeconds float64) {
	JobsProcessingDuration.WithLabelValues(jobType, stage).Observe(durationSeconds)
}

func SetAppInfo(version, environment, service string) {
	AppInfo.WithLabelValues(version, environment, service).Set(1)
	AppUp.Set(1)
}

func SetWorkerPoolSize(size int) {
	WorkerPoolSize.Set(float64(size))
}

func SetJobsInQueue(queue string, count int64) {
	JobsInQueue.WithLabelValues(queue).Set(float64(count))
}
