package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSSEHub_RegisterUnregister(t *testing.T) {
	hub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}

	go hub.run()

	client := &SSEClient{
		UserID:  uuid.New(),
		FileID:  "",
		Channel: make(chan SSEMessage, 10),
	}

	hub.register <- client

	time.Sleep(10 * time.Millisecond)

	hub.mu.RLock()
	_, exists := hub.clients[client]
	hub.mu.RUnlock()

	if !exists {
		t.Error("Client was not registered")
	}

	hub.unregister <- client

	time.Sleep(10 * time.Millisecond)

	hub.mu.RLock()
	_, exists = hub.clients[client]
	hub.mu.RUnlock()

	if exists {
		t.Error("Client was not unregistered")
	}
}

func TestSSEClient(t *testing.T) {
	userID := uuid.New()
	fileID := "file-123"

	client := &SSEClient{
		UserID:  userID,
		FileID:  fileID,
		Channel: make(chan SSEMessage, 10),
	}

	if client.UserID != userID {
		t.Errorf("UserID = %v, want %v", client.UserID, userID)
	}
	if client.FileID != fileID {
		t.Errorf("FileID = %q, want %q", client.FileID, fileID)
	}
	if cap(client.Channel) != 10 {
		t.Errorf("Channel capacity = %d, want 10", cap(client.Channel))
	}
}

func TestSSEMessage_JSON(t *testing.T) {
	msg := SSEMessage{
		Event: "file:status",
		Data: map[string]interface{}{
			"file_id":  "file-123",
			"status":   "processing",
			"progress": 50,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded SSEMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Event != msg.Event {
		t.Errorf("Event = %q, want %q", decoded.Event, msg.Event)
	}
}

func TestSSEConfig(t *testing.T) {
	cfg := &SSEConfig{
		Queries: nil,
	}

	if cfg.Queries != nil {
		t.Error("Queries should be nil")
	}
}

func TestBroadcastFileStatus(t *testing.T) {
	originalHub := sseHub
	defer func() { sseHub = originalHub }()

	testHub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}
	sseHub = testHub

	go testHub.run()

	userID := uuid.New()
	fileID := "file-123"

	client := &SSEClient{
		UserID:  userID,
		FileID:  fileID,
		Channel: make(chan SSEMessage, 10),
	}

	testHub.register <- client
	time.Sleep(10 * time.Millisecond)

	BroadcastFileStatus(fileID, userID, "processing", 50)

	select {
	case msg := <-client.Channel:
		if msg.Event != "file:status" {
			t.Errorf("Event = %q, want %q", msg.Event, "file:status")
		}
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Data is not a map")
		}
		if data["file_id"] != fileID {
			t.Errorf("file_id = %v, want %v", data["file_id"], fileID)
		}
		if data["status"] != "processing" {
			t.Errorf("status = %v, want %v", data["status"], "processing")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive message")
	}

	testHub.unregister <- client
}

func TestBroadcastFileStatus_DifferentUser(t *testing.T) {
	originalHub := sseHub
	defer func() { sseHub = originalHub }()

	testHub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}
	sseHub = testHub

	go testHub.run()

	userID := uuid.New()
	otherUserID := uuid.New()
	fileID := "file-123"

	client := &SSEClient{
		UserID:  userID,
		FileID:  fileID,
		Channel: make(chan SSEMessage, 10),
	}

	testHub.register <- client
	time.Sleep(10 * time.Millisecond)

	BroadcastFileStatus(fileID, otherUserID, "processing", 50)

	select {
	case <-client.Channel:
		t.Error("Should not receive message for different user")
	case <-time.After(50 * time.Millisecond):
	}

	testHub.unregister <- client
}

func TestBroadcastFileStatus_DifferentFile(t *testing.T) {
	originalHub := sseHub
	defer func() { sseHub = originalHub }()

	testHub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}
	sseHub = testHub

	go testHub.run()

	userID := uuid.New()
	fileID := "file-123"
	otherFileID := "file-456"

	client := &SSEClient{
		UserID:  userID,
		FileID:  fileID,
		Channel: make(chan SSEMessage, 10),
	}

	testHub.register <- client
	time.Sleep(10 * time.Millisecond)

	BroadcastFileStatus(otherFileID, userID, "processing", 50)

	select {
	case <-client.Channel:
		t.Error("Should not receive message for different file")
	case <-time.After(50 * time.Millisecond):
	}

	testHub.unregister <- client
}

func TestBroadcastFileStatus_WatchAllFiles(t *testing.T) {
	originalHub := sseHub
	defer func() { sseHub = originalHub }()

	testHub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}
	sseHub = testHub

	go testHub.run()

	userID := uuid.New()
	fileID := "file-123"

	client := &SSEClient{
		UserID:  userID,
		FileID:  "",
		Channel: make(chan SSEMessage, 10),
	}

	testHub.register <- client
	time.Sleep(10 * time.Millisecond)

	BroadcastFileStatus(fileID, userID, "processing", 50)

	select {
	case msg := <-client.Channel:
		if msg.Event != "file:status" {
			t.Errorf("Event = %q, want %q", msg.Event, "file:status")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Should receive message when watching all files")
	}

	testHub.unregister <- client
}

func TestBroadcastJobComplete(t *testing.T) {
	originalHub := sseHub
	defer func() { sseHub = originalHub }()

	testHub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}
	sseHub = testHub

	go testHub.run()

	userID := uuid.New()
	fileID := "file-123"
	variantID := "variant-456"

	client := &SSEClient{
		UserID:  userID,
		FileID:  fileID,
		Channel: make(chan SSEMessage, 10),
	}

	testHub.register <- client
	time.Sleep(10 * time.Millisecond)

	BroadcastJobComplete(fileID, userID, "transcode", variantID)

	select {
	case msg := <-client.Channel:
		if msg.Event != "job:complete" {
			t.Errorf("Event = %q, want %q", msg.Event, "job:complete")
		}
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Data is not a map")
		}
		if data["file_id"] != fileID {
			t.Errorf("file_id = %v, want %v", data["file_id"], fileID)
		}
		if data["job_type"] != "transcode" {
			t.Errorf("job_type = %v, want %v", data["job_type"], "transcode")
		}
		if data["variant_id"] != variantID {
			t.Errorf("variant_id = %v, want %v", data["variant_id"], variantID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive message")
	}

	testHub.unregister <- client
}

func TestFileStatusSSEHandler_Unauthorized(t *testing.T) {
	cfg := &SSEConfig{}

	handler := FileStatusSSEHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/123/events", nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestFileStatusHandler_Unauthorized(t *testing.T) {
	cfg := &SSEConfig{}

	handler := FileStatusHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/123/status", nil)
	req.SetPathValue("id", "123")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestFileStatusHandler_InvalidFileID(t *testing.T) {
	cfg := &SSEConfig{}

	handler := FileStatusHandler(cfg)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/invalid-uuid/status", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("id", "invalid-uuid")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestFileStatusHandler_NoQueries(t *testing.T) {
	cfg := &SSEConfig{
		Queries: nil,
	}

	handler := FileStatusHandler(cfg)

	userID := uuid.New()
	fileID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID.String()+"/status", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("id", fileID.String())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestUploadProgressSSEHandler_Unauthorized(t *testing.T) {
	handler := UploadProgressSSEHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/upload/progress?upload_id=123", nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUploadProgressSSEHandler_MissingUploadID(t *testing.T) {
	handler := UploadProgressSSEHandler()

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)

	req := httptest.NewRequest(http.MethodGet, "/v1/upload/progress", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestProcessingStatusResponse_JSON(t *testing.T) {
	resp := ProcessingStatusResponse{
		FileID: "file-123",
		Status: "completed",
		Variants: []VariantResponse{
			{
				ID:          "variant-1",
				VariantType: "mp4_720p",
				ContentType: "video/mp4",
				SizeBytes:   1024 * 1024,
				Width:       1280,
				Height:      720,
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded ProcessingStatusResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.FileID != resp.FileID {
		t.Errorf("FileID = %q, want %q", decoded.FileID, resp.FileID)
	}
	if decoded.Status != resp.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, resp.Status)
	}
	if len(decoded.Variants) != 1 {
		t.Fatalf("Variants length = %d, want 1", len(decoded.Variants))
	}
	if decoded.Variants[0].VariantType != "mp4_720p" {
		t.Errorf("Variant type = %q, want %q", decoded.Variants[0].VariantType, "mp4_720p")
	}
}

func TestVariantResponse_JSON(t *testing.T) {
	resp := VariantResponse{
		ID:          "variant-123",
		VariantType: "hls_1080p",
		ContentType: "application/x-mpegURL",
		SizeBytes:   5 * 1024 * 1024,
		Width:       1920,
		Height:      1080,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded VariantResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.ID != resp.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, resp.ID)
	}
	if decoded.Width != resp.Width {
		t.Errorf("Width = %d, want %d", decoded.Width, resp.Width)
	}
	if decoded.Height != resp.Height {
		t.Errorf("Height = %d, want %d", decoded.Height, resp.Height)
	}
}

func TestSSEHub_ConcurrentAccess(t *testing.T) {
	hub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}

	go hub.run()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client := &SSEClient{
				UserID:  uuid.New(),
				FileID:  "",
				Channel: make(chan SSEMessage, 10),
			}

			hub.register <- client
			time.Sleep(5 * time.Millisecond)
			hub.unregister <- client
		}()
	}

	wg.Wait()
}

func TestSSEHub_Broadcast(t *testing.T) {
	hub := &SSEHub{
		clients:    make(map[*SSEClient]bool),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan SSEMessage),
	}

	go hub.run()

	client1 := &SSEClient{
		UserID:  uuid.New(),
		FileID:  "",
		Channel: make(chan SSEMessage, 10),
	}
	client2 := &SSEClient{
		UserID:  uuid.New(),
		FileID:  "",
		Channel: make(chan SSEMessage, 10),
	}

	hub.register <- client1
	hub.register <- client2
	time.Sleep(10 * time.Millisecond)

	msg := SSEMessage{
		Event: "test",
		Data:  "test data",
	}
	hub.broadcast <- msg

	time.Sleep(10 * time.Millisecond)

	select {
	case received := <-client1.Channel:
		if received.Event != "test" {
			t.Errorf("Client1 event = %q, want %q", received.Event, "test")
		}
	default:
		t.Error("Client1 did not receive broadcast")
	}

	select {
	case received := <-client2.Channel:
		if received.Event != "test" {
			t.Errorf("Client2 event = %q, want %q", received.Event, "test")
		}
	default:
		t.Error("Client2 did not receive broadcast")
	}

	hub.unregister <- client1
	hub.unregister <- client2
}
