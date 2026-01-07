package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/processor"
	"github.com/abdul-hamid-achik/file-processor/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func nowPgtype() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now(), Valid: true}
}

type MockQuerier struct {
	mu sync.RWMutex

	files    map[pgtype.UUID]db.File
	jobs     map[pgtype.UUID]db.ProcessingJob
	variants map[pgtype.UUID]db.FileVariant

	GetFileErr          error
	CreateVariantErr    error
	GetJobErr           error
	UpdateFileStatusErr error

	UpdateFileStatusCalls []db.UpdateFileStatusParams
}

func NewMockQuerier() *MockQuerier {
	return &MockQuerier{
		files:    make(map[pgtype.UUID]db.File),
		jobs:     make(map[pgtype.UUID]db.ProcessingJob),
		variants: make(map[pgtype.UUID]db.FileVariant),
	}
}

func (m *MockQuerier) AddFile(f db.File) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[f.ID] = f
}

func (m *MockQuerier) GetFile(ctx context.Context, id pgtype.UUID) (db.File, error) {
	if m.GetFileErr != nil {
		return db.File{}, m.GetFileErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	f, ok := m.files[id]
	if !ok {
		return db.File{}, errors.New("file not found")
	}
	return f, nil
}

func (m *MockQuerier) GetFileByStorageKey(ctx context.Context, storageKey string) (db.File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, f := range m.files {
		if f.StorageKey == storageKey {
			return f, nil
		}
	}
	return db.File{}, errors.New("file not found")
}

func (m *MockQuerier) ListFilesByUser(ctx context.Context, arg db.ListFilesByUserParams) ([]db.File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.File
	for _, f := range m.files {
		if f.UserID == arg.UserID && !f.DeletedAt.Valid {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *MockQuerier) CountFilesByUser(ctx context.Context, userID pgtype.UUID) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int64
	for _, f := range m.files {
		if f.UserID == userID && !f.DeletedAt.Valid {
			count++
		}
	}
	return count, nil
}

func (m *MockQuerier) CreateFile(ctx context.Context, arg db.CreateFileParams) (db.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	f := db.File{
		ID:          uuidToPgtype(uuid.New()),
		UserID:      arg.UserID,
		Filename:    arg.Filename,
		ContentType: arg.ContentType,
		SizeBytes:   arg.SizeBytes,
		StorageKey:  arg.StorageKey,
		Status:      arg.Status,
		CreatedAt:   nowPgtype(),
		UpdatedAt:   nowPgtype(),
	}
	m.files[f.ID] = f
	return f, nil
}

func (m *MockQuerier) UpdateFileStatus(ctx context.Context, arg db.UpdateFileStatusParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UpdateFileStatusCalls = append(m.UpdateFileStatusCalls, arg)

	if m.UpdateFileStatusErr != nil {
		return m.UpdateFileStatusErr
	}

	f, ok := m.files[arg.ID]
	if !ok {
		return errors.New("file not found")
	}
	f.Status = arg.Status
	f.UpdatedAt = nowPgtype()
	m.files[arg.ID] = f
	return nil
}

func (m *MockQuerier) SoftDeleteFile(ctx context.Context, id pgtype.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, ok := m.files[id]
	if !ok {
		return errors.New("file not found")
	}
	f.DeletedAt = nowPgtype()
	m.files[id] = f
	return nil
}

func (m *MockQuerier) GetJob(ctx context.Context, id pgtype.UUID) (db.ProcessingJob, error) {
	if m.GetJobErr != nil {
		return db.ProcessingJob{}, m.GetJobErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	j, ok := m.jobs[id]
	if !ok {
		return db.ProcessingJob{}, errors.New("job not found")
	}
	return j, nil
}

func (m *MockQuerier) ListPendingJobs(ctx context.Context, limit int32) ([]db.ProcessingJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.ProcessingJob
	for _, j := range m.jobs {
		if j.Status == db.JobStatusPending {
			result = append(result, j)
			if int32(len(result)) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *MockQuerier) ListJobsByFile(ctx context.Context, fileID pgtype.UUID) ([]db.ProcessingJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.ProcessingJob
	for _, j := range m.jobs {
		if j.FileID == fileID {
			result = append(result, j)
		}
	}
	return result, nil
}

func (m *MockQuerier) CreateJob(ctx context.Context, arg db.CreateJobParams) (db.ProcessingJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	j := db.ProcessingJob{
		ID:        uuidToPgtype(uuid.New()),
		FileID:    arg.FileID,
		JobType:   arg.JobType,
		Priority:  arg.Priority,
		Status:    db.JobStatusPending,
		Attempts:  0,
		CreatedAt: nowPgtype(),
	}
	m.jobs[j.ID] = j
	return j, nil
}

func (m *MockQuerier) MarkJobRunning(ctx context.Context, id pgtype.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	j.Status = db.JobStatusRunning
	j.StartedAt = nowPgtype()
	j.Attempts++
	m.jobs[id] = j
	return nil
}

func (m *MockQuerier) MarkJobCompleted(ctx context.Context, id pgtype.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	j.Status = db.JobStatusCompleted
	j.CompletedAt = nowPgtype()
	m.jobs[id] = j
	return nil
}

func (m *MockQuerier) MarkJobFailed(ctx context.Context, arg db.MarkJobFailedParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[arg.ID]
	if !ok {
		return errors.New("job not found")
	}
	j.Status = db.JobStatusFailed
	j.ErrorMessage = arg.ErrorMessage
	j.CompletedAt = nowPgtype()
	m.jobs[arg.ID] = j
	return nil
}

func (m *MockQuerier) GetVariant(ctx context.Context, id pgtype.UUID) (db.FileVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.variants[id]
	if !ok {
		return db.FileVariant{}, errors.New("variant not found")
	}
	return v, nil
}

func (m *MockQuerier) ListVariantsByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.FileVariant
	for _, v := range m.variants {
		if v.FileID == fileID {
			result = append(result, v)
		}
	}
	return result, nil
}

func (m *MockQuerier) CreateVariant(ctx context.Context, arg db.CreateVariantParams) (db.FileVariant, error) {
	if m.CreateVariantErr != nil {
		return db.FileVariant{}, m.CreateVariantErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	v := db.FileVariant{
		ID:          uuidToPgtype(uuid.New()),
		FileID:      arg.FileID,
		VariantType: db.VariantType(arg.VariantType),
		ContentType: arg.ContentType,
		SizeBytes:   arg.SizeBytes,
		StorageKey:  arg.StorageKey,
		Width:       arg.Width,
		Height:      arg.Height,
		CreatedAt:   nowPgtype(),
	}
	m.variants[v.ID] = v
	return v, nil
}

func (m *MockQuerier) DeleteVariantsByFile(ctx context.Context, fileID pgtype.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, v := range m.variants {
		if v.FileID == fileID {
			delete(m.variants, id)
		}
	}
	return nil
}

func (m *MockQuerier) GetCreatedVariants() []db.FileVariant {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.FileVariant
	for _, v := range m.variants {
		result = append(result, v)
	}
	return result
}

type MockStorage struct {
	*storage.MemoryStorage
	DownloadErr error
	UploadErr   error
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		MemoryStorage: storage.NewMemoryStorage(),
	}
}

func (m *MockStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.DownloadErr != nil {
		return nil, m.DownloadErr
	}
	return m.MemoryStorage.Download(ctx, key)
}

func (m *MockStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error {
	if m.UploadErr != nil {
		return m.UploadErr
	}
	return m.MemoryStorage.Upload(ctx, key, reader, contentType, size)
}

type MockProcessor struct {
	name           string
	supportedTypes []string
	ProcessFunc    func(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error)
	ProcessErr     error
}

func NewMockProcessor(name string, types ...string) *MockProcessor {
	return &MockProcessor{
		name:           name,
		supportedTypes: types,
	}
}

func (m *MockProcessor) Name() string             { return m.name }
func (m *MockProcessor) SupportedTypes() []string { return m.supportedTypes }

func (m *MockProcessor) Process(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
	if m.ProcessErr != nil {
		return nil, m.ProcessErr
	}
	if m.ProcessFunc != nil {
		return m.ProcessFunc(ctx, opts, input)
	}

	data := []byte("processed image data")
	return &processor.Result{
		Data:        bytes.NewReader(data),
		ContentType: "image/jpeg",
		Size:        int64(len(data)),
		Metadata: processor.ResultMetadata{
			Width:  opts.Width,
			Height: opts.Height,
		},
	}, nil
}

type MockJob struct {
	id      string
	jobType string
	payload []byte
}

func NewMockJob(jobType string, payload interface{}) *MockJob {
	var data []byte
	switch p := payload.(type) {
	case []byte:
		data = p
	}
	return &MockJob{
		id:      uuid.New().String(),
		jobType: jobType,
		payload: data,
	}
}

func (j *MockJob) ID() string   { return j.id }
func (j *MockJob) Type() string { return j.jobType }
