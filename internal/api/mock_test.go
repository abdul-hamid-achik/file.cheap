package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/abdul-hamid-achik/file-processor/internal/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type MockBroker struct {
	mu          sync.Mutex
	jobs        []EnqueuedJob
	EnqueueErr  error
	EnqueueFunc func(jobType string, payload interface{}) (string, error)
}

type EnqueuedJob struct {
	ID      string
	Type    string
	Payload interface{}
}

func NewMockBroker() *MockBroker {
	return &MockBroker{
		jobs: make([]EnqueuedJob, 0),
	}
}

func (m *MockBroker) Enqueue(jobType string, payload interface{}) (string, error) {
	if m.EnqueueErr != nil {
		return "", m.EnqueueErr
	}
	if m.EnqueueFunc != nil {
		return m.EnqueueFunc(jobType, payload)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	m.jobs = append(m.jobs, EnqueuedJob{
		ID:      id,
		Type:    jobType,
		Payload: payload,
	})
	return id, nil
}

func (m *MockBroker) HasJob(jobType string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Type == jobType {
			return true
		}
	}
	return false
}

type MockQuerier struct {
	mu sync.RWMutex

	files    map[string]db.File
	variants map[string]db.FileVariant

	GetFileErr        error
	ListFilesErr      error
	CreateFileErr     error
	SoftDeleteFileErr error
	ListVariantsErr   error
	CountFilesResult  int64

	BillingTier db.SubscriptionTier
}

func NewMockQuerier() *MockQuerier {
	return &MockQuerier{
		files:       make(map[string]db.File),
		variants:    make(map[string]db.FileVariant),
		BillingTier: db.SubscriptionTierPro, // Default to Pro for existing tests
	}
}

func (m *MockQuerier) AddFile(f db.File) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := uuidToString(f.ID)
	m.files[key] = f
}

func (m *MockQuerier) AddVariant(v db.FileVariant) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := uuidToString(v.ID)
	m.variants[key] = v
}

func (m *MockQuerier) GetFile(ctx context.Context, id pgtype.UUID) (db.File, error) {
	if m.GetFileErr != nil {
		return db.File{}, m.GetFileErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := uuidToString(id)
	f, ok := m.files[key]
	if !ok || f.DeletedAt.Valid {
		return db.File{}, errors.New("file not found")
	}
	return f, nil
}

func (m *MockQuerier) ListFilesByUser(ctx context.Context, arg db.ListFilesByUserParams) ([]db.File, error) {
	if m.ListFilesErr != nil {
		return nil, m.ListFilesErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.File
	userKey := uuidToString(arg.UserID)
	for _, f := range m.files {
		if uuidToString(f.UserID) == userKey && !f.DeletedAt.Valid {
			result = append(result, f)
		}
	}

	start := int(arg.Offset)
	if start >= len(result) {
		return []db.File{}, nil
	}
	end := start + int(arg.Limit)
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], nil
}

func (m *MockQuerier) CountFilesByUser(ctx context.Context, userID pgtype.UUID) (int64, error) {
	if m.CountFilesResult > 0 {
		return m.CountFilesResult, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int64
	userKey := uuidToString(userID)
	for _, f := range m.files {
		if uuidToString(f.UserID) == userKey && !f.DeletedAt.Valid {
			count++
		}
	}
	return count, nil
}

func (m *MockQuerier) CreateFile(ctx context.Context, arg db.CreateFileParams) (db.File, error) {
	if m.CreateFileErr != nil {
		return db.File{}, m.CreateFileErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}

	now := time.Now()
	createdAt := pgtype.Timestamptz{Time: now, Valid: true}
	updatedAt := pgtype.Timestamptz{Time: now, Valid: true}

	f := db.File{
		ID:          pgID,
		UserID:      arg.UserID,
		Filename:    arg.Filename,
		ContentType: arg.ContentType,
		SizeBytes:   arg.SizeBytes,
		StorageKey:  arg.StorageKey,
		Status:      arg.Status,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
	m.files[id.String()] = f
	return f, nil
}

func (m *MockQuerier) SoftDeleteFile(ctx context.Context, id pgtype.UUID) error {
	if m.SoftDeleteFileErr != nil {
		return m.SoftDeleteFileErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := uuidToString(id)
	f, ok := m.files[key]
	if !ok {
		return errors.New("file not found")
	}

	f.DeletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	m.files[key] = f
	return nil
}

func (m *MockQuerier) ListVariantsByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileVariant, error) {
	if m.ListVariantsErr != nil {
		return nil, m.ListVariantsErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	fileKey := uuidToString(fileID)
	var result []db.FileVariant
	for _, v := range m.variants {
		if uuidToString(v.FileID) == fileKey {
			result = append(result, v)
		}
	}
	return result, nil
}

func (m *MockQuerier) GetAPITokenByHash(ctx context.Context, tokenHash string) (db.GetAPITokenByHashRow, error) {
	return db.GetAPITokenByHashRow{}, errors.New("not implemented in mock")
}

func (m *MockQuerier) UpdateAPITokenLastUsed(ctx context.Context, id pgtype.UUID) error {
	return nil
}

func (m *MockQuerier) GetFileShareByToken(ctx context.Context, token string) (db.GetFileShareByTokenRow, error) {
	return db.GetFileShareByTokenRow{}, errors.New("not implemented in mock")
}

func (m *MockQuerier) IncrementShareAccessCount(ctx context.Context, id pgtype.UUID) error {
	return nil
}

func (m *MockQuerier) GetTransformCache(ctx context.Context, arg db.GetTransformCacheParams) (db.TransformCache, error) {
	return db.TransformCache{}, errors.New("not implemented in mock")
}

func (m *MockQuerier) CreateTransformCache(ctx context.Context, arg db.CreateTransformCacheParams) (db.TransformCache, error) {
	return db.TransformCache{}, errors.New("not implemented in mock")
}

func (m *MockQuerier) IncrementTransformCacheCount(ctx context.Context, arg db.IncrementTransformCacheCountParams) error {
	return nil
}

func (m *MockQuerier) GetTransformRequestCount(ctx context.Context, arg db.GetTransformRequestCountParams) (int32, error) {
	return 0, nil
}

func (m *MockQuerier) CreateFileShare(ctx context.Context, arg db.CreateFileShareParams) (db.FileShare, error) {
	return db.FileShare{}, errors.New("not implemented in mock")
}

func (m *MockQuerier) ListFileSharesByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileShare, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *MockQuerier) DeleteFileShare(ctx context.Context, arg db.DeleteFileShareParams) error {
	return errors.New("not implemented in mock")
}

func (m *MockQuerier) GetUserBillingInfo(ctx context.Context, id pgtype.UUID) (db.GetUserBillingInfoRow, error) {
	tier := m.BillingTier
	if tier == "" {
		tier = db.SubscriptionTierPro
	}

	var filesLimit int32
	var maxFileSize int64
	var status db.SubscriptionStatus

	switch tier {
	case db.SubscriptionTierPro, db.SubscriptionTierEnterprise:
		filesLimit = 2000
		maxFileSize = 100 * 1024 * 1024 // 100 MB
		status = db.SubscriptionStatusActive
	default:
		filesLimit = 100
		maxFileSize = 10 * 1024 * 1024 // 10 MB
		status = db.SubscriptionStatusNone
	}

	return db.GetUserBillingInfoRow{
		SubscriptionTier:   tier,
		SubscriptionStatus: status,
		FilesLimit:         filesLimit,
		MaxFileSize:        maxFileSize,
	}, nil
}

func (m *MockQuerier) GetUserFilesCount(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return m.CountFilesByUser(ctx, userID)
}

func (m *MockQuerier) GetAllFiles() []db.File {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]db.File, 0, len(m.files))
	for _, f := range m.files {
		result = append(result, f)
	}
	return result
}

var _ Querier = (*MockQuerier)(nil)

type MockStorage struct {
	*storage.MemoryStorage
	UploadErr      error
	DownloadErr    error
	DeleteErr      error
	PresignedURLFn func(key string, expiry int) (string, error)
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		MemoryStorage: storage.NewMemoryStorage(),
	}
}

func (m *MockStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string, size int64) error {
	if m.UploadErr != nil {
		return m.UploadErr
	}
	return m.MemoryStorage.Upload(ctx, key, reader, contentType, size)
}

func (m *MockStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.DownloadErr != nil {
		return nil, m.DownloadErr
	}
	return m.MemoryStorage.Download(ctx, key)
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	return m.MemoryStorage.Delete(ctx, key)
}

func (m *MockStorage) GetPresignedURL(ctx context.Context, key string, expirySeconds int) (string, error) {
	if m.PresignedURLFn != nil {
		return m.PresignedURLFn(key, expirySeconds)
	}
	return "https://storage.example.com/" + key + "?token=test", nil
}

const testJWTSecret = "test-secret-key-for-testing-only"

func generateTestToken(t *testing.T, userID uuid.UUID, expiry time.Duration) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": userID.String(),
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(expiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}

	return tokenString
}

func generateExpiredToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": userID.String(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}

	return tokenString
}

type TestConfig struct {
	JWTSecret     string
	MaxUploadSize int64
}

func DefaultTestConfig() TestConfig {
	return TestConfig{
		JWTSecret:     testJWTSecret,
		MaxUploadSize: 100 * 1024 * 1024,
	}
}

func setupTestDeps(t *testing.T) (*MockQuerier, *MockStorage, *MockBroker, TestConfig) {
	t.Helper()
	return NewMockQuerier(), NewMockStorage(), NewMockBroker(), DefaultTestConfig()
}

func createTestFile(userID uuid.UUID, filename string) db.File {
	fileID := uuid.New()
	pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	now := time.Now()
	createdAt := pgtype.Timestamptz{Time: now, Valid: true}
	updatedAt := pgtype.Timestamptz{Time: now, Valid: true}

	return db.File{
		ID:          pgFileID,
		UserID:      pgUserID,
		Filename:    filename,
		ContentType: "image/jpeg",
		SizeBytes:   1024,
		StorageKey:  "uploads/" + fileID.String() + "/" + filename,
		Status:      db.FileStatusPending,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func createTestFileWithID(id, userID uuid.UUID, filename string) db.File {
	pgFileID := pgtype.UUID{Bytes: id, Valid: true}
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	now := time.Now()
	createdAt := pgtype.Timestamptz{Time: now, Valid: true}
	updatedAt := pgtype.Timestamptz{Time: now, Valid: true}

	return db.File{
		ID:          pgFileID,
		UserID:      pgUserID,
		Filename:    filename,
		ContentType: "image/jpeg",
		SizeBytes:   1024,
		StorageKey:  "uploads/" + id.String() + "/" + filename,
		Status:      db.FileStatusPending,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func createTestVariant(fileID uuid.UUID, variantType string) db.FileVariant {
	variantID := uuid.New()
	pgVariantID := pgtype.UUID{Bytes: variantID, Valid: true}
	pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}

	now := time.Now()
	createdAt := pgtype.Timestamptz{Time: now, Valid: true}

	width := int32(200)
	height := int32(200)
	return db.FileVariant{
		ID:          pgVariantID,
		FileID:      pgFileID,
		VariantType: db.VariantType(variantType),
		ContentType: "image/jpeg",
		SizeBytes:   512,
		StorageKey:  "processed/" + fileID.String() + "/" + variantType + "/image.jpg",
		Width:       &width,
		Height:      &height,
		CreatedAt:   createdAt,
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, want, rec.Body.String())
	}
}

func assertBodyContains(t *testing.T, rec *httptest.ResponseRecorder, substr string) {
	t.Helper()
	if !bytes.Contains(rec.Body.Bytes(), []byte(substr)) {
		t.Errorf("body = %q, want to contain %q", rec.Body.String(), substr)
	}
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u := uuid.UUID(id.Bytes)
	return u.String()
}
