package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/api"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	testPool    *pgxpool.Pool
	testStorage storage.Storage
	testSecret  = "test-secret-key-for-integration-tests"
)

func TestMain(m *testing.M) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		fmt.Println("Skipping integration tests: TEST_DATABASE_URL not set")
		os.Exit(0)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	if err := pool.Ping(ctx); err != nil {
		fmt.Printf("Failed to ping database: %v\n", err)
		os.Exit(1)
	}

	testPool = pool

	minioEndpoint := os.Getenv("TEST_MINIO_ENDPOINT")
	if minioEndpoint == "" {
		minioEndpoint = "localhost:9000"
	}

	storageCfg := &storage.Config{
		Endpoint:  minioEndpoint,
		AccessKey: os.Getenv("TEST_MINIO_ACCESS_KEY"),
		SecretKey: os.Getenv("TEST_MINIO_SECRET_KEY"),
		Bucket:    "test-files",
		UseSSL:    false,
	}
	if storageCfg.AccessKey == "" {
		storageCfg.AccessKey = "minioadmin"
	}
	if storageCfg.SecretKey == "" {
		storageCfg.SecretKey = "minioadmin"
	}

	store, err := storage.NewMinIOStorage(storageCfg)
	if err != nil {
		fmt.Printf("Failed to create storage: %v\n", err)
		os.Exit(1)
	}

	if err := store.EnsureBucket(ctx); err != nil {
		fmt.Printf("Failed to ensure bucket: %v\n", err)
		os.Exit(1)
	}

	testStorage = store

	code := m.Run()

	pool.Close()
	os.Exit(code)
}

func TestAPIHealthEndpoint(t *testing.T) {
	cfg := &api.Config{
		Storage:   testStorage,
		Queries:   db.New(testPool),
		JWTSecret: testSecret,
		BaseURL:   "https://api.file.cheap",
	}
	router := api.NewRouter(cfg)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %q", body["status"])
	}
}

func TestAPIRoutePaths(t *testing.T) {
	cfg := &api.Config{
		Storage:   testStorage,
		Queries:   db.New(testPool),
		JWTSecret: testSecret,
		BaseURL:   "https://api.file.cheap",
	}
	router := api.NewRouter(cfg)
	server := httptest.NewServer(router)
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		authenticated  bool
		expectedStatus int
	}{
		{
			name:           "health endpoint accessible",
			method:         "GET",
			path:           "/health",
			authenticated:  false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "v1/files requires auth",
			method:         "GET",
			path:           "/v1/files",
			authenticated:  false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "v1/files accessible with auth",
			method:         "GET",
			path:           "/v1/files",
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "v1/upload requires auth",
			method:         "POST",
			path:           "/v1/upload",
			authenticated:  false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "v1/auth/device is public",
			method:         "POST",
			path:           "/v1/auth/device",
			authenticated:  false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "old /api/v1 path returns 404",
			method:         "GET",
			path:           "/api/v1/files",
			authenticated:  true,
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			var err error

			if tt.method == "GET" {
				req, err = http.NewRequest(tt.method, server.URL+tt.path, nil)
			} else {
				req, err = http.NewRequest(tt.method, server.URL+tt.path, strings.NewReader("{}"))
				req.Header.Set("Content-Type", "application/json")
			}
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.authenticated {
				token := generateTestToken(t, uuid.New(), time.Hour)
				req.Header.Set("Authorization", "Bearer "+token)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectedStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, resp.StatusCode, string(body))
			}
		})
	}
}

func TestAPIFileUploadFlow(t *testing.T) {
	queries := db.New(testPool)

	testUser, err := createTestUser(t, queries)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupTestUser(t, queries, testUser.ID)

	cfg := &api.Config{
		Storage:       testStorage,
		Queries:       queries,
		JWTSecret:     testSecret,
		BaseURL:       "https://api.file.cheap",
		MaxUploadSize: 10 * 1024 * 1024,
	}
	router := api.NewRouter(cfg)
	server := httptest.NewServer(router)
	defer server.Close()

	var userUUID uuid.UUID
	copy(userUUID[:], testUser.ID.Bytes[:])
	token := generateTestToken(t, userUUID, time.Hour)

	body, contentType := createTestImageForm(t)
	req, err := http.NewRequest("POST", server.URL+"/v1/upload", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 202, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var uploadResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := uploadResp["id"]; !ok {
		t.Error("Response missing 'id' field")
	}
	if _, ok := uploadResp["filename"]; !ok {
		t.Error("Response missing 'filename' field")
	}
	if _, ok := uploadResp["status"]; !ok {
		t.Error("Response missing 'status' field")
	}

	fileID := uploadResp["id"].(string)

	req, err = http.NewRequest("GET", server.URL+"/v1/files/"+fileID, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	req, err = http.NewRequest("DELETE", server.URL+"/v1/files/"+fileID, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 204, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}
}

func TestAPIDeviceAuthFlow(t *testing.T) {
	cfg := &api.Config{
		Storage:   testStorage,
		Queries:   db.New(testPool),
		JWTSecret: testSecret,
		BaseURL:   "https://api.file.cheap",
	}
	router := api.NewRouter(cfg)
	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest("POST", server.URL+"/v1/auth/device", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var deviceResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	requiredFields := []string{"device_code", "user_code", "verification_uri", "expires_in", "interval"}
	for _, field := range requiredFields {
		if _, ok := deviceResp[field]; !ok {
			t.Errorf("Response missing %q field", field)
		}
	}

	verificationURI, ok := deviceResp["verification_uri"].(string)
	if !ok {
		t.Fatal("verification_uri is not a string")
	}

	expectedPrefix := "https://api.file.cheap/auth/device"
	if verificationURI != expectedPrefix {
		t.Errorf("Expected verification_uri to be %q, got %q", expectedPrefix, verificationURI)
	}
}

func TestAPIBatchTransformEndpoint(t *testing.T) {
	queries := db.New(testPool)

	testUser, err := createTestUser(t, queries)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupTestUser(t, queries, testUser.ID)

	var userUUID uuid.UUID
	copy(userUUID[:], testUser.ID.Bytes[:])

	mockBroker := &mockBroker{}
	cfg := &api.Config{
		Storage:       testStorage,
		Queries:       queries,
		Broker:        mockBroker,
		JWTSecret:     testSecret,
		BaseURL:       "https://api.file.cheap",
		MaxUploadSize: 10 * 1024 * 1024,
	}
	router := api.NewRouter(cfg)
	server := httptest.NewServer(router)
	defer server.Close()

	token := generateTestToken(t, userUUID, time.Hour)

	body, contentType := createTestImageForm(t)
	req, _ := http.NewRequest("POST", server.URL+"/v1/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, _ := http.DefaultClient.Do(req)
	var uploadResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResp)
	_ = resp.Body.Close()

	fileID := uploadResp["id"].(string)

	batchBody := fmt.Sprintf(`{"file_ids":["%s"],"presets":["thumbnail"]}`, fileID)
	req, err = http.NewRequest("POST", server.URL+"/v1/batch/transform", strings.NewReader(batchBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 202, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var batchResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := batchResp["batch_id"]; !ok {
		t.Error("Response missing 'batch_id' field")
	}
	if _, ok := batchResp["status_url"]; !ok {
		t.Error("Response missing 'status_url' field")
	}

	statusURL, ok := batchResp["status_url"].(string)
	if ok && strings.Contains(statusURL, "/api/v1/") {
		t.Errorf("status_url should use /v1/ not /api/v1/: %s", statusURL)
	}
}

func generateTestToken(t *testing.T, userID uuid.UUID, expiry time.Duration) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": time.Now().Add(expiry).Unix(),
		"iat": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	return tokenString
}

func createTestUser(t *testing.T, queries *db.Queries) (db.User, error) {
	t.Helper()

	ctx := context.Background()
	email := fmt.Sprintf("test-%s@example.com", uuid.New().String()[:8])
	hash := "test-hash"

	return queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: &hash,
		Role:         db.UserRoleUser,
	})
}

func cleanupTestUser(t *testing.T, queries *db.Queries, userID pgtype.UUID) {
	t.Helper()

	ctx := context.Background()
	_ = queries.DeleteUser(ctx, userID)
}

func createTestImageForm(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "test.jpg")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	if err := jpeg.Encode(part, img, nil); err != nil {
		t.Fatalf("Failed to encode JPEG: %v", err)
	}

	_ = writer.Close()

	return &buf, writer.FormDataContentType()
}

type mockBroker struct {
	jobs []mockJob
}

type mockJob struct {
	jobType string
	payload interface{}
}

func (b *mockBroker) Enqueue(jobType string, payload interface{}) (string, error) {
	b.jobs = append(b.jobs, mockJob{jobType: jobType, payload: payload})
	return uuid.New().String(), nil
}

// TestCDNShareFlow tests the complete CDN share workflow:
// 1. Upload a file
// 2. Create a share
// 3. Access via CDN
// 4. List shares
// 5. Delete share
func TestCDNShareFlow(t *testing.T) {
	queries := db.New(testPool)

	testUser, err := createTestUser(t, queries)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupTestUser(t, queries, testUser.ID)

	broker := &mockBroker{}
	cfg := &api.Config{
		Storage:       testStorage,
		Queries:       queries,
		Broker:        broker,
		JWTSecret:     testSecret,
		BaseURL:       "https://api.file.cheap",
		MaxUploadSize: 10 * 1024 * 1024,
	}
	router := api.NewRouter(cfg)
	server := httptest.NewServer(router)
	defer server.Close()

	var userUUID uuid.UUID
	copy(userUUID[:], testUser.ID.Bytes[:])
	token := generateTestToken(t, userUUID, time.Hour)

	// Step 1: Upload a file
	body, contentType := createTestImageForm(t)
	req, err := http.NewRequest("POST", server.URL+"/v1/upload", body)
	if err != nil {
		t.Fatalf("Failed to create upload request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Upload request failed: %v", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("Expected status 202, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var uploadResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResp)
	_ = resp.Body.Close()

	fileID, ok := uploadResp["id"].(string)
	if !ok {
		t.Fatalf("Upload response missing 'id' field")
	}

	// Step 2: Create a share
	req, err = http.NewRequest("POST", server.URL+"/v1/files/"+fileID+"/share?expires=1h", nil)
	if err != nil {
		t.Fatalf("Failed to create share request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Create share request failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("Expected status 201, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var shareResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&shareResp)
	_ = resp.Body.Close()

	shareID, ok := shareResp["id"].(string)
	if !ok {
		t.Fatalf("Share response missing 'id' field")
	}
	shareToken, ok := shareResp["token"].(string)
	if !ok {
		t.Fatalf("Share response missing 'token' field")
	}
	shareURL, ok := shareResp["share_url"].(string)
	if !ok {
		t.Fatalf("Share response missing 'share_url' field")
	}
	if _, ok := shareResp["expires_at"]; !ok {
		t.Error("Share response missing 'expires_at' field")
	}

	t.Logf("Created share: id=%s, token=%s, url=%s", shareID, shareToken, shareURL)

	// Step 3: Access via CDN (original file)
	cdnPath := fmt.Sprintf("/cdn/%s/_/test.jpg", shareToken)
	req, err = http.NewRequest("GET", server.URL+cdnPath, nil)
	if err != nil {
		t.Fatalf("Failed to create CDN request: %v", err)
	}

	// Don't follow redirects automatically
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("CDN request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("Expected CDN redirect (307), got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		t.Error("CDN response missing Location header")
	}

	// Step 4: List shares
	req, err = http.NewRequest("GET", server.URL+"/v1/files/"+fileID+"/shares", nil)
	if err != nil {
		t.Fatalf("Failed to create list shares request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("List shares request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var listResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	_ = resp.Body.Close()

	shares, ok := listResp["shares"].([]interface{})
	if !ok {
		t.Fatalf("List shares response missing 'shares' array")
	}
	if len(shares) != 1 {
		t.Errorf("Expected 1 share, got %d", len(shares))
	}

	// Step 5: Delete share
	req, err = http.NewRequest("DELETE", server.URL+"/v1/shares/"+shareID, nil)
	if err != nil {
		t.Fatalf("Failed to create delete share request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Delete share request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify share was deleted - CDN access should fail
	req, err = http.NewRequest("GET", server.URL+cdnPath, nil)
	if err != nil {
		t.Fatalf("Failed to create CDN verification request: %v", err)
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("CDN verification request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected CDN to return 404 after share deletion, got %d", resp.StatusCode)
	}
}

// TestCDNInvalidToken tests CDN access with invalid/missing tokens
func TestCDNInvalidToken(t *testing.T) {
	cfg := &api.Config{
		Storage:   testStorage,
		Queries:   db.New(testPool),
		JWTSecret: testSecret,
		BaseURL:   "https://api.file.cheap",
	}
	router := api.NewRouter(cfg)
	server := httptest.NewServer(router)
	defer server.Close()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "missing token",
			path:           "/cdn//_/test.jpg",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid token",
			path:           "/cdn/nonexistent-token/_/test.jpg",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", server.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			_ = resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}
