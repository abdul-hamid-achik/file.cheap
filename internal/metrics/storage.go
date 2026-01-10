package metrics

import (
	"context"
	"io"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
)

type InstrumentedStorage struct {
	storage.Storage
}

func NewInstrumentedStorage(s storage.Storage) *InstrumentedStorage {
	return &InstrumentedStorage{Storage: s}
}

func (s *InstrumentedStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error {
	start := time.Now()

	err := s.Storage.Upload(ctx, key, reader, contentType, size)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}

	StorageOperationsTotal.WithLabelValues("upload", status).Inc()
	StorageOperationDuration.WithLabelValues("upload").Observe(duration)
	if err == nil {
		StorageBytesTotal.WithLabelValues("upload").Add(float64(size))
	}

	return err
}

func (s *InstrumentedStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	start := time.Now()

	reader, err := s.Storage.Download(ctx, key)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}

	StorageOperationsTotal.WithLabelValues("download", status).Inc()
	StorageOperationDuration.WithLabelValues("download").Observe(duration)

	if err != nil {
		return nil, err
	}

	return &instrumentedReadCloser{ReadCloser: reader}, nil
}

func (s *InstrumentedStorage) Delete(ctx context.Context, key string) error {
	start := time.Now()

	err := s.Storage.Delete(ctx, key)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}

	StorageOperationsTotal.WithLabelValues("delete", status).Inc()
	StorageOperationDuration.WithLabelValues("delete").Observe(duration)

	return err
}

func (s *InstrumentedStorage) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()

	exists, err := s.Storage.Exists(ctx, key)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}

	StorageOperationsTotal.WithLabelValues("exists", status).Inc()
	StorageOperationDuration.WithLabelValues("exists").Observe(duration)

	return exists, err
}

type instrumentedReadCloser struct {
	io.ReadCloser
	bytesRead int64
}

func (r *instrumentedReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func (r *instrumentedReadCloser) Close() error {
	StorageBytesTotal.WithLabelValues("download").Add(float64(r.bytesRead))
	return r.ReadCloser.Close()
}
