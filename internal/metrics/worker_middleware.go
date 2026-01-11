package metrics

import (
	"time"
)

// PrometheusCollector implements the job-queue MetricsCollector interface
// using Prometheus metrics for job processing statistics.
type PrometheusCollector struct{}

// NewPrometheusCollector creates a new metrics collector for job processing.
func NewPrometheusCollector() *PrometheusCollector {
	return &PrometheusCollector{}
}

// JobStarted is called when a job begins processing.
func (c *PrometheusCollector) JobStarted(jobType, queue string) {
	WorkerPoolActiveJobs.Inc()
}

// JobCompleted is called when a job finishes successfully.
func (c *PrometheusCollector) JobCompleted(jobType, queue string, duration time.Duration) {
	WorkerPoolActiveJobs.Dec()
	JobsProcessedTotal.WithLabelValues(jobType, "success").Inc()
	JobsProcessingDuration.WithLabelValues(jobType, "total").Observe(duration.Seconds())
}

// JobFailed is called when a job fails permanently.
func (c *PrometheusCollector) JobFailed(jobType, queue string, duration time.Duration) {
	WorkerPoolActiveJobs.Dec()
	JobsProcessedTotal.WithLabelValues(jobType, "error").Inc()
	JobsProcessingDuration.WithLabelValues(jobType, "total").Observe(duration.Seconds())
}

// JobRetrying is called when a job is being retried.
func (c *PrometheusCollector) JobRetrying(jobType, queue string, attempt int) {
	JobsProcessedTotal.WithLabelValues(jobType, "retry").Inc()
}
