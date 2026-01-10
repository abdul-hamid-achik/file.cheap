package client

import (
	"context"
	"io"
	"time"
)

// ClientInterface defines all client operations for mocking in tests.
// The Client struct implements this interface.
type ClientInterface interface {
	// SetAPIKey updates the API key for authentication
	SetAPIKey(apiKey string)

	// File operations
	Upload(ctx context.Context, filePath string, transforms []string, wait bool) (*UploadResponse, error)
	UploadReader(ctx context.Context, r io.Reader, filename string, size int64, transforms []string, wait bool) (*UploadResponse, error)
	ListFiles(ctx context.Context, limit, offset int, status, search string) (*ListFilesResponse, error)
	GetFile(ctx context.Context, fileID string) (*File, error)
	DeleteFile(ctx context.Context, fileID string) error
	Download(ctx context.Context, fileID, variant string) (io.ReadCloser, string, error)

	// Transform operations
	Transform(ctx context.Context, fileID string, req *TransformRequest) (*TransformResponse, error)
	BatchTransform(ctx context.Context, req *BatchTransformRequest) (*BatchTransformResponse, error)
	GetBatchStatus(ctx context.Context, batchID string, includeItems bool) (*BatchStatusResponse, error)
	GetJobStatus(ctx context.Context, jobID string) (*JobStatus, error)

	// Sharing
	CreateShare(ctx context.Context, fileID string, expires string) (*ShareResponse, error)

	// Authentication
	DeviceAuth(ctx context.Context) (*DeviceAuthResponse, error)
	DeviceToken(ctx context.Context, deviceCode string) (*DeviceTokenResponse, error)

	// Polling helpers
	WaitForFile(ctx context.Context, fileID string, pollInterval time.Duration, timeout time.Duration) (*File, error)
	WaitForBatch(ctx context.Context, batchID string, pollInterval time.Duration, timeout time.Duration) (*BatchStatusResponse, error)
}

// Ensure Client implements ClientInterface at compile time
var _ ClientInterface = (*Client)(nil)
