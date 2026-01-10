//go:build !workers_basic
// +build !workers_basic

package main

import (
	"log/slog"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/video"
	fpworker "github.com/abdul-hamid-achik/file.cheap/internal/worker"
	jobqueueworker "github.com/abdul-hamid-achik/job-queue/pkg/worker"
)

func registerVideoProcessors(procRegistry *processor.Registry, log *slog.Logger) {
	videoThumbProc, err := video.NewThumbnailProcessor(nil)
	if err != nil {
		log.Warn("video thumbnail processor unavailable (ffmpeg not found)", "error", err)
	} else {
		procRegistry.Register("video_thumbnail", videoThumbProc)
	}

	videoTranscodeProc, err := video.NewFFmpegProcessor(nil)
	if err != nil {
		log.Warn("video transcode processor unavailable (ffmpeg not found)", "error", err)
	} else {
		procRegistry.Register("video_transcode", videoTranscodeProc)
	}
}

func registerVideoHandlers(registry *jobqueueworker.Registry, deps *fpworker.Dependencies) {
	_ = registry.Register("video_thumbnail", fpworker.VideoThumbnailHandler(deps))
	_ = registry.Register("video_transcode", fpworker.VideoTranscodeHandler(deps))
}
