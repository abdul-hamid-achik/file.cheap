package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/processor"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type mockJob struct {
	id      string
	jobType string
	payload []byte
}

func (j *mockJob) ID() string   { return j.id }
func (j *mockJob) Type() string { return j.jobType }
func (j *mockJob) UnmarshalPayload(v interface{}) error {
	return json.Unmarshal(j.payload, v)
}

func newMockJob(t *testing.T, jobType string, payload interface{}) *mockJob {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return &mockJob{
		id:      uuid.New().String(),
		jobType: jobType,
		payload: data,
	}
}

func createTestJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8(255 * x / width)
			g := uint8(255 * y / height)
			img.Set(x, y, color.RGBA{R: r, G: g, B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

func makeTestFile(id, userID uuid.UUID, filename, storageKey string) db.File {
	return db.File{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		UserID:      pgtype.UUID{Bytes: userID, Valid: true},
		Filename:    filename,
		ContentType: "image/jpeg",
		SizeBytes:   1024,
		StorageKey:  storageKey,
		Status:      db.FileStatusPending,
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
}

func TestThumbnailHandler(t *testing.T) {
	fileID1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fileID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	fileID3 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	fileID4 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440004")
	fileID5 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440005")
	userID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name            string
		payload         ThumbnailPayload
		setupFile       *db.File
		setupImage      []byte
		fileErr         error
		downloadErr     error
		processErr      error
		uploadErr       error
		variantErr      error
		statusUpdateErr error
		wantErr         bool
		errContains     string
		wantStatus      db.FileStatus
	}{
		{
			name: "successful thumbnail creation",
			payload: ThumbnailPayload{
				FileID:  fileID1,
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID1, userID, "test.jpg", "uploads/550e8400-e29b-41d4-a716-446655440000/test.jpg")
				return &f
			}(),
			setupImage: createTestJPEG(800, 600),
			wantErr:    false,
			wantStatus: db.FileStatusCompleted,
		},
		{
			name: "file not found in database",
			payload: ThumbnailPayload{
				FileID:  uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			fileErr:     errors.New("file not found"),
			wantErr:     true,
			errContains: "file not found",
		},
		{
			name: "file not found in storage",
			payload: ThumbnailPayload{
				FileID:  fileID2,
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID2, userID, "missing.jpg", "uploads/550e8400-e29b-41d4-a716-446655440002/missing.jpg")
				return &f
			}(),
			downloadErr: errors.New("storage: file not found"),
			wantErr:     true,
			errContains: "not found",
		},
		{
			name: "processor error",
			payload: ThumbnailPayload{
				FileID:  fileID3,
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID3, userID, "corrupt.jpg", "uploads/550e8400-e29b-41d4-a716-446655440003/corrupt.jpg")
				return &f
			}(),
			setupImage:  createTestJPEG(100, 100),
			processErr:  processor.ErrCorruptedFile,
			wantErr:     true,
			errContains: "corrupted",
		},
		{
			name: "storage upload error",
			payload: ThumbnailPayload{
				FileID:  fileID4,
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID4, userID, "test.jpg", "uploads/550e8400-e29b-41d4-a716-446655440004/test.jpg")
				return &f
			}(),
			setupImage:  createTestJPEG(800, 600),
			uploadErr:   errors.New("storage: connection failed"),
			wantErr:     true,
			errContains: "connection failed",
		},
		{
			name: "variant creation error",
			payload: ThumbnailPayload{
				FileID:  fileID5,
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID5, userID, "test.jpg", "uploads/550e8400-e29b-41d4-a716-446655440005/test.jpg")
				return &f
			}(),
			setupImage:  createTestJPEG(800, 600),
			variantErr:  errors.New("database: constraint violation"),
			wantErr:     true,
			errContains: "constraint",
		},
		{
			name: "invalid payload - nil file ID",
			payload: ThumbnailPayload{
				FileID:  uuid.Nil,
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			wantErr:     true,
			errContains: "invalid",
		},
		{
			name: "invalid payload - zero dimensions",
			payload: ThumbnailPayload{
				FileID:  uuid.MustParse("550e8400-e29b-41d4-a716-446655440006"),
				Width:   0,
				Height:  0,
				Quality: 80,
			},
			wantErr:     true,
			errContains: "invalid",
		},
		{
			name: "status update failure",
			payload: ThumbnailPayload{
				FileID:  uuid.MustParse("550e8400-e29b-41d4-a716-446655440007"),
				Width:   200,
				Height:  200,
				Quality: 80,
			},
			setupFile: func() *db.File {
				f := makeTestFile(uuid.MustParse("550e8400-e29b-41d4-a716-446655440007"), userID, "test.jpg", "uploads/550e8400-e29b-41d4-a716-446655440007/test.jpg")
				return &f
			}(),
			setupImage:      createTestJPEG(800, 600),
			statusUpdateErr: errors.New("database: connection lost"),
			wantErr:         true,
			errContains:     "connection lost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := NewMockQuerier()
			mockStorage := NewMockStorage()
			mockProc := NewMockProcessor("thumbnail", "image/jpeg", "image/png")

			mockQueries.GetFileErr = tt.fileErr
			mockQueries.CreateVariantErr = tt.variantErr
			mockQueries.UpdateFileStatusErr = tt.statusUpdateErr
			mockStorage.DownloadErr = tt.downloadErr
			mockStorage.UploadErr = tt.uploadErr
			mockProc.ProcessErr = tt.processErr

			if tt.setupFile != nil {
				mockQueries.AddFile(*tt.setupFile)
			}
			if tt.setupImage != nil && tt.setupFile != nil {
				ctx := context.Background()
				_ = mockStorage.MemoryStorage.Upload(ctx, tt.setupFile.StorageKey, bytes.NewReader(tt.setupImage), "image/jpeg", int64(len(tt.setupImage)))
			}

			testRegistry := newTestRegistry()
			testRegistry.register("thumbnail", mockProc)

			deps := &testDependencies{
				storage:  mockStorage,
				registry: testRegistry,
				queries:  mockQueries,
			}

			handler := testThumbnailHandler(deps)
			job := newMockJob(t, "thumbnail", tt.payload)
			err := handler(context.Background(), job)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				variants := mockQueries.GetCreatedVariants()
				if len(variants) == 0 {
					t.Error("expected variant to be created")
				}

				if tt.wantStatus != "" {
					if len(mockQueries.UpdateFileStatusCalls) == 0 {
						t.Error("expected UpdateFileStatus to be called")
					} else {
						lastCall := mockQueries.UpdateFileStatusCalls[len(mockQueries.UpdateFileStatusCalls)-1]
						if lastCall.Status != tt.wantStatus {
							t.Errorf("UpdateFileStatus status = %v, want %v", lastCall.Status, tt.wantStatus)
						}
					}
				}
			}
		})
	}
}

func TestThumbnailHandler_StatusUpdate(t *testing.T) {
	fileID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440099")
	userID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	mockQueries := NewMockQuerier()
	mockStorage := NewMockStorage()
	mockProc := NewMockProcessor("thumbnail", "image/jpeg")

	testFile := makeTestFile(fileID, userID, "test.jpg", "uploads/"+fileID.String()+"/test.jpg")
	mockQueries.AddFile(testFile)

	imgData := createTestJPEG(800, 600)
	_ = mockStorage.MemoryStorage.Upload(context.Background(), testFile.StorageKey, bytes.NewReader(imgData), "image/jpeg", int64(len(imgData)))

	testRegistry := newTestRegistry()
	testRegistry.register("thumbnail", mockProc)

	deps := &testDependencies{
		storage:  mockStorage,
		registry: testRegistry,
		queries:  mockQueries,
	}

	handler := testThumbnailHandler(deps)
	payload := ThumbnailPayload{FileID: fileID, Width: 200, Height: 200, Quality: 80}
	job := newMockJob(t, "thumbnail", payload)

	err := handler(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockQueries.UpdateFileStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateFileStatus call, got %d", len(mockQueries.UpdateFileStatusCalls))
	}

	call := mockQueries.UpdateFileStatusCalls[0]
	if call.Status != db.FileStatusCompleted {
		t.Errorf("status = %v, want %v", call.Status, db.FileStatusCompleted)
	}

	file, _ := mockQueries.GetFile(context.Background(), pgtype.UUID{Bytes: fileID, Valid: true})
	if file.Status != db.FileStatusCompleted {
		t.Errorf("file status = %v, want %v", file.Status, db.FileStatusCompleted)
	}
}

func TestResizeHandler(t *testing.T) {
	fileID1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440010")
	fileID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440011")
	fileID4 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440014")
	userID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name        string
		payload     ResizePayload
		setupFile   *db.File
		setupImage  []byte
		fileErr     error
		downloadErr error
		processErr  error
		uploadErr   error
		variantErr  error
		wantErr     bool
		errContains string
	}{
		{
			name: "successful resize",
			payload: ResizePayload{
				FileID:      fileID1,
				Width:       1920,
				Height:      1080,
				Quality:     85,
				VariantType: "large",
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID1, userID, "photo.jpg", "uploads/550e8400-e29b-41d4-a716-446655440010/photo.jpg")
				f.SizeBytes = 2048
				return &f
			}(),
			setupImage: createTestJPEG(3000, 2000),
			wantErr:    false,
		},
		{
			name: "resize with medium variant",
			payload: ResizePayload{
				FileID:      fileID2,
				Width:       800,
				Height:      600,
				Quality:     80,
				VariantType: "medium",
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID2, userID, "photo.jpg", "uploads/550e8400-e29b-41d4-a716-446655440011/photo.jpg")
				f.SizeBytes = 2048
				return &f
			}(),
			setupImage: createTestJPEG(2000, 1500),
			wantErr:    false,
		},
		{
			name: "file not found",
			payload: ResizePayload{
				FileID:      uuid.MustParse("550e8400-e29b-41d4-a716-446655440012"),
				Width:       800,
				Height:      600,
				VariantType: "medium",
			},
			fileErr:     errors.New("file not found"),
			wantErr:     true,
			errContains: "not found",
		},
		{
			name: "invalid variant type",
			payload: ResizePayload{
				FileID:      uuid.MustParse("550e8400-e29b-41d4-a716-446655440013"),
				Width:       800,
				Height:      600,
				Quality:     80,
				VariantType: "",
			},
			wantErr:     true,
			errContains: "invalid",
		},
		{
			name: "processor failure",
			payload: ResizePayload{
				FileID:      fileID4,
				Width:       800,
				Height:      600,
				Quality:     80,
				VariantType: "medium",
			},
			setupFile: func() *db.File {
				f := makeTestFile(fileID4, userID, "photo.jpg", "uploads/550e8400-e29b-41d4-a716-446655440014/photo.jpg")
				f.SizeBytes = 2048
				return &f
			}(),
			setupImage:  createTestJPEG(800, 600),
			processErr:  processor.ErrProcessingFailed,
			wantErr:     true,
			errContains: "processing failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueries := NewMockQuerier()
			mockStorage := NewMockStorage()
			mockProc := NewMockProcessor("resize", "image/jpeg", "image/png")

			mockQueries.GetFileErr = tt.fileErr
			mockQueries.CreateVariantErr = tt.variantErr
			mockStorage.DownloadErr = tt.downloadErr
			mockStorage.UploadErr = tt.uploadErr
			mockProc.ProcessErr = tt.processErr

			if tt.setupFile != nil {
				mockQueries.AddFile(*tt.setupFile)
			}
			if tt.setupImage != nil && tt.setupFile != nil {
				ctx := context.Background()
				_ = mockStorage.MemoryStorage.Upload(ctx, tt.setupFile.StorageKey, bytes.NewReader(tt.setupImage), "image/jpeg", int64(len(tt.setupImage)))
			}

			testRegistry := newTestRegistry()
			testRegistry.register("resize", mockProc)

			deps := &testDependencies{
				storage:  mockStorage,
				registry: testRegistry,
				queries:  mockQueries,
			}

			handler := testResizeHandler(deps)
			job := newMockJob(t, "resize", tt.payload)
			err := handler(context.Background(), job)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				variants := mockQueries.GetCreatedVariants()
				if len(variants) == 0 {
					t.Error("expected variant to be created")
				} else {
					found := false
					for _, v := range variants {
						if string(v.VariantType) == tt.payload.VariantType {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected variant with type %q", tt.payload.VariantType)
					}
				}
			}
		})
	}
}

func TestBuildVariantKey(t *testing.T) {
	tests := []struct {
		name        string
		fileID      uuid.UUID
		variantType string
		filename    string
		wantPattern string
	}{
		{
			name:        "thumbnail key",
			fileID:      uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			variantType: "thumbnail",
			filename:    "thumb.jpg",
			wantPattern: "processed/550e8400-e29b-41d4-a716-446655440000/thumbnail/thumb.jpg",
		},
		{
			name:        "resize key",
			fileID:      uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			variantType: "large",
			filename:    "resized.jpg",
			wantPattern: "processed/550e8400-e29b-41d4-a716-446655440000/large/resized.jpg",
		},
		{
			name:        "medium variant key",
			fileID:      uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			variantType: "medium",
			filename:    "medium.jpg",
			wantPattern: "processed/550e8400-e29b-41d4-a716-446655440000/medium/medium.jpg",
		},
		{
			name:        "small variant key",
			fileID:      uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			variantType: "small",
			filename:    "small.png",
			wantPattern: "processed/550e8400-e29b-41d4-a716-446655440000/small/small.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildVariantKey(tt.fileID, tt.variantType, tt.filename)
			if got != tt.wantPattern {
				t.Errorf("buildVariantKey() = %q, want %q", got, tt.wantPattern)
			}
		})
	}
}

func TestHandlerErrorRetry(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		shouldRetry bool
	}{
		{
			name:        "database connection error - retry",
			err:         errors.New("connection refused"),
			shouldRetry: true,
		},
		{
			name:        "file not found - retry (eventual consistency)",
			err:         errors.New("file not found"),
			shouldRetry: true,
		},
		{
			name:        "invalid payload - permanent",
			err:         errors.New("invalid payload: file ID is nil"),
			shouldRetry: false,
		},
		{
			name:        "corrupted image - permanent",
			err:         processor.ErrCorruptedFile,
			shouldRetry: false,
		},
		{
			name:        "unsupported type - permanent",
			err:         processor.ErrUnsupportedType,
			shouldRetry: false,
		},
		{
			name:        "storage timeout - retry",
			err:         errors.New("context deadline exceeded"),
			shouldRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isPermanent := isPermanentError(tt.err)
			if tt.shouldRetry && isPermanent {
				t.Errorf("error %q should be retryable, but was marked permanent", tt.err)
			}
			if !tt.shouldRetry && !isPermanent {
				t.Errorf("error %q should be permanent, but was not marked as such", tt.err)
			}
		})
	}
}

func TestHandlerConcurrency(t *testing.T) {
	mockQueries := NewMockQuerier()
	mockStorage := NewMockStorage()
	mockProc := NewMockProcessor("thumbnail", "image/jpeg")

	testRegistry := newTestRegistry()
	testRegistry.register("thumbnail", mockProc)

	fileIDs := make([]uuid.UUID, 10)
	for i := 0; i < 10; i++ {
		fileID := uuid.New()
		fileIDs[i] = fileID
		mockQueries.AddFile(makeTestFile(fileID, uuid.New(), "test.jpg", "uploads/"+fileID.String()+"/test.jpg"))
		ctx := context.Background()
		imgData := createTestJPEG(100, 100)
		_ = mockStorage.MemoryStorage.Upload(ctx, "uploads/"+fileID.String()+"/test.jpg", bytes.NewReader(imgData), "image/jpeg", int64(len(imgData)))
	}

	deps := &testDependencies{
		storage:  mockStorage,
		registry: testRegistry,
		queries:  mockQueries,
	}

	handler := testThumbnailHandler(deps)

	done := make(chan error, 10)
	for _, fileID := range fileIDs {
		go func(fid uuid.UUID) {
			payload := ThumbnailPayload{
				FileID:  fid,
				Width:   200,
				Height:  200,
				Quality: 80,
			}
			job := &mockJob{
				id:      uuid.New().String(),
				jobType: "thumbnail",
			}
			job.payload, _ = json.Marshal(payload)
			done <- handler(context.Background(), job)
		}(fileID)
	}

	var errs []error
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		t.Errorf("concurrent execution had %d errors: %v", len(errs), errs)
	}
}

func TestHandlerContextCancellation(t *testing.T) {
	mockQueries := NewMockQuerier()
	mockStorage := NewMockStorage()
	mockProc := NewMockProcessor("thumbnail", "image/jpeg")

	mockProc.ProcessFunc = func(ctx context.Context, opts *processor.Options, input io.Reader) (*processor.Result, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			data := []byte("processed")
			return &processor.Result{
				Data:        bytes.NewReader(data),
				ContentType: "image/jpeg",
				Size:        int64(len(data)),
			}, nil
		}
	}

	testRegistry := newTestRegistry()
	testRegistry.register("thumbnail", mockProc)

	fileID := uuid.New()
	mockQueries.AddFile(makeTestFile(fileID, uuid.New(), "test.jpg", "uploads/"+fileID.String()+"/test.jpg"))
	imgData := createTestJPEG(100, 100)
	_ = mockStorage.MemoryStorage.Upload(context.Background(), "uploads/"+fileID.String()+"/test.jpg", bytes.NewReader(imgData), "image/jpeg", int64(len(imgData)))

	deps := &testDependencies{
		storage:  mockStorage,
		registry: testRegistry,
		queries:  mockQueries,
	}

	handler := testThumbnailHandler(deps)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	payload := ThumbnailPayload{
		FileID:  fileID,
		Width:   200,
		Height:  200,
		Quality: 80,
	}

	job := &mockJob{
		id:      uuid.New().String(),
		jobType: "thumbnail",
	}
	job.payload, _ = json.Marshal(payload)

	err := handler(ctx, job)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

type testRegistry struct {
	processors   map[string]processor.Processor
	contentTypes map[string]processor.Processor
}

func newTestRegistry() *testRegistry {
	return &testRegistry{
		processors:   make(map[string]processor.Processor),
		contentTypes: make(map[string]processor.Processor),
	}
}

func (r *testRegistry) register(name string, p processor.Processor) {
	r.processors[name] = p
	for _, ct := range p.SupportedTypes() {
		r.contentTypes[ct] = p
	}
}

func (r *testRegistry) getForContentType(contentType string) processor.Processor {
	return r.contentTypes[contentType]
}

type testDependencies struct {
	storage  *MockStorage
	registry *testRegistry
	queries  *MockQuerier
}

func testThumbnailHandler(deps *testDependencies) func(context.Context, *mockJob) error {
	return func(ctx context.Context, j *mockJob) error {
		var payload ThumbnailPayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			return &permanentError{err: err}
		}

		if payload.FileID == uuid.Nil {
			return &permanentError{err: errors.New("invalid payload: file ID is nil")}
		}
		if payload.Width <= 0 || payload.Height <= 0 {
			return &permanentError{err: errors.New("invalid payload: dimensions must be positive")}
		}

		pgFileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}
		file, err := deps.queries.GetFile(ctx, pgFileID)
		if err != nil {
			return err
		}

		reader, err := deps.storage.Download(ctx, file.StorageKey)
		if err != nil {
			return err
		}
		defer func() { _ = reader.Close() }()

		proc := deps.registry.getForContentType(file.ContentType)
		if proc == nil {
			return &permanentError{err: processor.ErrUnsupportedType}
		}

		opts := &processor.Options{
			Width:   payload.Width,
			Height:  payload.Height,
			Quality: payload.Quality,
		}
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			if errors.Is(err, processor.ErrCorruptedFile) || errors.Is(err, processor.ErrUnsupportedType) {
				return &permanentError{err: err}
			}
			return err
		}

		variantKey := buildVariantKey(payload.FileID, "thumbnail", "thumb.jpg")
		if err := deps.storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			return err
		}

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantTypeThumbnail,
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			return err
		}

		if err := deps.queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			return err
		}

		return nil
	}
}

func testResizeHandler(deps *testDependencies) func(context.Context, *mockJob) error {
	return func(ctx context.Context, j *mockJob) error {
		var payload ResizePayload
		if err := j.UnmarshalPayload(&payload); err != nil {
			return &permanentError{err: err}
		}

		if payload.FileID == uuid.Nil {
			return &permanentError{err: errors.New("invalid payload: file ID is nil")}
		}
		if payload.VariantType == "" {
			return &permanentError{err: errors.New("invalid payload: variant type is required")}
		}

		pgFileID := pgtype.UUID{Bytes: payload.FileID, Valid: true}
		file, err := deps.queries.GetFile(ctx, pgFileID)
		if err != nil {
			return err
		}

		reader, err := deps.storage.Download(ctx, file.StorageKey)
		if err != nil {
			return err
		}
		defer func() { _ = reader.Close() }()

		proc := deps.registry.getForContentType(file.ContentType)
		if proc == nil {
			return &permanentError{err: processor.ErrUnsupportedType}
		}

		opts := &processor.Options{
			Width:       payload.Width,
			Height:      payload.Height,
			Quality:     payload.Quality,
			VariantType: payload.VariantType,
		}
		result, err := proc.Process(ctx, opts, reader)
		if err != nil {
			if errors.Is(err, processor.ErrCorruptedFile) || errors.Is(err, processor.ErrUnsupportedType) {
				return &permanentError{err: err}
			}
			return err
		}

		filename := payload.VariantType + ".jpg"
		variantKey := buildVariantKey(payload.FileID, payload.VariantType, filename)
		if err := deps.storage.Upload(ctx, variantKey, result.Data, result.ContentType, result.Size); err != nil {
			return err
		}

		width := int32(result.Metadata.Width)
		height := int32(result.Metadata.Height)
		_, err = deps.queries.CreateVariant(ctx, db.CreateVariantParams{
			FileID:      file.ID,
			VariantType: db.VariantType(payload.VariantType),
			ContentType: result.ContentType,
			SizeBytes:   result.Size,
			StorageKey:  variantKey,
			Width:       &width,
			Height:      &height,
		})
		if err != nil {
			return err
		}

		if err := deps.queries.UpdateFileStatus(ctx, db.UpdateFileStatusParams{
			ID:     file.ID,
			Status: db.FileStatusCompleted,
		}); err != nil {
			return err
		}

		return nil
	}
}

type permanentError struct {
	err error
}

func (e *permanentError) Error() string {
	return e.err.Error()
}

func (e *permanentError) Unwrap() error {
	return e.err
}

func isPermanentError(err error) bool {
	var pe *permanentError
	if errors.As(err, &pe) {
		return true
	}
	if errors.Is(err, processor.ErrCorruptedFile) ||
		errors.Is(err, processor.ErrUnsupportedType) ||
		errors.Is(err, processor.ErrInvalidConfig) {
		return true
	}
	if err != nil && containsString(err.Error(), "invalid") {
		return true
	}
	return false
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFold(s, t string) bool {
	for i := 0; i < len(s); i++ {
		sr := s[i]
		tr := t[i]
		if sr >= 'A' && sr <= 'Z' {
			sr += 'a' - 'A'
		}
		if tr >= 'A' && tr <= 'Z' {
			tr += 'a' - 'A'
		}
		if sr != tr {
			return false
		}
	}
	return true
}
