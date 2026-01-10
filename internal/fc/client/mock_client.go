package client

import (
	"context"
	"io"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockClient is a mock implementation of ClientInterface for testing.
type MockClient struct {
	mock.Mock
}

func (m *MockClient) SetAPIKey(apiKey string) {
	m.Called(apiKey)
}

func (m *MockClient) Upload(ctx context.Context, filePath string, transforms []string, wait bool) (*UploadResponse, error) {
	args := m.Called(ctx, filePath, transforms, wait)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UploadResponse), args.Error(1)
}

func (m *MockClient) UploadReader(ctx context.Context, r io.Reader, filename string, size int64, transforms []string, wait bool) (*UploadResponse, error) {
	args := m.Called(ctx, r, filename, size, transforms, wait)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UploadResponse), args.Error(1)
}

func (m *MockClient) ListFiles(ctx context.Context, limit, offset int, status, search string) (*ListFilesResponse, error) {
	args := m.Called(ctx, limit, offset, status, search)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ListFilesResponse), args.Error(1)
}

func (m *MockClient) GetFile(ctx context.Context, fileID string) (*File, error) {
	args := m.Called(ctx, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*File), args.Error(1)
}

func (m *MockClient) DeleteFile(ctx context.Context, fileID string) error {
	args := m.Called(ctx, fileID)
	return args.Error(0)
}

func (m *MockClient) Download(ctx context.Context, fileID, variant string) (io.ReadCloser, string, error) {
	args := m.Called(ctx, fileID, variant)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(io.ReadCloser), args.String(1), args.Error(2)
}

func (m *MockClient) Transform(ctx context.Context, fileID string, req *TransformRequest) (*TransformResponse, error) {
	args := m.Called(ctx, fileID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*TransformResponse), args.Error(1)
}

func (m *MockClient) BatchTransform(ctx context.Context, req *BatchTransformRequest) (*BatchTransformResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BatchTransformResponse), args.Error(1)
}

func (m *MockClient) GetBatchStatus(ctx context.Context, batchID string, includeItems bool) (*BatchStatusResponse, error) {
	args := m.Called(ctx, batchID, includeItems)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BatchStatusResponse), args.Error(1)
}

func (m *MockClient) GetJobStatus(ctx context.Context, jobID string) (*JobStatus, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*JobStatus), args.Error(1)
}

func (m *MockClient) CreateShare(ctx context.Context, fileID string, expires string) (*ShareResponse, error) {
	args := m.Called(ctx, fileID, expires)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ShareResponse), args.Error(1)
}

func (m *MockClient) DeviceAuth(ctx context.Context) (*DeviceAuthResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DeviceAuthResponse), args.Error(1)
}

func (m *MockClient) DeviceToken(ctx context.Context, deviceCode string) (*DeviceTokenResponse, error) {
	args := m.Called(ctx, deviceCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DeviceTokenResponse), args.Error(1)
}

func (m *MockClient) WaitForFile(ctx context.Context, fileID string, pollInterval time.Duration, timeout time.Duration) (*File, error) {
	args := m.Called(ctx, fileID, pollInterval, timeout)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*File), args.Error(1)
}

func (m *MockClient) WaitForBatch(ctx context.Context, batchID string, pollInterval time.Duration, timeout time.Duration) (*BatchStatusResponse, error) {
	args := m.Called(ctx, batchID, pollInterval, timeout)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BatchStatusResponse), args.Error(1)
}

// Ensure MockClient implements ClientInterface
var _ ClientInterface = (*MockClient)(nil)
