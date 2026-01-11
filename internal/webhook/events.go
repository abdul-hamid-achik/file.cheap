package webhook

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	EventFileUploaded        = "file.uploaded"
	EventProcessingStarted   = "processing.started"
	EventProcessingCompleted = "processing.completed"
	EventProcessingFailed    = "processing.failed"
)

var ValidEventTypes = map[string]bool{
	EventFileUploaded:        true,
	EventProcessingStarted:   true,
	EventProcessingCompleted: true,
	EventProcessingFailed:    true,
}

type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

type FileUploadedData struct {
	FileID      string `json:"file_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type ProcessingStartedData struct {
	FileID  string `json:"file_id"`
	JobID   string `json:"job_id"`
	JobType string `json:"job_type"`
}

type ProcessingCompletedData struct {
	FileID      string `json:"file_id"`
	JobID       string `json:"job_id"`
	JobType     string `json:"job_type"`
	VariantKey  string `json:"variant_key,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
}

type ProcessingFailedData struct {
	FileID       string `json:"file_id"`
	JobID        string `json:"job_id"`
	JobType      string `json:"job_type"`
	ErrorMessage string `json:"error_message"`
}

func NewEvent(eventType string, data any) (*Event, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		CreatedAt: time.Now().UTC(),
		Data:      dataBytes,
	}, nil
}

func NewFileUploadedEvent(fileID, filename, contentType string, sizeBytes int64) (*Event, error) {
	return NewEvent(EventFileUploaded, FileUploadedData{
		FileID:      fileID,
		Filename:    filename,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
	})
}

func NewProcessingStartedEvent(fileID, jobID, jobType string) (*Event, error) {
	return NewEvent(EventProcessingStarted, ProcessingStartedData{
		FileID:  fileID,
		JobID:   jobID,
		JobType: jobType,
	})
}

func NewProcessingCompletedEvent(fileID, jobID, jobType, variantKey, contentType string, sizeBytes, durationMs int64) (*Event, error) {
	return NewEvent(EventProcessingCompleted, ProcessingCompletedData{
		FileID:      fileID,
		JobID:       jobID,
		JobType:     jobType,
		VariantKey:  variantKey,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
		DurationMs:  durationMs,
	})
}

func NewProcessingFailedEvent(fileID, jobID, jobType, errorMessage string) (*Event, error) {
	return NewEvent(EventProcessingFailed, ProcessingFailedData{
		FileID:       fileID,
		JobID:        jobID,
		JobType:      jobType,
		ErrorMessage: errorMessage,
	})
}

func (e *Event) Marshal() ([]byte, error) {
	return json.Marshal(e)
}
