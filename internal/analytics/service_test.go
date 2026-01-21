package analytics

import (
	"testing"
	"time"
)

func TestParseRedisMemory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "valid memory info",
			input:    "# Memory\r\nused_memory:1234567\r\nused_memory_human:1.18M\r\n",
			expected: 1234567,
		},
		{
			name:     "memory info with unix line endings",
			input:    "# Memory\nused_memory:9876543\nused_memory_human:9.42M\n",
			expected: 9876543,
		},
		{
			name:     "memory info with spaces",
			input:    "used_memory: 5555555 \n",
			expected: 5555555,
		},
		{
			name:     "invalid number",
			input:    "used_memory:not_a_number\n",
			expected: 0,
		},
		{
			name:     "missing used_memory",
			input:    "used_memory_peak:1234567\n",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRedisMemory(tt.input)
			if result != tt.expected {
				t.Errorf("parseRedisMemory(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "just now",
			duration: 30 * time.Second,
			expected: "just now",
		},
		{
			name:     "1 minute",
			duration: 1*time.Minute + 30*time.Second,
			expected: "1 min ago",
		},
		{
			name:     "multiple minutes",
			duration: 5 * time.Minute,
			expected: "5 min ago",
		},
		{
			name:     "1 hour",
			duration: 1*time.Hour + 15*time.Minute,
			expected: "1 hour ago",
		},
		{
			name:     "multiple hours",
			duration: 3 * time.Hour,
			expected: "3 hours ago",
		},
		{
			name:     "1 day",
			duration: 25 * time.Hour,
			expected: "1 day ago",
		},
		{
			name:     "multiple days",
			duration: 72 * time.Hour,
			expected: "3 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Now().Add(-tt.duration)
			result := TimeAgo(testTime)
			if result != tt.expected {
				t.Errorf("TimeAgo() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetStorageLimit(t *testing.T) {
	tests := []struct {
		tier     string
		expected int64
	}{
		{"free", StorageLimitFree},
		{"pro", StorageLimitPro},
		{"enterprise", StorageLimitEnterprise},
		{"unknown", StorageLimitFree},
		{"", StorageLimitFree},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			result := getStorageLimit(tt.tier)
			if result != tt.expected {
				t.Errorf("getStorageLimit(%q) = %d, want %d", tt.tier, result, tt.expected)
			}
		})
	}
}

type mockPoolStats struct {
	acquired int32
	total    int32
}

func (m *mockPoolStats) AcquiredConns() int32 { return m.acquired }
func (m *mockPoolStats) TotalConns() int32    { return m.total }

func TestPoolStatsFunc(t *testing.T) {
	mock := &mockPoolStats{acquired: 5, total: 10}
	adapter := NewPoolStatsFunc(func() PoolStats { return mock })

	if adapter.AcquiredConns() != 5 {
		t.Errorf("AcquiredConns() = %d, want 5", adapter.AcquiredConns())
	}
	if adapter.TotalConns() != 10 {
		t.Errorf("TotalConns() = %d, want 10", adapter.TotalConns())
	}

	mock.acquired = 8
	if adapter.AcquiredConns() != 8 {
		t.Errorf("AcquiredConns() after update = %d, want 8", adapter.AcquiredConns())
	}
}

func TestCostConstants(t *testing.T) {
	// Verify cost constants are reasonable
	if CostPerGBStorage <= 0 {
		t.Errorf("CostPerGBStorage should be positive, got %f", CostPerGBStorage)
	}
	if CostPerVideoMinute <= 0 {
		t.Errorf("CostPerVideoMinute should be positive, got %f", CostPerVideoMinute)
	}
	if CostPerGBBandwidth <= 0 {
		t.Errorf("CostPerGBBandwidth should be positive, got %f", CostPerGBBandwidth)
	}
	if CostPerThousandTransforms <= 0 {
		t.Errorf("CostPerThousandTransforms should be positive, got %f", CostPerThousandTransforms)
	}
}

func TestEnhancedAnalyticsTypes(t *testing.T) {
	now := time.Now()
	enhanced := EnhancedAnalytics{
		ProcessingVolume: []ProcessingVolumePoint{
			{Date: now, JobType: "thumbnail", Count: 100, TotalDurationSeconds: 50},
		},
		StorageGrowth: []StorageGrowthPoint{
			{Date: now, CumulativeBytes: 1024 * 1024, BytesAdded: 512, FilesAdded: 2},
		},
		VideoStats: VideoProcessingStats{
			TotalVideoFiles: 10,
			TranscodeJobs:   5,
		},
		BandwidthStats: BandwidthStats{
			TotalDownloads:          100,
			EstimatedBandwidthBytes: 1024 * 1024 * 100,
			EstimatedBandwidthGB:    0.1,
		},
		CostForecast: CostForecast{
			TotalStorageGB:            1.0,
			TotalEstimatedMonthlyCost: 5.00,
		},
		ProcessingByTier: []ProcessingVolumeByTier{
			{Tier: "pro", JobType: "thumbnail", Count: 50, TotalDurationSeconds: 25},
		},
	}

	if len(enhanced.ProcessingVolume) != 1 {
		t.Errorf("ProcessingVolume length = %d, want 1", len(enhanced.ProcessingVolume))
	}
	if len(enhanced.StorageGrowth) != 1 {
		t.Errorf("StorageGrowth length = %d, want 1", len(enhanced.StorageGrowth))
	}
	if enhanced.VideoStats.TotalVideoFiles != 10 {
		t.Errorf("VideoStats.TotalVideoFiles = %d, want 10", enhanced.VideoStats.TotalVideoFiles)
	}
	if enhanced.BandwidthStats.TotalDownloads != 100 {
		t.Errorf("BandwidthStats.TotalDownloads = %d, want 100", enhanced.BandwidthStats.TotalDownloads)
	}
}

func TestUserAnalyticsTypes(t *testing.T) {
	analytics := UserAnalytics{
		FilesUsed:         50,
		FilesLimit:        100,
		TransformsUsed:    200,
		TransformsLimit:   1000,
		StorageUsedBytes:  1024 * 1024 * 500,
		StorageLimitBytes: 1024 * 1024 * 1024 * 5,
		PlanName:          "pro",
		PlanRenewsAt:      time.Now().AddDate(0, 1, 0),
		DaysUntilRenewal:  30,
	}

	if analytics.FilesUsed > analytics.FilesLimit {
		t.Errorf("FilesUsed (%d) should not exceed FilesLimit (%d)", analytics.FilesUsed, analytics.FilesLimit)
	}
	if analytics.TransformsUsed > analytics.TransformsLimit {
		t.Errorf("TransformsUsed (%d) should not exceed TransformsLimit (%d)", analytics.TransformsUsed, analytics.TransformsLimit)
	}
}

func TestAdminDashboardTypes(t *testing.T) {
	dashboard := AdminDashboard{
		MRR:              4500.00,
		MRRGrowth:        12.5,
		TotalUsers:       1250,
		NewUsersThisWeek: 35,
		TotalFiles:       125000,
		JobSuccessRate:   99.2,
	}

	if dashboard.MRR < 0 {
		t.Errorf("MRR should not be negative, got %f", dashboard.MRR)
	}
	if dashboard.JobSuccessRate < 0 || dashboard.JobSuccessRate > 100 {
		t.Errorf("JobSuccessRate should be 0-100, got %f", dashboard.JobSuccessRate)
	}
}

func TestStorageAnalyticsTypes(t *testing.T) {
	storage := StorageAnalytics{
		BreakdownByType: []StorageBreakdownByType{
			{FileType: "image", FileCount: 100, TotalBytes: 1024 * 1024 * 100},
			{FileType: "video", FileCount: 10, TotalBytes: 1024 * 1024 * 1024},
		},
		BreakdownByVariant: []StorageBreakdownByVariant{
			{VariantType: "thumbnail", VariantCount: 100, TotalBytes: 1024 * 1024},
		},
		LargestFiles: []LargestFile{
			{ID: "file-1", Filename: "big-video.mp4", SizeBytes: 500 * 1024 * 1024},
		},
		TotalBytes: 1024 * 1024 * 1124,
	}

	if len(storage.BreakdownByType) != 2 {
		t.Errorf("BreakdownByType length = %d, want 2", len(storage.BreakdownByType))
	}
	if len(storage.LargestFiles) != 1 {
		t.Errorf("LargestFiles length = %d, want 1", len(storage.LargestFiles))
	}
}

func TestChurnMetricsTypes(t *testing.T) {
	churn := ChurnMetrics{
		ChurnedThisMonth: 5,
		CurrentActive:    1000,
		MonthlyChurnRate: 0.5,
		RetentionRate:    99.5,
	}

	if churn.MonthlyChurnRate < 0 || churn.MonthlyChurnRate > 100 {
		t.Errorf("MonthlyChurnRate should be 0-100, got %f", churn.MonthlyChurnRate)
	}
	if churn.RetentionRate < 0 || churn.RetentionRate > 100 {
		t.Errorf("RetentionRate should be 0-100, got %f", churn.RetentionRate)
	}
}

func TestRevenueMetricsTypes(t *testing.T) {
	revenue := RevenueMetrics{
		MRR:         4500.00,
		ARR:         54000.00,
		ARPU:        45.00,
		PayingUsers: 100,
	}

	expectedARR := revenue.MRR * 12
	if revenue.ARR != expectedARR {
		t.Errorf("ARR = %f, want %f (MRR * 12)", revenue.ARR, expectedARR)
	}
}
