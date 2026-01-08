package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

const (
	StorageLimitFree       = 1 * 1024 * 1024 * 1024  // 1 GB
	StorageLimitPro        = 5 * 1024 * 1024 * 1024  // 5 GB
	StorageLimitEnterprise = 50 * 1024 * 1024 * 1024 // 50 GB
)

type Service struct {
	queries *db.Queries
	redis   *redis.Client
}

func NewService(queries *db.Queries, redis *redis.Client) *Service {
	return &Service{queries: queries, redis: redis}
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

	dbConns := 12

	allHealthy := queueSize < 1000 &&
		failedHour < 10 &&
		dbConns < 50

	return &SystemHealth{
		APILatencyP95:   47,
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
	return 0
}
