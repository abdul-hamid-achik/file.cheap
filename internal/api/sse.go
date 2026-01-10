package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SSEConfig holds configuration for SSE endpoints
type SSEConfig struct {
	Queries Querier
}

// SSEClient represents a connected SSE client
type SSEClient struct {
	UserID  uuid.UUID
	FileID  string // Optional - specific file to watch
	Channel chan SSEMessage
}

// SSEMessage represents a message to send to SSE clients
type SSEMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// SSEHub manages SSE client connections
type SSEHub struct {
	clients    map[*SSEClient]bool
	register   chan *SSEClient
	unregister chan *SSEClient
	broadcast  chan SSEMessage
	mu         sync.RWMutex
}

// Global SSE hub
var sseHub = &SSEHub{
	clients:    make(map[*SSEClient]bool),
	register:   make(chan *SSEClient),
	unregister: make(chan *SSEClient),
	broadcast:  make(chan SSEMessage),
}

// StartSSEHub starts the SSE hub goroutine
func StartSSEHub() {
	go sseHub.run()
}

func (h *SSEHub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Channel)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Channel <- message:
				default:
					// Client buffer full, skip
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastFileStatus sends a file status update to relevant clients
func BroadcastFileStatus(fileID string, userID uuid.UUID, status string, progress int) {
	message := SSEMessage{
		Event: "file:status",
		Data: map[string]interface{}{
			"file_id":  fileID,
			"status":   status,
			"progress": progress,
		},
	}

	sseHub.mu.RLock()
	defer sseHub.mu.RUnlock()

	for client := range sseHub.clients {
		if client.UserID == userID && (client.FileID == "" || client.FileID == fileID) {
			select {
			case client.Channel <- message:
			default:
				// Client buffer full, skip
			}
		}
	}
}

// BroadcastJobComplete sends a job completion event
func BroadcastJobComplete(fileID string, userID uuid.UUID, jobType string, variantID string) {
	message := SSEMessage{
		Event: "job:complete",
		Data: map[string]interface{}{
			"file_id":    fileID,
			"job_type":   jobType,
			"variant_id": variantID,
		},
	}

	sseHub.mu.RLock()
	defer sseHub.mu.RUnlock()

	for client := range sseHub.clients {
		if client.UserID == userID && (client.FileID == "" || client.FileID == fileID) {
			select {
			case client.Channel <- message:
			default:
			}
		}
	}
}

// FileStatusSSEHandler handles SSE connections for file status updates
func FileStatusSSEHandler(cfg *SSEConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		fileID := r.URL.Query().Get("file_id")

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		client := &SSEClient{
			UserID:  userID,
			FileID:  fileID,
			Channel: make(chan SSEMessage, 10),
		}

		sseHub.register <- client

		log.Info("SSE client connected", "user_id", userID.String(), "file_id", fileID)

		sendSSEMessage(w, flusher, SSEMessage{
			Event: "connected",
			Data:  map[string]string{"status": "connected"},
		})

		if fileID != "" && cfg.Queries != nil {
			go sendInitialFileStatus(r.Context(), w, flusher, cfg.Queries, userID, fileID)
		}

		ctx := r.Context()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				sseHub.unregister <- client
				log.Info("SSE client disconnected", "user_id", userID.String())
				return
			case msg := <-client.Channel:
				sendSSEMessage(w, flusher, msg)
			case <-ticker.C:
				sendSSEMessage(w, flusher, SSEMessage{
					Event: "keepalive",
					Data:  map[string]int64{"timestamp": time.Now().Unix()},
				})
			}
		}
	}
}

func sendSSEMessage(w http.ResponseWriter, flusher http.Flusher, msg SSEMessage) {
	data, err := json.Marshal(msg.Data)
	if err != nil {
		return
	}

	_, _ = fmt.Fprintf(w, "event: %s\n", msg.Event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func sendInitialFileStatus(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, queries Querier, userID uuid.UUID, fileIDStr string) {
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		return
	}

	pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
	file, err := queries.GetFile(ctx, pgFileID)
	if err != nil {
		return
	}

	if uuidFromPgtype(file.UserID) != userID.String() {
		return
	}

	sendSSEMessage(w, flusher, SSEMessage{
		Event: "file:status",
		Data: map[string]interface{}{
			"file_id":  fileIDStr,
			"status":   string(file.Status),
			"filename": file.Filename,
		},
	})

	variants, err := queries.ListVariantsByFile(ctx, pgFileID)
	if err == nil && len(variants) > 0 {
		variantsList := make([]map[string]interface{}, len(variants))
		for i, v := range variants {
			variantsList[i] = map[string]interface{}{
				"id":           uuidFromPgtype(v.ID),
				"variant_type": string(v.VariantType),
				"content_type": v.ContentType,
				"size_bytes":   v.SizeBytes,
			}
		}
		sendSSEMessage(w, flusher, SSEMessage{
			Event: "file:variants",
			Data: map[string]interface{}{
				"file_id":  fileIDStr,
				"variants": variantsList,
			},
		})
	}
}

// ProcessingStatusResponse represents processing status for a file
type ProcessingStatusResponse struct {
	FileID   string            `json:"file_id"`
	Status   string            `json:"status"`
	Variants []VariantResponse `json:"variants,omitempty"`
}

// VariantResponse represents a file variant in status responses
type VariantResponse struct {
	ID          string `json:"id"`
	VariantType string `json:"variant_type"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Width       int32  `json:"width,omitempty"`
	Height      int32  `json:"height,omitempty"`
}

// FileStatusHandler returns current file status (for polling fallback)
func FileStatusHandler(cfg *SSEConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			http.Error(w, "invalid file ID", http.StatusBadRequest)
			return
		}

		if cfg.Queries == nil {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if uuidFromPgtype(file.UserID) != userID.String() {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		response := ProcessingStatusResponse{
			FileID: fileIDStr,
			Status: string(file.Status),
		}

		variants, err := cfg.Queries.ListVariantsByFile(r.Context(), pgFileID)
		if err == nil && len(variants) > 0 {
			response.Variants = make([]VariantResponse, len(variants))
			for i, v := range variants {
				vr := VariantResponse{
					ID:          uuidFromPgtype(v.ID),
					VariantType: string(v.VariantType),
					ContentType: v.ContentType,
					SizeBytes:   v.SizeBytes,
				}
				if v.Width != nil {
					vr.Width = *v.Width
				}
				if v.Height != nil {
					vr.Height = *v.Height
				}
				response.Variants[i] = vr
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

// UploadProgressSSEHandler handles SSE for upload progress
func UploadProgressSSEHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		uploadID := r.URL.Query().Get("upload_id")
		if uploadID == "" {
			http.Error(w, "upload_id required", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		session, ok := sessionStore.Get(uploadID)
		if !ok {
			sendSSEMessage(w, flusher, SSEMessage{
				Event: "error",
				Data:  map[string]string{"message": "upload session not found"},
			})
			return
		}

		if session.UserID != userID {
			sendSSEMessage(w, flusher, SSEMessage{
				Event: "error",
				Data:  map[string]string{"message": "unauthorized"},
			})
			return
		}

		sendSSEMessage(w, flusher, SSEMessage{
			Event: "connected",
			Data: map[string]interface{}{
				"upload_id":    uploadID,
				"chunks_total": session.ChunksTotal,
			},
		})

		ctx := r.Context()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastChunks := -1
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				session, ok := sessionStore.Get(uploadID)
				if !ok {
					sendSSEMessage(w, flusher, SSEMessage{
						Event: "complete",
						Data:  map[string]string{"status": "completed"},
					})
					return
				}

				session.mu.Lock()
				chunksLoaded := len(session.ChunksLoaded)
				complete := chunksLoaded == session.ChunksTotal
				session.mu.Unlock()

				if chunksLoaded != lastChunks {
					lastChunks = chunksLoaded
					sendSSEMessage(w, flusher, SSEMessage{
						Event: "progress",
						Data: map[string]interface{}{
							"chunks_loaded": chunksLoaded,
							"chunks_total":  session.ChunksTotal,
							"complete":      complete,
							"percent":       float64(chunksLoaded) / float64(session.ChunksTotal) * 100,
						},
					})
				}

				if complete {
					return
				}
			}
		}
	}
}
