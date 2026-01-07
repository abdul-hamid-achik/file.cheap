package metrics

import (
	"context"
	"time"

	"github.com/abdul-hamid-achik/job-queue/pkg/job"
)

type JobHandler func(context.Context, *job.Job) error

func JobMetricsMiddleware(next JobHandler) JobHandler {
	return func(ctx context.Context, j *job.Job) error {
		start := time.Now()
		WorkerPoolActiveJobs.Inc()
		defer WorkerPoolActiveJobs.Dec()

		err := next(ctx, j)

		duration := time.Since(start).Seconds()
		status := "success"
		if err != nil {
			status = "error"
		}

		JobsProcessedTotal.WithLabelValues(j.Type, status).Inc()
		JobsProcessingDuration.WithLabelValues(j.Type, "total").Observe(duration)

		return err
	}
}
