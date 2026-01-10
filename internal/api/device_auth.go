package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type DeviceAuthQuerier interface {
	CreateAPIToken(ctx context.Context, arg db.CreateAPITokenParams) (db.ApiToken, error)
}

type DeviceCode struct {
	UserCode   string
	DeviceCode string
	ExpiresAt  time.Time
	Approved   bool
	APIKey     string
	UserID     *uuid.UUID
}

type DeviceAuthStore struct {
	mu    sync.RWMutex
	codes map[string]*DeviceCode
}

func NewDeviceAuthStore() *DeviceAuthStore {
	store := &DeviceAuthStore{
		codes: make(map[string]*DeviceCode),
	}
	go store.cleanup()
	return store
}

func (s *DeviceAuthStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.codes {
			if now.After(v.ExpiresAt) {
				delete(s.codes, k)
			}
		}
		s.mu.Unlock()
	}
}

func (s *DeviceAuthStore) Create() (*DeviceCode, error) {
	deviceCode := make([]byte, 32)
	if _, err := rand.Read(deviceCode); err != nil {
		return nil, err
	}

	userCodeBytes := make([]byte, 4)
	if _, err := rand.Read(userCodeBytes); err != nil {
		return nil, err
	}
	userCode := strings.ToUpper(hex.EncodeToString(userCodeBytes))
	userCode = userCode[:4] + "-" + userCode[4:]

	code := &DeviceCode{
		UserCode:   userCode,
		DeviceCode: base64.URLEncoding.EncodeToString(deviceCode),
		ExpiresAt:  time.Now().Add(15 * time.Minute),
	}

	s.mu.Lock()
	s.codes[code.DeviceCode] = code
	s.mu.Unlock()

	return code, nil
}

func (s *DeviceAuthStore) Get(deviceCode string) *DeviceCode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.codes[deviceCode]
}

func (s *DeviceAuthStore) GetByUserCode(userCode string) *DeviceCode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, code := range s.codes {
		if code.UserCode == userCode {
			return code
		}
	}
	return nil
}

func (s *DeviceAuthStore) Approve(deviceCode string, userID uuid.UUID, apiKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if code, ok := s.codes[deviceCode]; ok {
		code.Approved = true
		code.UserID = &userID
		code.APIKey = apiKey
	}
}

func (s *DeviceAuthStore) Delete(deviceCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.codes, deviceCode)
}

var deviceAuthStore = NewDeviceAuthStore()

type DeviceAuthConfig struct {
	Queries DeviceAuthQuerier
	BaseURL string
}

type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type DeviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

type DeviceTokenResponse struct {
	APIKey           string `json:"api_key,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func DeviceAuthHandler(cfg *DeviceAuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code, err := deviceAuthStore.Create()
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		verificationURI := cfg.BaseURL + "/auth/device"

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceAuthResponse{
			DeviceCode:      code.DeviceCode,
			UserCode:        code.UserCode,
			VerificationURI: verificationURI,
			ExpiresIn:       900,
			Interval:        5,
		})
	}
}

func DeviceTokenHandler(cfg *DeviceAuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DeviceTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
				Error:            "invalid_request",
				ErrorDescription: "Invalid request body",
			})
			return
		}

		code := deviceAuthStore.Get(req.DeviceCode)
		if code == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
				Error:            "invalid_grant",
				ErrorDescription: "Device code not found or expired",
			})
			return
		}

		if time.Now().After(code.ExpiresAt) {
			deviceAuthStore.Delete(req.DeviceCode)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
				Error:            "expired_token",
				ErrorDescription: "Device code has expired",
			})
			return
		}

		if !code.Approved {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
				Error:            "authorization_pending",
				ErrorDescription: "User has not yet authorized this device",
			})
			return
		}

		apiKey := code.APIKey
		deviceAuthStore.Delete(req.DeviceCode)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
			APIKey: apiKey,
		})
	}
}

func DeviceApproveHandler(cfg *DeviceAuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		userCode := r.URL.Query().Get("code")
		if userCode == "" {
			var req struct {
				UserCode string `json:"user_code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				userCode = req.UserCode
			}
		}

		if userCode == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "missing_code", "User code is required", http.StatusBadRequest))
			return
		}

		code := deviceAuthStore.GetByUserCode(userCode)
		if code == nil || time.Now().After(code.ExpiresAt) {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_code", "Invalid or expired code", http.StatusBadRequest))
			return
		}

		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}
		apiKey := "fp_" + base64.URLEncoding.EncodeToString(tokenBytes)

		tokenHash := sha256.Sum256([]byte(apiKey))
		tokenHashStr := hex.EncodeToString(tokenHash[:])

		if cfg.Queries != nil {
			pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
			_, err := cfg.Queries.CreateAPIToken(r.Context(), db.CreateAPITokenParams{
				UserID:    pgUserID,
				Name:      "fc CLI",
				TokenHash: tokenHashStr,
			})
			if err != nil {
				apperror.WriteJSON(w, r, apperror.ErrInternal)
				return
			}
		}

		deviceAuthStore.Approve(code.DeviceCode, userID, apiKey)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "approved",
			"message": "Device authorized successfully",
		})
	}
}
