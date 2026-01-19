package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/abdul-hamid-achik/file.cheap/internal/analytics"
	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type AnalyticsQuerier interface {
	GetUserRole(ctx context.Context, id pgtype.UUID) (db.UserRole, error)
}

type AnalyticsConfig struct {
	Service *analytics.Service
	Queries AnalyticsQuerier
}

func AnalyticsHandler(cfg *AnalyticsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		data, err := cfg.Service.GetUserAnalytics(r.Context(), userID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data)
	}
}

func UsageAnalyticsHandler(cfg *AnalyticsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		data, err := cfg.Service.GetUserAnalytics(r.Context(), userID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		usage := map[string]any{
			"files_used":         data.FilesUsed,
			"files_limit":        data.FilesLimit,
			"transforms_used":    data.TransformsUsed,
			"transforms_limit":   data.TransformsLimit,
			"storage_used_bytes": data.StorageUsedBytes,
			"storage_limit_bytes": data.StorageLimitBytes,
			"plan_name":          data.PlanName,
			"plan_renews_at":     data.PlanRenewsAt,
			"days_until_renewal": data.DaysUntilRenewal,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(usage)
	}
}

func StorageAnalyticsHandler(cfg *AnalyticsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		data, err := cfg.Service.GetStorageAnalytics(r.Context(), userID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data)
	}
}

func ActivityAnalyticsHandler(cfg *AnalyticsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		data, err := cfg.Service.GetUserAnalytics(r.Context(), userID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"recent_activity": data.RecentActivity,
		})
	}
}

func AdminAnalyticsHandler(cfg *AnalyticsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		role, err := cfg.Queries.GetUserRole(r.Context(), pgUserID)
		if err != nil || role != db.UserRoleAdmin {
			apperror.WriteJSON(w, r, apperror.ErrForbidden)
			return
		}

		data, err := cfg.Service.GetAdminDashboard(r.Context())
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data)
	}
}

func AdminUsersAnalyticsHandler(cfg *AnalyticsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		role, err := cfg.Queries.GetUserRole(r.Context(), pgUserID)
		if err != nil || role != db.UserRoleAdmin {
			apperror.WriteJSON(w, r, apperror.ErrForbidden)
			return
		}

		data, err := cfg.Service.GetAdminDashboard(r.Context())
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_users":         data.TotalUsers,
			"new_users_this_week": data.NewUsersThisWeek,
			"users_by_plan":       data.UsersByPlan,
			"top_users":           data.TopUsers,
			"recent_signups":      data.RecentSignups,
		})
	}
}

func AdminHealthAnalyticsHandler(cfg *AnalyticsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		role, err := cfg.Queries.GetUserRole(r.Context(), pgUserID)
		if err != nil || role != db.UserRoleAdmin {
			apperror.WriteJSON(w, r, apperror.ErrForbidden)
			return
		}

		data, err := cfg.Service.GetAdminDashboard(r.Context())
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data.Health)
	}
}
