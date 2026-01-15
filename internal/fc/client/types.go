package client

import "time"

type File struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	Variants    []Variant `json:"variants,omitempty"`
}

type Variant struct {
	ID          string `json:"id"`
	VariantType string `json:"variant_type"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type UploadResponse struct {
	ID       string            `json:"id"`
	Filename string            `json:"filename"`
	URL      string            `json:"url"`
	Status   string            `json:"status"`
	Variants map[string]string `json:"variants,omitempty"`
}

type ListFilesResponse struct {
	Files   []File `json:"files"`
	Total   int    `json:"total"`
	HasMore bool   `json:"has_more"`
}

type TransformRequest struct {
	Presets   []string `json:"presets,omitempty"`
	WebP      bool     `json:"webp,omitempty"`
	Quality   int      `json:"quality,omitempty"`
	Watermark string   `json:"watermark,omitempty"`
}

type TransformResponse struct {
	FileID string   `json:"file_id"`
	Jobs   []string `json:"jobs"`
}

type BatchTransformRequest struct {
	FileIDs   []string `json:"file_ids"`
	Presets   []string `json:"presets,omitempty"`
	WebP      bool     `json:"webp,omitempty"`
	Quality   int      `json:"quality,omitempty"`
	Watermark string   `json:"watermark,omitempty"`
}

type BatchTransformResponse struct {
	BatchID    string `json:"batch_id"`
	TotalFiles int    `json:"total_files"`
	TotalJobs  int    `json:"total_jobs"`
	Status     string `json:"status"`
	StatusURL  string `json:"status_url"`
}

type BatchStatusResponse struct {
	ID             string      `json:"id"`
	Status         string      `json:"status"`
	TotalFiles     int         `json:"total_files"`
	CompletedFiles int         `json:"completed_files"`
	FailedFiles    int         `json:"failed_files"`
	Presets        []string    `json:"presets,omitempty"`
	WebP           bool        `json:"webp,omitempty"`
	Quality        int         `json:"quality,omitempty"`
	Watermark      string      `json:"watermark,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	StartedAt      *time.Time  `json:"started_at,omitempty"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty"`
	Items          []BatchItem `json:"items,omitempty"`
}

type BatchItem struct {
	FileID       string   `json:"file_id"`
	Status       string   `json:"status"`
	JobIDs       []string `json:"job_ids,omitempty"`
	ErrorMessage string   `json:"error_message,omitempty"`
}

type JobStatus struct {
	ID           string     `json:"id"`
	FileID       string     `json:"file_id"`
	JobType      string     `json:"job_type"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type ShareResponse struct {
	ID        string     `json:"id"`
	Token     string     `json:"token"`
	ShareURL  string     `json:"share_url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type DeviceTokenResponse struct {
	APIKey           string `json:"api_key,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type UploadResult struct {
	File     string
	FileID   string
	URL      string
	Variants map[string]string
	Status   string
	Error    error
}

type UploadSummary struct {
	Uploaded   []UploadResult `json:"uploaded"`
	Failed     []UploadResult `json:"failed"`
	Total      int            `json:"total"`
	Successful int            `json:"successful"`
}

// Video types

type VideoTranscodeRequest struct {
	Resolutions []int   `json:"resolutions,omitempty"` // e.g., [360, 720, 1080]
	Format      string  `json:"format,omitempty"`      // mp4, webm
	Preset      string  `json:"preset,omitempty"`      // ultrafast, fast, medium, slow
	Thumbnail   bool    `json:"thumbnail,omitempty"`   // extract thumbnail
	ThumbnailAt float64 `json:"thumbnail_at,omitempty"` // timestamp as percentage (0.0-1.0) or negative for absolute seconds
}

type VideoTranscodeResponse struct {
	FileID string   `json:"file_id"`
	Jobs   []string `json:"jobs"`
}

type ChunkedUploadInitRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	TotalSize   int64  `json:"total_size"`
}

type ChunkedUploadInitResponse struct {
	UploadID    string `json:"upload_id"`
	ChunkSize   int64  `json:"chunk_size"`
	ChunksTotal int    `json:"chunks_total"`
}

type ChunkedUploadStatusResponse struct {
	UploadID     string `json:"upload_id"`
	Filename     string `json:"filename"`
	TotalSize    int64  `json:"total_size"`
	ChunksTotal  int    `json:"chunks_total"`
	ChunksLoaded int    `json:"chunks_loaded"`
	Complete     bool   `json:"complete"`
}

type ChunkedUploadChunkResponse struct {
	UploadID     string `json:"upload_id"`
	ChunkIndex   int    `json:"chunk_index"`
	ChunksLoaded int    `json:"chunks_loaded"`
	ChunksTotal  int    `json:"chunks_total"`
	Complete     bool   `json:"complete"`
	FileID       string `json:"file_id,omitempty"`
}
