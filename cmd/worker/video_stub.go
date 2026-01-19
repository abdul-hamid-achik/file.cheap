//go:build workers_image

package main

import (
	"log/slog"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	fpworker "github.com/abdul-hamid-achik/file.cheap/internal/worker"
	jobqueueworker "github.com/abdul-hamid-achik/job-queue/pkg/worker"
)

func registerVideoProcessors(_ *processor.Registry, _ *slog.Logger) {}

func registerVideoHandlers(_ *jobqueueworker.Registry, _ *fpworker.Dependencies) {}
