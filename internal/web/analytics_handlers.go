package web

import (
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/analytics"
	"github.com/abdul-hamid-achik/file.cheap/internal/auth"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/web/templates/pages"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type AnalyticsHandlers struct {
	service *analytics.Service
}

func NewAnalyticsHandlers(service *analytics.Service) *AnalyticsHandlers {
	return &AnalyticsHandlers{service: service}
}

func NewAnalyticsService(cfg *Config, redis *redis.Client, poolStats analytics.PoolStats) *analytics.Service {
	svc := analytics.NewService(cfg.Queries, redis)
	if poolStats != nil {
		svc.SetPoolStats(poolStats)
	}
	return svc
}

func (h *AnalyticsHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data, err := h.service.GetUserAnalytics(r.Context(), user.ID)
	if err != nil {
		log.Error("failed to get analytics", "error", err)
		http.Error(w, "Failed to load analytics", http.StatusInternalServerError)
		return
	}

	_ = pages.AnalyticsPage(user, data).Render(r.Context(), w)
}

func (h *AnalyticsHandlers) UsageChart(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data, err := h.service.GetUserAnalytics(r.Context(), user.ID)
	if err != nil {
		log.Error("failed to get analytics for chart", "error", err)
		http.Error(w, "Chart error", http.StatusInternalServerError)
		return
	}

	chartBytes, err := h.service.GenerateUsageChart(data.DailyUsage, 800, 288)
	if err != nil {
		log.Error("failed to generate usage chart", "error", err)
		http.Error(w, "Chart error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=300")
	_, _ = w.Write(chartBytes)
}

func (h *AnalyticsHandlers) TransformsChart(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data, err := h.service.GetUserAnalytics(r.Context(), user.ID)
	if err != nil {
		log.Error("failed to get analytics for pie chart", "error", err)
		http.Error(w, "Chart error", http.StatusInternalServerError)
		return
	}

	chartBytes, err := h.service.GeneratePieChart(data.TransformBreakdown, 160, 160)
	if err != nil {
		log.Error("failed to generate pie chart", "error", err)
		http.Error(w, "Chart error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=300")
	_, _ = w.Write(chartBytes)
}

func (h *AnalyticsHandlers) ActivityFeed(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data, err := h.service.GetUserAnalytics(r.Context(), user.ID)
	if err != nil {
		log.Error("failed to get activity feed", "error", err)
		http.Error(w, "Failed to load activity", http.StatusInternalServerError)
		return
	}

	_ = pages.AnalyticsActivityFeed(data.RecentActivity).Render(r.Context(), w)
}

func (h *AnalyticsHandlers) ChartPartial(w http.ResponseWriter, r *http.Request) {
	_ = pages.AnalyticsUsageChart().Render(r.Context(), w)
}

type AdminHandlers struct {
	service *analytics.Service
}

func NewAdminHandlers(service *analytics.Service) *AdminHandlers {
	return &AdminHandlers{service: service}
}

func (h *AdminHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data, err := h.service.GetAdminDashboard(r.Context())
	if err != nil {
		log.Error("failed to get admin dashboard", "error", err)
		http.Error(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}

	_ = pages.AdminDashboard(user, data).Render(r.Context(), w)
}

func (h *AdminHandlers) RevenueChart(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	data, err := h.service.GetAdminDashboard(r.Context())
	if err != nil {
		log.Error("failed to get admin data for chart", "error", err)
		http.Error(w, "Chart error", http.StatusInternalServerError)
		return
	}

	chartBytes, err := h.service.GenerateRevenueChart(data.RevenueHistory, 900, 288)
	if err != nil {
		log.Error("failed to generate revenue chart", "error", err)
		http.Error(w, "Chart error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=60")
	_, _ = w.Write(chartBytes)
}

func (h *AdminHandlers) HealthStatus(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	data, err := h.service.GetAdminDashboard(r.Context())
	if err != nil {
		log.Error("failed to get health status", "error", err)
		http.Error(w, "Failed to load health", http.StatusInternalServerError)
		return
	}

	_ = pages.AdminHealthRows(data.Health).Render(r.Context(), w)
}

func (h *AdminHandlers) RecentSignups(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	data, err := h.service.GetAdminDashboard(r.Context())
	if err != nil {
		log.Error("failed to get recent signups", "error", err)
		http.Error(w, "Failed to load signups", http.StatusInternalServerError)
		return
	}

	_ = pages.AdminSignupRows(data.RecentSignups).Render(r.Context(), w)
}

func (h *AdminHandlers) RetryJob(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	jobIDStr := r.PathValue("id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		log.Error("invalid job ID", "job_id", jobIDStr, "error", err)
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	if err := h.service.RetryJob(r.Context(), jobID); err != nil {
		log.Error("failed to retry job", "job_id", jobIDStr, "error", err)
		http.Error(w, "Failed to retry job", http.StatusInternalServerError)
		return
	}

	log.Info("job retry requested", "job_id", jobIDStr)

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<div class="flex items-center justify-center py-2 px-3 bg-nord-14/10 rounded-lg text-nord-14 text-sm">Job queued for retry</div>`))
}

func (h *AdminHandlers) CancelJob(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	jobIDStr := r.PathValue("id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		log.Error("invalid job ID", "job_id", jobIDStr, "error", err)
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	if err := h.service.CancelJob(r.Context(), jobID); err != nil {
		log.Error("failed to cancel job", "job_id", jobIDStr, "error", err)
		http.Error(w, "Failed to cancel job", http.StatusInternalServerError)
		return
	}

	log.Info("job cancelled", "job_id", jobIDStr)

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<div class="flex items-center justify-center py-2 px-3 bg-nord-11/10 rounded-lg text-nord-11 text-sm">Job cancelled</div>`))
}

func (h *AdminHandlers) Jobs(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	status := r.URL.Query().Get("status")
	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	data, err := h.service.GetJobsList(r.Context(), status, page, 50)
	if err != nil {
		log.Error("failed to get jobs list", "error", err)
		http.Error(w, "Failed to load jobs", http.StatusInternalServerError)
		return
	}

	_ = pages.AdminJobs(user, data).Render(r.Context(), w)
}

func (h *AnalyticsHandlers) ExportData(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	data, err := h.service.GetUserAnalytics(r.Context(), user.ID)
	if err != nil {
		log.Error("failed to get analytics for export", "error", err)
		http.Error(w, "Failed to export data", http.StatusInternalServerError)
		return
	}

	var exportData []byte
	var contentType string
	var filename string

	switch format {
	case "csv":
		exportData, err = analytics.ExportUserAnalyticsCSV(data)
		contentType = "text/csv"
		filename = "analytics-export.csv"
	default:
		exportData, err = analytics.ExportUserAnalyticsJSON(data)
		contentType = "application/json"
		filename = "analytics-export.json"
	}

	if err != nil {
		log.Error("failed to export analytics", "format", format, "error", err)
		http.Error(w, "Export failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(exportData)
}

func (h *AdminHandlers) ExportDashboard(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	data, err := h.service.GetAdminDashboard(r.Context())
	if err != nil {
		log.Error("failed to get admin dashboard for export", "error", err)
		http.Error(w, "Failed to export data", http.StatusInternalServerError)
		return
	}

	var exportData []byte
	var contentType string
	var filename string

	switch format {
	case "csv":
		exportData, err = analytics.ExportAdminDashboardCSV(data)
		contentType = "text/csv"
		filename = "admin-dashboard-export.csv"
	default:
		exportData, err = analytics.ExportAdminDashboardJSON(data)
		contentType = "application/json"
		filename = "admin-dashboard-export.json"
	}

	if err != nil {
		log.Error("failed to export admin dashboard", "format", format, "error", err)
		http.Error(w, "Export failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(exportData)
}
