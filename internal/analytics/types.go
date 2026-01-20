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

	Churn          ChurnMetrics   `json:"churn"`
	Revenue        RevenueMetrics `json:"revenue"`
	NRR            NRRMetrics     `json:"nrr"`
	CohortData     []CohortData   `json:"cohort_data"`
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

type ChurnMetrics struct {
	ChurnedThisMonth int64   `json:"churned_this_month"`
	Churned30Days    int64   `json:"churned_30_days"`
	CurrentActive    int64   `json:"current_active"`
	MonthlyChurnRate float64 `json:"monthly_churn_rate"`
	RetentionRate    float64 `json:"retention_rate"`
}

type RevenueMetrics struct {
	MRR          float64 `json:"mrr"`
	ARR          float64 `json:"arr"`
	ARPU         float64 `json:"arpu"`
	EstimatedLTV float64 `json:"estimated_ltv"`
	PayingUsers  int64   `json:"paying_users"`
}

type NRRMetrics struct {
	PreviousMRR float64 `json:"previous_mrr"`
	CurrentMRR  float64 `json:"current_mrr"`
	NRRPercent  float64 `json:"nrr_percent"`
}

type CohortData struct {
	CohortMonth  time.Time `json:"cohort_month"`
	CohortSize   int64     `json:"cohort_size"`
	MonthsSince  int       `json:"months_since"`
	Retained     int64     `json:"retained"`
	RetentionPct float64   `json:"retention_pct"`
}

type Notification struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	Link      string     `json:"link,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	TimeAgo   string     `json:"time_ago"`
}

type OnboardingProgress struct {
	UploadedFirstFile   bool `json:"uploaded_first_file"`
	CreatedTransform    bool `json:"created_transform"`
	GeneratedAPIToken   bool `json:"generated_api_token"`
	SetupWebhook        bool `json:"setup_webhook"`
	CompletedOnboarding bool `json:"completed_onboarding"`
}

type AlertConfig struct {
	ID             string    `json:"id"`
	MetricName     string    `json:"metric_name"`
	ThresholdValue float64   `json:"threshold_value"`
	Enabled        bool      `json:"enabled"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ExportData struct {
	UserAnalytics *UserAnalytics `json:"user_analytics,omitempty"`
	DailyUsage    []DailyUsage   `json:"daily_usage,omitempty"`
	TopFiles      []FileUsage    `json:"top_files,omitempty"`
	ExportedAt    time.Time      `json:"exported_at"`
}

type StorageBreakdownByType struct {
	FileType   string `json:"file_type"`
	FileCount  int64  `json:"file_count"`
	TotalBytes int64  `json:"total_bytes"`
}

type StorageBreakdownByVariant struct {
	VariantType  string `json:"variant_type"`
	VariantCount int64  `json:"variant_count"`
	TotalBytes   int64  `json:"total_bytes"`
}

type LargestFile struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

type StorageAnalytics struct {
	BreakdownByType    []StorageBreakdownByType    `json:"breakdown_by_type"`
	BreakdownByVariant []StorageBreakdownByVariant `json:"breakdown_by_variant"`
	LargestFiles       []LargestFile               `json:"largest_files"`
	TotalBytes         int64                       `json:"total_bytes"`
}

// Enhanced Analytics Types

// ProcessingVolumePoint represents processing volume for a specific day and job type
type ProcessingVolumePoint struct {
	Date                 time.Time `json:"date"`
	JobType              string    `json:"job_type"`
	Count                int64     `json:"count"`
	TotalDurationSeconds int64     `json:"total_duration_seconds"`
}

// StorageGrowthPoint represents storage growth for a specific day
type StorageGrowthPoint struct {
	Date            time.Time `json:"date"`
	CumulativeBytes int64     `json:"cumulative_bytes"`
	BytesAdded      int64     `json:"bytes_added"`
	FilesAdded      int64     `json:"files_added"`
}

// VideoProcessingStats contains video processing statistics
type VideoProcessingStats struct {
	TotalVideoFiles              int64   `json:"total_video_files"`
	TotalVideoBytes              int64   `json:"total_video_bytes"`
	TotalVideoJobs               int64   `json:"total_video_jobs"`
	TranscodeJobs                int64   `json:"transcode_jobs"`
	HLSJobs                      int64   `json:"hls_jobs"`
	VideoThumbnailJobs           int64   `json:"video_thumbnail_jobs"`
	AvgTranscodeDurationSeconds  float64 `json:"avg_transcode_duration_seconds"`
	TotalVideoProcessingSeconds  int64   `json:"total_video_processing_seconds"`
	EstimatedVideoMinutes        float64 `json:"estimated_video_minutes"`
	EstimatedMonthlyCost         float64 `json:"estimated_monthly_cost"`
}

// ProcessingVolumeByTier represents processing volume grouped by subscription tier
type ProcessingVolumeByTier struct {
	Tier                 string `json:"tier"`
	JobType              string `json:"job_type"`
	Count                int64  `json:"count"`
	TotalDurationSeconds int64  `json:"total_duration_seconds"`
}

// BandwidthStats contains bandwidth/download statistics
type BandwidthStats struct {
	TotalDownloads         int64   `json:"total_downloads"`
	EstimatedBandwidthBytes int64   `json:"estimated_bandwidth_bytes"`
	EstimatedBandwidthGB    float64 `json:"estimated_bandwidth_gb"`
}

// CostForecast contains data for cost forecasting
type CostForecast struct {
	// Storage
	TotalStorageBytes     int64   `json:"total_storage_bytes"`
	TotalStorageGB        float64 `json:"total_storage_gb"`
	TotalFiles            int64   `json:"total_files"`
	EstimatedStorageCost  float64 `json:"estimated_storage_cost"`

	// Processing
	JobsLast30Days               int64   `json:"jobs_last_30_days"`
	VideoJobsLast30Days          int64   `json:"video_jobs_last_30_days"`
	VideoProcessingSeconds30Days int64   `json:"video_processing_seconds_30_days"`
	VideoMinutes30Days           float64 `json:"video_minutes_30_days"`
	EstimatedProcessingCost      float64 `json:"estimated_processing_cost"`

	// Bandwidth
	Downloads30Days           int64   `json:"downloads_30_days"`
	EstimatedBandwidthCost    float64 `json:"estimated_bandwidth_cost"`

	// Totals
	TotalEstimatedMonthlyCost float64 `json:"total_estimated_monthly_cost"`
}

// EnhancedAnalytics contains all enhanced analytics data
type EnhancedAnalytics struct {
	ProcessingVolume    []ProcessingVolumePoint  `json:"processing_volume"`
	StorageGrowth       []StorageGrowthPoint     `json:"storage_growth"`
	VideoStats          VideoProcessingStats     `json:"video_stats"`
	BandwidthStats      BandwidthStats           `json:"bandwidth_stats"`
	CostForecast        CostForecast             `json:"cost_forecast"`
	ProcessingByTier    []ProcessingVolumeByTier `json:"processing_by_tier"`
}
