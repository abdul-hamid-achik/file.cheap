package analytics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

const (
	StorageLimitFree       = 1 * 1024 * 1024 * 1024  // 1 GB
	StorageLimitPro        = 5 * 1024 * 1024 * 1024  // 5 GB
	StorageLimitEnterprise = 50 * 1024 * 1024 * 1024 // 50 GB
)

type PoolStats interface {
	AcquiredConns() int32
	TotalConns() int32
}

type PoolStatsFunc func() PoolStats

type poolStatsFuncAdapter struct {
	fn PoolStatsFunc
}

func (p *poolStatsFuncAdapter) AcquiredConns() int32 {
	return p.fn().AcquiredConns()
}

func (p *poolStatsFuncAdapter) TotalConns() int32 {
	return p.fn().TotalConns()
}

func NewPoolStatsFunc(fn PoolStatsFunc) PoolStats {
	return &poolStatsFuncAdapter{fn: fn}
}

type Service struct {
	queries   *db.Queries
	redis     *redis.Client
	poolStats PoolStats
}

func NewService(queries *db.Queries, redis *redis.Client) *Service {
	return &Service{queries: queries, redis: redis}
}

func (s *Service) SetPoolStats(ps PoolStats) {
	s.poolStats = ps
}

func (s *Service) GetUserAnalytics(ctx context.Context, userID uuid.UUID) (*UserAnalytics, error) {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	stats, err := s.queries.GetUserUsageStats(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("get usage stats: %w", err)
	}

	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	dailyUsage, err := s.queries.GetDailyUsage(ctx, db.GetDailyUsageParams{
		UserID:  pgUserID,
		Column2: pgtype.Date{Time: thirtyDaysAgo, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("get daily usage: %w", err)
	}

	breakdown, err := s.queries.GetTransformBreakdown(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("get transform breakdown: %w", err)
	}

	var totalTransforms int64
	for _, b := range breakdown {
		totalTransforms += b.Count
	}

	transformStats := make([]TransformStat, len(breakdown))
	for i, b := range breakdown {
		pct := 0.0
		if totalTransforms > 0 {
			pct = float64(b.Count) / float64(totalTransforms) * 100
		}
		transformStats[i] = TransformStat{
			Type:       b.Type,
			Count:      int(b.Count),
			Percentage: pct,
		}
	}

	topFiles, err := s.queries.GetTopFilesByTransforms(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("get top files: %w", err)
	}

	activity, err := s.queries.GetRecentActivity(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("get recent activity: %w", err)
	}

	activityItems := make([]ActivityItem, len(activity))
	for i, a := range activity {
		msg := ""
		if a.Message != nil {
			msg = fmt.Sprintf("%v", a.Message)
		}
		activityItems[i] = ActivityItem{
			ID:        a.ID,
			Type:      a.Type,
			Message:   msg,
			Status:    a.Status,
			CreatedAt: a.CreatedAt.Time,
			TimeAgo:   TimeAgo(a.CreatedAt.Time),
		}
	}

	dailyUsageItems := make([]DailyUsage, len(dailyUsage))
	for i, d := range dailyUsage {
		dailyUsageItems[i] = DailyUsage{
			Date:       d.Date.Time,
			Uploads:    int(d.Uploads),
			Transforms: int(d.Transforms),
		}
	}

	topFilesItems := make([]FileUsage, len(topFiles))
	for i, f := range topFiles {
		topFilesItems[i] = FileUsage{
			FileID:     uuidToString(f.FileID),
			Filename:   f.Filename,
			Transforms: int(f.Transforms),
		}
	}

	storageLimitBytes := getStorageLimit(string(stats.PlanName))

	renewsAt := stats.PlanRenewsAt.Time
	if !stats.PlanRenewsAt.Valid {
		renewsAt = time.Now().AddDate(0, 1, 0)
	}

	daysUntilRenewal := int(time.Until(renewsAt).Hours() / 24)
	if daysUntilRenewal < 0 {
		daysUntilRenewal = 0
	}

	return &UserAnalytics{
		FilesUsed:          int(stats.FilesUsed),
		FilesLimit:         int(stats.FilesLimit),
		TransformsUsed:     int(stats.TransformsUsed),
		TransformsLimit:    int(stats.TransformsLimit),
		StorageUsedBytes:   stats.StorageUsedBytes,
		StorageLimitBytes:  storageLimitBytes,
		PlanName:           string(stats.PlanName),
		PlanRenewsAt:       renewsAt,
		DaysUntilRenewal:   daysUntilRenewal,
		DailyUsage:         dailyUsageItems,
		TransformBreakdown: transformStats,
		TopFiles:           topFilesItems,
		RecentActivity:     activityItems,
	}, nil
}

func (s *Service) GetAdminDashboard(ctx context.Context) (*AdminDashboard, error) {
	metrics, err := s.queries.GetAdminMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("get admin metrics: %w", err)
	}

	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	revenueHistory, err := s.queries.GetMRRHistory(ctx, pgtype.Timestamptz{Time: thirtyDaysAgo, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("get mrr history: %w", err)
	}

	var mrrGrowth float64
	if len(revenueHistory) >= 2 {
		first := revenueHistory[0].Mrr
		last := revenueHistory[len(revenueHistory)-1].Mrr
		if first > 0 {
			mrrGrowth = (last - first) / first * 100
		}
	}

	planStats, err := s.queries.GetUsersByPlan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get users by plan: %w", err)
	}

	usersByPlan := make([]PlanStats, len(planStats))
	totalUsers := int(metrics.TotalUsers)
	for i, p := range planStats {
		pct := 0.0
		if totalUsers > 0 {
			pct = float64(p.Count) / float64(totalUsers) * 100
		}
		revenue := 0.0
		switch p.Plan {
		case "pro":
			revenue = float64(p.Count) * 19.0
		case "enterprise":
			revenue = float64(p.Count) * 99.0
		}
		usersByPlan[i] = PlanStats{
			Plan:       p.Plan,
			Count:      int(p.Count),
			Percentage: pct,
			Revenue:    revenue,
		}
	}

	fileStats, err := s.queries.GetTotalFilesAndStorage(ctx)
	if err != nil {
		return nil, fmt.Errorf("get file stats: %w", err)
	}

	jobStats, err := s.queries.GetJobStats24h(ctx)
	if err != nil {
		return nil, fmt.Errorf("get job stats: %w", err)
	}

	successRate := 0.0
	if jobStats.TotalJobs > 0 {
		successRate = float64(jobStats.Completed) / float64(jobStats.TotalJobs) * 100
	}

	health, err := s.getSystemHealth(ctx, fileStats.TotalStorageBytes)
	if err != nil {
		return nil, fmt.Errorf("get system health: %w", err)
	}

	topUsers, err := s.queries.GetTopUsersByUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("get top users: %w", err)
	}

	signups, err := s.queries.GetRecentSignups(ctx)
	if err != nil {
		return nil, fmt.Errorf("get recent signups: %w", err)
	}

	failedJobs, err := s.queries.GetFailedJobs24h(ctx)
	if err != nil {
		return nil, fmt.Errorf("get failed jobs: %w", err)
	}

	revenuePoints := make([]RevenuePoint, len(revenueHistory))
	for i, r := range revenueHistory {
		revenuePoints[i] = RevenuePoint{
			Date:  r.Date.Time,
			MRR:   r.Mrr,
			Users: int(r.Users),
		}
	}

	topUserItems := make([]TopUser, len(topUsers))
	for i, u := range topUsers {
		topUserItems[i] = TopUser{
			Email:       u.Email,
			Plan:        u.Plan,
			Transforms:  int(u.Transforms),
			MonthlyRate: u.MonthlyRate,
		}
	}

	signupItems := make([]RecentSignup, len(signups))
	for i, s := range signups {
		signupItems[i] = RecentSignup{
			Email:     s.Email,
			Plan:      s.Plan,
			CreatedAt: s.CreatedAt.Time,
			TimeAgo:   TimeAgo(s.CreatedAt.Time),
		}
	}

	failedJobItems := make([]FailedJob, len(failedJobs))
	for i, j := range failedJobs {
		failedJobItems[i] = FailedJob{
			ID:       j.ID.String(),
			FileID:   uuid.UUID(j.FileID.Bytes),
			JobType:  j.JobType,
			Error:    j.Error,
			FailedAt: j.FailedAt.Time,
			CanRetry: j.CanRetry,
		}
	}

	churn, err := s.GetChurnMetrics(ctx)
	if err != nil {
		churn = &ChurnMetrics{}
	}

	revenue, err := s.GetRevenueMetrics(ctx)
	if err != nil {
		revenue = &RevenueMetrics{}
	}

	nrr, err := s.GetNRRMetrics(ctx)
	if err != nil {
		nrr = &NRRMetrics{}
	}

	cohorts, err := s.GetCohortAnalysis(ctx)
	if err != nil {
		cohorts = []CohortData{}
	}

	return &AdminDashboard{
		MRR:               metrics.Mrr,
		MRRGrowth:         mrrGrowth,
		TotalUsers:        totalUsers,
		NewUsersThisWeek:  int(metrics.NewUsersThisWeek),
		TotalFiles:        fileStats.TotalFiles,
		TotalStorageBytes: fileStats.TotalStorageBytes,
		JobsLast24h:       int(jobStats.TotalJobs),
		JobSuccessRate:    successRate,
		RevenueHistory:    revenuePoints,
		UsersByPlan:       usersByPlan,
		Health:            *health,
		TopUsers:          topUserItems,
		RecentSignups:     signupItems,
		FailedJobs:        failedJobItems,
		Churn:             *churn,
		Revenue:           *revenue,
		NRR:               *nrr,
		CohortData:        cohorts,
	}, nil
}

func (s *Service) getSystemHealth(ctx context.Context, storageUsed int64) (*SystemHealth, error) {
	queueSize, _ := s.queries.GetWorkerQueueSize(ctx)
	failedHour, _ := s.queries.GetFailedJobsLastHour(ctx)

	var redisMemory int64
	if s.redis != nil {
		info, err := s.redis.Info(ctx, "memory").Result()
		if err == nil {
			redisMemory = parseRedisMemory(info)
		}
	}

	var dbConns int
	if s.poolStats != nil {
		dbConns = int(s.poolStats.AcquiredConns())
	}

	var apiLatency int64 = 0
	if s.redis != nil {
		if latencyStr, err := s.redis.Get(ctx, "metrics:api_latency_p95").Result(); err == nil {
			if latency, err := strconv.ParseInt(latencyStr, 10, 64); err == nil {
				apiLatency = latency
			}
		}
	}

	allHealthy := queueSize < 1000 &&
		failedHour < 10 &&
		dbConns < 50 &&
		apiLatency < 200

	return &SystemHealth{
		APILatencyP95:   apiLatency,
		WorkerQueueSize: int(queueSize),
		FailedJobsHour:  int(failedHour),
		StorageUsed:     storageUsed,
		DBConnections:   dbConns,
		RedisMemory:     redisMemory,
		AllHealthy:      allHealthy,
	}, nil
}

func (s *Service) RetryJob(ctx context.Context, jobID uuid.UUID) error {
	return s.queries.RetryFailedJob(ctx, pgtype.UUID{Bytes: jobID, Valid: true})
}

func TimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func getStorageLimit(tier string) int64 {
	switch tier {
	case "pro":
		return StorageLimitPro
	case "enterprise":
		return StorageLimitEnterprise
	default:
		return StorageLimitFree
	}
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func parseRedisMemory(info string) int64 {
	lines := strings.Split(info, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "used_memory:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				if val, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
					return val
				}
			}
		}
	}
	return 0
}

func (s *Service) GetChurnMetrics(ctx context.Context) (*ChurnMetrics, error) {
	metrics, err := s.queries.GetChurnMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("get churn metrics: %w", err)
	}

	return &ChurnMetrics{
		ChurnedThisMonth: metrics.ChurnedThisMonth,
		Churned30Days:    metrics.Churned30d,
		CurrentActive:    metrics.CurrentActive,
		MonthlyChurnRate: float64(metrics.MonthlyChurnRate),
		RetentionRate:    float64(metrics.RetentionRate),
	}, nil
}

func (s *Service) GetRevenueMetrics(ctx context.Context) (*RevenueMetrics, error) {
	metrics, err := s.queries.GetRevenueMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("get revenue metrics: %w", err)
	}

	estimatedLTV := 0.0
	if ltv, ok := metrics.EstimatedLtv.(float64); ok {
		estimatedLTV = ltv
	}

	return &RevenueMetrics{
		MRR:          metrics.Mrr,
		ARR:          metrics.Arr,
		ARPU:         float64(metrics.Arpu),
		EstimatedLTV: estimatedLTV,
		PayingUsers:  metrics.PayingUsers,
	}, nil
}

func (s *Service) GetNRRMetrics(ctx context.Context) (*NRRMetrics, error) {
	metrics, err := s.queries.GetNRR(ctx)
	if err != nil {
		return nil, fmt.Errorf("get nrr metrics: %w", err)
	}

	return &NRRMetrics{
		PreviousMRR: metrics.PreviousMrr,
		CurrentMRR:  metrics.CurrentMrr,
		NRRPercent:  float64(metrics.NrrPercent),
	}, nil
}

func (s *Service) GetCohortAnalysis(ctx context.Context) ([]CohortData, error) {
	rows, err := s.queries.GetCohortRetention(ctx)
	if err != nil {
		return nil, fmt.Errorf("get cohort retention: %w", err)
	}

	cohorts := make([]CohortData, len(rows))
	for i, r := range rows {
		cohorts[i] = CohortData{
			CohortMonth:  r.CohortMonth.Time,
			CohortSize:   r.CsCohortSize,
			MonthsSince:  int(r.RMonthsSince),
			Retained:     r.RRetained,
			RetentionPct: float64(r.RetentionPct),
		}
	}

	return cohorts, nil
}

func (s *Service) GetAlertConfigs(ctx context.Context) ([]AlertConfig, error) {
	rows, err := s.queries.GetAlertConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get alert configs: %w", err)
	}

	configs := make([]AlertConfig, len(rows))
	for i, r := range rows {
		enabled := false
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		configs[i] = AlertConfig{
			ID:             uuidToString(r.ID),
			MetricName:     r.MetricName,
			ThresholdValue: r.ThresholdValue,
			Enabled:        enabled,
			UpdatedAt:      r.UpdatedAt.Time,
		}
	}

	return configs, nil
}

func (s *Service) UpdateAlertThreshold(ctx context.Context, metricName string, threshold float64, enabled bool) error {
	return s.queries.UpdateAlertThreshold(ctx, db.UpdateAlertThresholdParams{
		MetricName:     metricName,
		ThresholdValue: threshold,
		Enabled:        &enabled,
	})
}

func (s *Service) GetStorageAnalytics(ctx context.Context, userID uuid.UUID) (*StorageAnalytics, error) {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	byType, err := s.queries.GetStorageBreakdownByType(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("get storage breakdown by type: %w", err)
	}

	byVariant, err := s.queries.GetStorageBreakdownByVariant(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("get storage breakdown by variant: %w", err)
	}

	largest, err := s.queries.GetLargestFiles(ctx, db.GetLargestFilesParams{
		UserID: pgUserID,
		Limit:  10,
	})
	if err != nil {
		return nil, fmt.Errorf("get largest files: %w", err)
	}

	total, err := s.queries.GetTotalStorageByUser(ctx, pgUserID)
	if err != nil {
		return nil, fmt.Errorf("get total storage: %w", err)
	}

	typeBreakdown := make([]StorageBreakdownByType, len(byType))
	for i, t := range byType {
		typeBreakdown[i] = StorageBreakdownByType{
			FileType:   t.FileType,
			FileCount:  t.FileCount,
			TotalBytes: t.TotalBytes,
		}
	}

	variantBreakdown := make([]StorageBreakdownByVariant, len(byVariant))
	for i, v := range byVariant {
		variantBreakdown[i] = StorageBreakdownByVariant{
			VariantType:  v.VariantType,
			VariantCount: v.VariantCount,
			TotalBytes:   v.TotalBytes,
		}
	}

	largestFiles := make([]LargestFile, len(largest))
	for i, f := range largest {
		largestFiles[i] = LargestFile{
			ID:          uuidToString(f.ID),
			Filename:    f.Filename,
			ContentType: f.ContentType,
			SizeBytes:   f.SizeBytes,
			CreatedAt:   f.CreatedAt.Time,
		}
	}

	return &StorageAnalytics{
		BreakdownByType:    typeBreakdown,
		BreakdownByVariant: variantBreakdown,
		LargestFiles:       largestFiles,
		TotalBytes:         total,
	}, nil
}

func (s *Service) GetJobsList(ctx context.Context, status string, page, pageSize int) (*JobsListPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	var statusPtr *string
	if status != "" && status != "all" {
		statusPtr = &status
	}

	total, err := s.queries.CountJobsAdmin(ctx, statusPtr)
	if err != nil {
		return nil, fmt.Errorf("failed to count jobs: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := s.queries.ListJobsAdmin(ctx, db.ListJobsAdminParams{
		Status: statusPtr,
		Limit:  int32(pageSize),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	jobs := make([]JobListItem, len(rows))
	for i, r := range rows {
		job := JobListItem{
			ID:           uuidToString(r.ID),
			FileID:       uuidToString(r.FileID),
			JobType:      r.JobType,
			Status:       r.Status,
			Priority:     int(r.Priority),
			Attempts:     int(r.Attempts),
			ErrorMessage: r.ErrorMessage,
			CreatedAt:    r.CreatedAt.Time,
		}
		if r.Filename != nil {
			job.Filename = *r.Filename
		}
		if r.StartedAt.Valid {
			job.StartedAt = &r.StartedAt.Time
		}
		if r.CompletedAt.Valid {
			job.CompletedAt = &r.CompletedAt.Time
		}
		jobs[i] = job
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &JobsListPage{
		Jobs:       jobs,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		Status:     status,
	}, nil
}

func (s *Service) ListUsersForAdmin(ctx context.Context, search string, limit, offset int32) ([]db.ListUsersForAdminRow, error) {
	return s.queries.ListUsersForAdmin(ctx, db.ListUsersForAdminParams{
		Column1: search,
		Limit:   limit,
		Offset:  offset,
	})
}

func (s *Service) CountUsersForAdmin(ctx context.Context, search string) (int64, error) {
	return s.queries.CountUsersForAdmin(ctx, search)
}

func (s *Service) UpdateUserTier(ctx context.Context, userID pgtype.UUID, tier db.SubscriptionTier) (*db.User, error) {
	user, err := s.queries.UpdateUserTier(ctx, db.UpdateUserTierParams{
		ID:               userID,
		SubscriptionTier: tier,
	})
	if err != nil {
		return nil, fmt.Errorf("update user tier: %w", err)
	}
	return &user, nil
}

func (s *Service) UpdateUserToEnterprise(ctx context.Context, userID pgtype.UUID) (*db.User, error) {
	user, err := s.queries.UpdateUserToEnterprise(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("update user to enterprise: %w", err)
	}
	return &user, nil
}

func (s *Service) ListEnterpriseInquiries(ctx context.Context, status string, limit, offset int32) ([]db.EnterpriseInquiry, error) {
	if status == "" || status == "all" {
		return s.queries.ListEnterpriseInquiries(ctx, db.ListEnterpriseInquiriesParams{
			Limit:  limit,
			Offset: offset,
		})
	}
	return s.queries.ListEnterpriseInquiriesByStatus(ctx, db.ListEnterpriseInquiriesByStatusParams{
		Status: status,
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) CountEnterpriseInquiries(ctx context.Context, status string) (int64, error) {
	if status == "" || status == "all" {
		return s.queries.CountEnterpriseInquiries(ctx)
	}
	return s.queries.CountEnterpriseInquiriesByStatus(ctx, status)
}

func (s *Service) UpdateEnterpriseInquiryStatus(ctx context.Context, inquiryID pgtype.UUID, status string, notes *string, processedBy pgtype.UUID) (*db.EnterpriseInquiry, error) {
	inquiry, err := s.queries.UpdateEnterpriseInquiryStatus(ctx, db.UpdateEnterpriseInquiryStatusParams{
		ID:          inquiryID,
		Status:      status,
		AdminNotes:  notes,
		ProcessedBy: processedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("update inquiry status: %w", err)
	}
	return &inquiry, nil
}
