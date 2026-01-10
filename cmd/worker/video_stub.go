//go:build workers_image
// +build workers_image

package main

import (
	"log/slog"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	fpworker "github.com/abdul-hamid-achik/file.cheap/internal/worker"
	jobqueueworker "github.com/abdul-hamid-achik/job-queue/pkg/worker"
)

func registerVideoProcessorsStub(procRegistry *processor.Registry, log *slog.Logger) {
	log.Info("video processors disabled (workers_basic build tag)")
}

func registerVideoHandlersStub(registry *jobqueueworker.Registry, deps *fpworker.Dependencies) {
}
