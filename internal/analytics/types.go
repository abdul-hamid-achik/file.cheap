package analytics

import (
	"time"

	"github.com/google/uuid"
)

type UserAnalytics struct {
	FilesUsed         int       `json:"files_used"`
	FilesLimit        int       `json:"files_limit"`
	TransformsUsed    int       `json:"transforms_used"`
	TransformsLimit   int       `json:"transforms_limit"`
	StorageUsedBytes  int64     `json:"storage_used_bytes"`
	StorageLimitBytes int64     `json:"storage_limit_bytes"`
	PlanName          string    `json:"plan_name"`
	PlanRenewsAt      time.Time `json:"plan_renews_at"`
	DaysUntilRenewal  int       `json:"days_until_renewal"`

	DailyUsage         []DailyUsage    `json:"daily_usage"`
	TransformBreakdown []TransformStat `json:"transform_breakdown"`
	TopFiles           []FileUsage     `json:"top_files"`
	RecentActivity     []ActivityItem  `json:"recent_activity"`
}

type DailyUsage struct {
	Date       time.Time `json:"date"`
	Uploads    int       `json:"uploads"`
	Transforms int       `json:"transforms"`
}

type TransformStat struct {
	Type       string  `json:"type"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

type FileUsage struct {
	FileID     string `json:"file_id"`
	Filename   string `json:"filename"`
	Transforms int    `json:"transforms"`
}

type ActivityItem struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	TimeAgo   string    `json:"time_ago"`
}

type AdminDashboard struct {
	MRR               float64 `json:"mrr"`
	MRRGrowth         float64 `json:"mrr_growth"`
	TotalUsers        int     `json:"total_users"`
	NewUsersThisWeek  int     `json:"new_users_this_week"`
	TotalFiles        int64   `json:"total_files"`
	TotalStorageBytes int64   `json:"total_storage_bytes"`
	JobsLast24h       int     `json:"jobs_last_24h"`
	JobSuccessRate    float64 `json:"job_success_rate"`

	RevenueHistory []RevenuePoint `json:"revenue_history"`
	UsersByPlan    []PlanStats    `json:"users_by_plan"`
	Health         SystemHealth   `json:"health"`
	TopUsers       []TopUser      `json:"top_users"`
	RecentSignups  []RecentSignup `json:"recent_signups"`
	FailedJobs     []FailedJob    `json:"failed_jobs"`
}

type RevenuePoint struct {
	Date  time.Time `json:"date"`
	MRR   float64   `json:"mrr"`
	Users int       `json:"users"`
}

type PlanStats struct {
	Plan       string  `json:"plan"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
	Revenue    float64 `json:"revenue"`
}

type SystemHealth struct {
	APILatencyP95   int64 `json:"api_latency_p95_ms"`
	WorkerQueueSize int   `json:"worker_queue_size"`
	FailedJobsHour  int   `json:"failed_jobs_hour"`
	StorageUsed     int64 `json:"storage_used_bytes"`
	DBConnections   int   `json:"db_connections"`
	RedisMemory     int64 `json:"redis_memory_bytes"`
	AllHealthy      bool  `json:"all_healthy"`
}

type TopUser struct {
	Email       string  `json:"email"`
	Plan        string  `json:"plan"`
	Transforms  int     `json:"transforms"`
	MonthlyRate float64 `json:"monthly_rate"`
}

type RecentSignup struct {
	Email     string    `json:"email"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
	TimeAgo   string    `json:"time_ago"`
}

type FailedJob struct {
	ID       string    `json:"id"`
	FileID   uuid.UUID `json:"file_id"`
	JobType  string    `json:"job_type"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failed_at"`
	CanRetry bool      `json:"can_retry"`
}

type JobListItem struct {
	ID           string     `json:"id"`
	FileID       string     `json:"file_id"`
	Filename     string     `json:"filename"`
	JobType      string     `json:"job_type"`
	Status       string     `json:"status"`
	Priority     int        `json:"priority"`
	Attempts     int        `json:"attempts"`
	ErrorMessage string     `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
}

type JobsListPage struct {
	Jobs       []JobListItem `json:"jobs"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
	Status     string        `json:"status"`
}
