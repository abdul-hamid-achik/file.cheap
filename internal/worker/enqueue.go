package worker

import (
	"context"
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type Broker interface {
	Enqueue(jobType string, payload interface{}) (string, error)
}

type JobPayload interface {
	SetJobID(id pgtype.UUID)
	GetFileID() pgtype.UUID
}

func (p *ThumbnailPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *ThumbnailPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *ThumbnailPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *ResizePayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *ResizePayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *ResizePayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *WebPPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *WebPPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *WebPPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *WatermarkPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *WatermarkPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *WatermarkPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *ConvertPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *ConvertPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *ConvertPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *PDFThumbnailPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *PDFThumbnailPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *PDFThumbnailPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *MetadataPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *MetadataPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *MetadataPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *OptimizePayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *OptimizePayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *OptimizePayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *VideoThumbnailPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *VideoThumbnailPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *VideoThumbnailPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *VideoTranscodePayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *VideoTranscodePayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *VideoTranscodePayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *VideoHLSPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *VideoHLSPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *VideoHLSPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

func (p *VideoWatermarkPayload) SetJobID(id pgtype.UUID) { p.JobID = id }
func (p *VideoWatermarkPayload) GetJobID() pgtype.UUID   { return p.JobID }
func (p *VideoWatermarkPayload) GetFileID() pgtype.UUID {
	return pgtype.UUID{Bytes: p.FileID, Valid: true}
}

type JobCreator interface {
	CreateJob(ctx context.Context, arg db.CreateJobParams) (db.ProcessingJob, error)
}

func EnqueueWithTracking(ctx context.Context, queries JobCreator, broker Broker, payload JobPayload, jobType db.JobType) (string, error) {
	job, err := queries.CreateJob(ctx, db.CreateJobParams{
		FileID:   payload.GetFileID(),
		JobType:  jobType,
		Priority: 0,
	})
	if err != nil {
		return "", fmt.Errorf("create job record: %w", err)
	}

	payload.SetJobID(job.ID)

	queueID, err := broker.Enqueue(string(jobType), payload)
	if err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}

	return queueID, nil
}
