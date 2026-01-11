package api

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
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

	// Share-related storage
	shares        map[string]db.FileShare
	sharesByToken map[string]db.GetFileShareByTokenRow
	caches        map[string]db.TransformCache
	requestCounts map[string]int32

	GetFileErr        error
	ListFilesErr      error
	CreateFileErr     error
	SoftDeleteFileErr error
	ListVariantsErr   error
	CountFilesResult  int64

	// Share-related errors
	GetFileShareByTokenErr  error
	CreateFileShareErr      error
	ListFileSharesByFileErr error
	DeleteFileShareErr      error
	GetTransformCacheErr    error
	CreateTransformCacheErr error

	BillingTier db.SubscriptionTier
}

func NewMockQuerier() *MockQuerier {
	return &MockQuerier{
		files:         make(map[string]db.File),
		variants:      make(map[string]db.FileVariant),
		shares:        make(map[string]db.FileShare),
		sharesByToken: make(map[string]db.GetFileShareByTokenRow),
		caches:        make(map[string]db.TransformCache),
		requestCounts: make(map[string]int32),
		BillingTier:   db.SubscriptionTierPro, // Default to Pro for existing tests
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

func (m *MockQuerier) GetFilesByIDs(ctx context.Context, ids []pgtype.UUID) ([]db.File, error) {
	if m.GetFileErr != nil {
		return nil, m.GetFileErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []db.File
	for _, id := range ids {
		key := uuidToString(id)
		f, ok := m.files[key]
		if ok && !f.DeletedAt.Valid {
			result = append(result, f)
		}
	}
	return result, nil
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

func (m *MockQuerier) ListFilesByUserWithCount(ctx context.Context, arg db.ListFilesByUserWithCountParams) ([]db.ListFilesByUserWithCountRow, error) {
	if m.ListFilesErr != nil {
		return nil, m.ListFilesErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var allFiles []db.File
	userKey := uuidToString(arg.UserID)
	for _, f := range m.files {
		if uuidToString(f.UserID) == userKey && !f.DeletedAt.Valid {
			allFiles = append(allFiles, f)
		}
	}

	totalCount := int64(len(allFiles))

	start := int(arg.Offset)
	if start >= len(allFiles) {
		return []db.ListFilesByUserWithCountRow{}, nil
	}
	end := start + int(arg.Limit)
	if end > len(allFiles) {
		end = len(allFiles)
	}

	paginated := allFiles[start:end]
	result := make([]db.ListFilesByUserWithCountRow, len(paginated))
	for i, f := range paginated {
		result[i] = db.ListFilesByUserWithCountRow{
			ID:          f.ID,
			UserID:      f.UserID,
			Filename:    f.Filename,
			ContentType: f.ContentType,
			SizeBytes:   f.SizeBytes,
			StorageKey:  f.StorageKey,
			Status:      f.Status,
			CreatedAt:   f.CreatedAt,
			UpdatedAt:   f.UpdatedAt,
			DeletedAt:   f.DeletedAt,
			TotalCount:  totalCount,
		}
	}
	return result, nil
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

func (m *MockQuerier) GetVariant(ctx context.Context, arg db.GetVariantParams) (db.FileVariant, error) {
	if m.ListVariantsErr != nil {
		return db.FileVariant{}, m.ListVariantsErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	fileKey := uuidToString(arg.FileID)
	for _, v := range m.variants {
		if uuidToString(v.FileID) == fileKey && v.VariantType == arg.VariantType {
			return v, nil
		}
	}
	return db.FileVariant{}, errors.New("variant not found")
}

func (m *MockQuerier) GetAPITokenByHash(ctx context.Context, tokenHash string) (db.GetAPITokenByHashRow, error) {
	return db.GetAPITokenByHashRow{}, errors.New("not implemented in mock")
}

func (m *MockQuerier) UpdateAPITokenLastUsed(ctx context.Context, id pgtype.UUID) error {
	return nil
}

func (m *MockQuerier) GetFileShareByToken(ctx context.Context, token string) (db.GetFileShareByTokenRow, error) {
	if m.GetFileShareByTokenErr != nil {
		return db.GetFileShareByTokenRow{}, m.GetFileShareByTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	share, ok := m.sharesByToken[token]
	if !ok {
		return db.GetFileShareByTokenRow{}, errors.New("share not found")
	}

	// Check expiration
	if share.ExpiresAt.Valid && share.ExpiresAt.Time.Before(time.Now()) {
		return db.GetFileShareByTokenRow{}, errors.New("share expired")
	}

	return share, nil
}

// AddShareByToken adds a share to the mock for testing CDN handler
func (m *MockQuerier) AddShareByToken(token string, share db.GetFileShareByTokenRow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sharesByToken[token] = share
}

func (m *MockQuerier) IncrementShareAccessCount(ctx context.Context, id pgtype.UUID) error {
	return nil
}

func (m *MockQuerier) GetTransformCache(ctx context.Context, arg db.GetTransformCacheParams) (db.TransformCache, error) {
	if m.GetTransformCacheErr != nil {
		return db.TransformCache{}, m.GetTransformCacheErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := uuidToString(arg.FileID) + ":" + arg.CacheKey
	cache, ok := m.caches[key]
	if !ok {
		return db.TransformCache{}, errors.New("cache not found")
	}
	return cache, nil
}

func (m *MockQuerier) CreateTransformCache(ctx context.Context, arg db.CreateTransformCacheParams) (db.TransformCache, error) {
	if m.CreateTransformCacheErr != nil {
		return db.TransformCache{}, m.CreateTransformCacheErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	cache := db.TransformCache{
		ID:              pgID,
		FileID:          arg.FileID,
		CacheKey:        arg.CacheKey,
		TransformParams: arg.TransformParams,
		StorageKey:      arg.StorageKey,
		ContentType:     arg.ContentType,
		SizeBytes:       arg.SizeBytes,
		Width:           arg.Width,
		Height:          arg.Height,
		RequestCount:    1,
		CreatedAt:       now,
		LastAccessedAt:  now,
	}

	key := uuidToString(arg.FileID) + ":" + arg.CacheKey
	m.caches[key] = cache
	return cache, nil
}

// AddTransformCache adds a cache entry for testing
func (m *MockQuerier) AddTransformCache(cache db.TransformCache) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := uuidToString(cache.FileID) + ":" + cache.CacheKey
	m.caches[key] = cache
}

// SetTransformRequestCount sets the request count for testing cache threshold
func (m *MockQuerier) SetTransformRequestCount(fileID pgtype.UUID, cacheKey string, count int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := uuidToString(fileID) + ":" + cacheKey
	m.requestCounts[key] = count
}

func (m *MockQuerier) IncrementTransformCacheCount(ctx context.Context, arg db.IncrementTransformCacheCountParams) error {
	return nil
}

func (m *MockQuerier) GetTransformRequestCount(ctx context.Context, arg db.GetTransformRequestCountParams) (int32, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := uuidToString(arg.FileID) + ":" + arg.CacheKey
	return m.requestCounts[key], nil
}

func (m *MockQuerier) CreateFileShare(ctx context.Context, arg db.CreateFileShareParams) (db.FileShare, error) {
	if m.CreateFileShareErr != nil {
		return db.FileShare{}, m.CreateFileShareErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	share := db.FileShare{
		ID:                pgID,
		FileID:            arg.FileID,
		Token:             arg.Token,
		ExpiresAt:         arg.ExpiresAt,
		AllowedTransforms: arg.AllowedTransforms,
		AccessCount:       0,
		CreatedAt:         now,
	}

	m.shares[id.String()] = share
	return share, nil
}

func (m *MockQuerier) ListFileSharesByFile(ctx context.Context, fileID pgtype.UUID) ([]db.FileShare, error) {
	if m.ListFileSharesByFileErr != nil {
		return nil, m.ListFileSharesByFileErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	fileKey := uuidToString(fileID)
	var result []db.FileShare
	for _, s := range m.shares {
		if uuidToString(s.FileID) == fileKey {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *MockQuerier) DeleteFileShare(ctx context.Context, arg db.DeleteFileShareParams) error {
	if m.DeleteFileShareErr != nil {
		return m.DeleteFileShareErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	shareKey := uuidToString(arg.ID)
	share, ok := m.shares[shareKey]
	if !ok {
		return errors.New("share not found")
	}

	// Verify ownership via file
	fileKey := uuidToString(share.FileID)
	file, ok := m.files[fileKey]
	if !ok {
		return errors.New("file not found")
	}

	if uuidToString(file.UserID) != uuidToString(arg.UserID) {
		return errors.New("not authorized")
	}

	delete(m.shares, shareKey)
	return nil
}

// AddShare adds a share to the mock for testing
func (m *MockQuerier) AddShare(share db.FileShare) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := uuidToString(share.ID)
	m.shares[key] = share
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

func (m *MockQuerier) GetUserTransformationUsage(ctx context.Context, id pgtype.UUID) (db.GetUserTransformationUsageRow, error) {
	tier := m.BillingTier
	if tier == "" {
		tier = db.SubscriptionTierPro
	}

	var limit int32
	switch tier {
	case db.SubscriptionTierEnterprise:
		limit = -1
	case db.SubscriptionTierPro:
		limit = 10000
	default:
		limit = 100
	}

	return db.GetUserTransformationUsageRow{
		TransformationsCount: 0,
		TransformationsLimit: limit,
	}, nil
}

func (m *MockQuerier) IncrementTransformationCount(ctx context.Context, id pgtype.UUID) error {
	return nil
}

func (m *MockQuerier) CreateBatchOperation(ctx context.Context, arg db.CreateBatchOperationParams) (db.BatchOperation, error) {
	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	return db.BatchOperation{
		ID:             pgID,
		UserID:         arg.UserID,
		Status:         db.BatchStatusPending,
		TotalFiles:     arg.TotalFiles,
		CompletedFiles: 0,
		FailedFiles:    0,
		Presets:        arg.Presets,
		Webp:           arg.Webp,
		Quality:        arg.Quality,
		Watermark:      arg.Watermark,
		CreatedAt:      now,
	}, nil
}

func (m *MockQuerier) GetBatchOperationByUser(ctx context.Context, arg db.GetBatchOperationByUserParams) (db.BatchOperation, error) {
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return db.BatchOperation{
		ID:             arg.ID,
		UserID:         arg.UserID,
		Status:         db.BatchStatusPending,
		TotalFiles:     5,
		CompletedFiles: 0,
		FailedFiles:    0,
		Presets:        []string{"thumbnail"},
		Webp:           false,
		Quality:        85,
		CreatedAt:      now,
	}, nil
}

func (m *MockQuerier) CreateBatchItem(ctx context.Context, arg db.CreateBatchItemParams) (db.BatchItem, error) {
	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	return db.BatchItem{
		ID:        pgID,
		BatchID:   arg.BatchID,
		FileID:    arg.FileID,
		Status:    db.BatchStatusPending,
		JobIds:    arg.JobIds,
		CreatedAt: now,
	}, nil
}

func (m *MockQuerier) ListBatchItems(ctx context.Context, batchID pgtype.UUID) ([]db.BatchItem, error) {
	return []db.BatchItem{}, nil
}

func (m *MockQuerier) CountBatchItemsByStatus(ctx context.Context, batchID pgtype.UUID) (db.CountBatchItemsByStatusRow, error) {
	return db.CountBatchItemsByStatusRow{
		Pending:    0,
		Processing: 0,
		Completed:  0,
		Failed:     0,
	}, nil
}

func (m *MockQuerier) CreateAPIToken(ctx context.Context, arg db.CreateAPITokenParams) (db.ApiToken, error) {
	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	return db.ApiToken{
		ID:        pgID,
		UserID:    arg.UserID,
		Name:      arg.Name,
		TokenHash: arg.TokenHash,
		CreatedAt: now,
	}, nil
}

func (m *MockQuerier) GetUserVideoStorageUsage(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return 0, nil
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

func (m *MockQuerier) CreateJob(ctx context.Context, arg db.CreateJobParams) (db.ProcessingJob, error) {
	return db.ProcessingJob{
		ID:       pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true},
		FileID:   arg.FileID,
		JobType:  arg.JobType,
		Status:   db.JobStatusPending,
		Priority: arg.Priority,
	}, nil
}

func (m *MockQuerier) CreateWebhook(ctx context.Context, arg db.CreateWebhookParams) (db.Webhook, error) {
	return db.Webhook{
		ID:     pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true},
		UserID: arg.UserID,
		Url:    arg.Url,
		Secret: arg.Secret,
		Events: arg.Events,
		Active: true,
	}, nil
}

func (m *MockQuerier) GetWebhook(ctx context.Context, arg db.GetWebhookParams) (db.Webhook, error) {
	return db.Webhook{
		ID:     arg.ID,
		UserID: arg.UserID,
		Active: true,
	}, nil
}

func (m *MockQuerier) ListWebhooksByUser(ctx context.Context, arg db.ListWebhooksByUserParams) ([]db.Webhook, error) {
	return []db.Webhook{}, nil
}

func (m *MockQuerier) CountWebhooksByUser(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return 0, nil
}

func (m *MockQuerier) UpdateWebhook(ctx context.Context, arg db.UpdateWebhookParams) (db.Webhook, error) {
	return db.Webhook{
		ID:     arg.ID,
		UserID: arg.UserID,
		Url:    arg.Url,
		Events: arg.Events,
		Active: arg.Active,
	}, nil
}

func (m *MockQuerier) DeleteWebhook(ctx context.Context, arg db.DeleteWebhookParams) error {
	return nil
}

func (m *MockQuerier) ListDeliveriesByWebhook(ctx context.Context, arg db.ListDeliveriesByWebhookParams) ([]db.WebhookDelivery, error) {
	return []db.WebhookDelivery{}, nil
}

func (m *MockQuerier) CountDeliveriesByWebhook(ctx context.Context, webhookID pgtype.UUID) (int64, error) {
	return 0, nil
}

func (m *MockQuerier) CreateWebhookDelivery(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error) {
	return db.WebhookDelivery{
		ID:        pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true},
		WebhookID: arg.WebhookID,
		EventType: arg.EventType,
		Payload:   arg.Payload,
		Status:    db.WebhookDeliveryStatusPending,
	}, nil
}

func (m *MockQuerier) ListActiveWebhooksByUserAndEvent(ctx context.Context, arg db.ListActiveWebhooksByUserAndEventParams) ([]db.Webhook, error) {
	return []db.Webhook{}, nil
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

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u := uuid.UUID(id.Bytes)
	return u.String()
}
