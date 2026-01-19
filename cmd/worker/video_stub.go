//go:build workers_image

package main

import (
	"log/slog"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	fpworker "github.com/abdul-hamid-achik/file.cheap/internal/worker"
	jobqueueworker "github.com/abdul-hamid-achik/job-queue/pkg/worker"
)

func registerVideoProcessors(procRegistry *processor.Registry, log *slog.Logger) {
	log.Debug("video processors disabled (image-only worker)")
}

func registerVideoHandlers(registry *jobqueueworker.Registry, deps *fpworker.Dependencies) {
}
