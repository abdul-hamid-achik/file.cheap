package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type UserQuerier interface {
	GetUserByID(ctx context.Context, id pgtype.UUID) (db.User, error)
	GetUserFilesCount(ctx context.Context, userID pgtype.UUID) (int64, error)
	GetUserTotalStorageUsage(ctx context.Context, userID pgtype.UUID) (int64, error)
}

type UserConfig struct {
	Queries UserQuerier
}

type CurrentUserResponse struct {
	ID               string  `json:"id"`
	Email            string  `json:"email"`
	Name             string  `json:"name"`
	AvatarURL        *string `json:"avatar_url,omitempty"`
	SubscriptionTier string  `json:"subscription_tier"`
	CreatedAt        string  `json:"created_at"`
}

type UsageResponse struct {
	FilesUsed         int64   `json:"files_used"`
	FilesLimit        int32   `json:"files_limit"`
	TransformsUsed    int32   `json:"transforms_used"`
	TransformsLimit   int32   `json:"transforms_limit"`
	StorageUsedBytes  int64   `json:"storage_used_bytes"`
	StorageLimitBytes int64   `json:"storage_limit_bytes"`
	PlanName          string  `json:"plan_name"`
	PlanRenewsAt      *string `json:"plan_renews_at,omitempty"`
}

func GetCurrentUserHandler(cfg *UserConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "authentication required",
				},
			})
			return
		}

		if cfg.Queries == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "internal_error",
					"message": "service unavailable",
				},
			})
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		user, err := cfg.Queries.GetUserByID(r.Context(), pgUserID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "not_found",
					"message": "user not found",
				},
			})
			return
		}

		response := CurrentUserResponse{
			ID:               uuidFromPgtype(user.ID),
			Email:            user.Email,
			Name:             user.Name,
			AvatarURL:        user.AvatarUrl,
			SubscriptionTier: string(user.SubscriptionTier),
			CreatedAt:        user.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func GetUsageHandler(cfg *UserConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "authentication required",
				},
			})
			return
		}

		if cfg.Queries == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "internal_error",
					"message": "service unavailable",
				},
			})
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		user, err := cfg.Queries.GetUserByID(r.Context(), pgUserID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "not_found",
					"message": "user not found",
				},
			})
			return
		}

		filesCount, err := cfg.Queries.GetUserFilesCount(r.Context(), pgUserID)
		if err != nil {
			filesCount = 0
		}

		storageUsed, err := cfg.Queries.GetUserTotalStorageUsage(r.Context(), pgUserID)
		if err != nil {
			storageUsed = user.StorageUsedBytes
		}

		response := UsageResponse{
			FilesUsed:         filesCount,
			FilesLimit:        user.FilesLimit,
			TransformsUsed:    user.TransformationsCount,
			TransformsLimit:   user.TransformationsLimit,
			StorageUsedBytes:  storageUsed,
			StorageLimitBytes: user.StorageLimitBytes,
			PlanName:          string(user.SubscriptionTier),
		}

		if user.SubscriptionPeriodEnd.Valid {
			renewsAt := user.SubscriptionPeriodEnd.Time.Format("2006-01-02T15:04:05Z07:00")
			response.PlanRenewsAt = &renewsAt
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}
