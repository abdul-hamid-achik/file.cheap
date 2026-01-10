package billing

import (
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
)

func TestGetTierLimits(t *testing.T) {
	tests := []struct {
		name           string
		tier           db.SubscriptionTier
		wantFilesLimit int
		wantMaxSize    int64
		wantAPIAccess  APIAccessLevel
	}{
		{
			name:           "free tier",
			tier:           db.SubscriptionTierFree,
			wantFilesLimit: FreeFilesLimit,
			wantMaxSize:    FreeMaxFileSize,
			wantAPIAccess:  APIAccessReadOnly,
		},
		{
			name:           "pro tier",
			tier:           db.SubscriptionTierPro,
			wantFilesLimit: ProFilesLimit,
			wantMaxSize:    ProMaxFileSize,
			wantAPIAccess:  APIAccessFull,
		},
		{
			name:           "enterprise tier",
			tier:           db.SubscriptionTierEnterprise,
			wantFilesLimit: ProFilesLimit,
			wantMaxSize:    ProMaxFileSize,
			wantAPIAccess:  APIAccessFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := GetTierLimits(tt.tier)
			if limits.FilesLimit != tt.wantFilesLimit {
				t.Errorf("FilesLimit = %d, want %d", limits.FilesLimit, tt.wantFilesLimit)
			}
			if limits.MaxFileSize != tt.wantMaxSize {
				t.Errorf("MaxFileSize = %d, want %d", limits.MaxFileSize, tt.wantMaxSize)
			}
			if limits.APIAccess != tt.wantAPIAccess {
				t.Errorf("APIAccess = %s, want %s", limits.APIAccess, tt.wantAPIAccess)
			}
		})
	}
}

func TestCanUseFeature(t *testing.T) {
	tests := []struct {
		name    string
		tier    db.SubscriptionTier
		feature string
		want    bool
	}{
		{"free can use thumbnail", db.SubscriptionTierFree, "thumbnail", true},
		{"free can use sm", db.SubscriptionTierFree, "sm", true},
		{"free cannot use md", db.SubscriptionTierFree, "md", false},
		{"free cannot use lg", db.SubscriptionTierFree, "lg", false},
		{"free cannot use xl", db.SubscriptionTierFree, "xl", false},
		{"free cannot use og", db.SubscriptionTierFree, "og", false},
		{"free cannot use twitter", db.SubscriptionTierFree, "twitter", false},
		{"free cannot use instagram_square", db.SubscriptionTierFree, "instagram_square", false},
		{"free cannot use webp", db.SubscriptionTierFree, "webp", false},
		{"free cannot use watermark", db.SubscriptionTierFree, "watermark", false},
		{"pro can use thumbnail", db.SubscriptionTierPro, "thumbnail", true},
		{"pro can use sm", db.SubscriptionTierPro, "sm", true},
		{"pro can use md", db.SubscriptionTierPro, "md", true},
		{"pro can use lg", db.SubscriptionTierPro, "lg", true},
		{"pro can use xl", db.SubscriptionTierPro, "xl", true},
		{"pro can use og", db.SubscriptionTierPro, "og", true},
		{"pro can use twitter", db.SubscriptionTierPro, "twitter", true},
		{"pro can use instagram_square", db.SubscriptionTierPro, "instagram_square", true},
		{"pro can use instagram_portrait", db.SubscriptionTierPro, "instagram_portrait", true},
		{"pro can use instagram_story", db.SubscriptionTierPro, "instagram_story", true},
		{"pro can use webp", db.SubscriptionTierPro, "webp", true},
		{"pro can use watermark", db.SubscriptionTierPro, "watermark", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanUseFeature(tt.tier, tt.feature); got != tt.want {
				t.Errorf("CanUseFeature(%s, %s) = %v, want %v", tt.tier, tt.feature, got, tt.want)
			}
		})
	}
}

func TestCanUseAPI(t *testing.T) {
	tests := []struct {
		name  string
		tier  db.SubscriptionTier
		write bool
		want  bool
	}{
		{"free can read", db.SubscriptionTierFree, false, true},
		{"free cannot write", db.SubscriptionTierFree, true, false},
		{"pro can read", db.SubscriptionTierPro, false, true},
		{"pro can write", db.SubscriptionTierPro, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanUseAPI(tt.tier, tt.write); got != tt.want {
				t.Errorf("CanUseAPI(%s, %v) = %v, want %v", tt.tier, tt.write, got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoIsActive(t *testing.T) {
	now := time.Now()
	pastDueRecent := now.Add(-1 * time.Hour)
	pastDueOld := now.Add(-4 * 24 * time.Hour)

	tests := []struct {
		name string
		info SubscriptionInfo
		want bool
	}{
		{
			name: "active status",
			info: SubscriptionInfo{Status: db.SubscriptionStatusActive},
			want: true,
		},
		{
			name: "trialing status",
			info: SubscriptionInfo{Status: db.SubscriptionStatusTrialing},
			want: true,
		},
		{
			name: "past_due within grace period",
			info: SubscriptionInfo{
				Status:    db.SubscriptionStatusPastDue,
				PeriodEnd: &pastDueRecent,
			},
			want: true,
		},
		{
			name: "past_due beyond grace period",
			info: SubscriptionInfo{
				Status:    db.SubscriptionStatusPastDue,
				PeriodEnd: &pastDueOld,
			},
			want: false,
		},
		{
			name: "canceled status",
			info: SubscriptionInfo{Status: db.SubscriptionStatusCanceled},
			want: false,
		},
		{
			name: "none status",
			info: SubscriptionInfo{Status: db.SubscriptionStatusNone},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoIsPro(t *testing.T) {
	tests := []struct {
		name string
		info SubscriptionInfo
		want bool
	}{
		{
			name: "pro tier active",
			info: SubscriptionInfo{
				Tier:   db.SubscriptionTierPro,
				Status: db.SubscriptionStatusActive,
			},
			want: true,
		},
		{
			name: "free tier active",
			info: SubscriptionInfo{
				Tier:   db.SubscriptionTierFree,
				Status: db.SubscriptionStatusActive,
			},
			want: false,
		},
		{
			name: "pro tier canceled",
			info: SubscriptionInfo{
				Tier:   db.SubscriptionTierPro,
				Status: db.SubscriptionStatusCanceled,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.IsPro(); got != tt.want {
				t.Errorf("IsPro() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoCanUpload(t *testing.T) {
	tests := []struct {
		name string
		info SubscriptionInfo
		want bool
	}{
		{
			name: "under limit",
			info: SubscriptionInfo{FilesUsed: 50, FilesLimit: 100},
			want: true,
		},
		{
			name: "at limit",
			info: SubscriptionInfo{FilesUsed: 100, FilesLimit: 100},
			want: false,
		},
		{
			name: "over limit",
			info: SubscriptionInfo{FilesUsed: 150, FilesLimit: 100},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.CanUpload(); got != tt.want {
				t.Errorf("CanUpload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoCanUploadSize(t *testing.T) {
	info := SubscriptionInfo{MaxFileSize: 10 * 1024 * 1024}

	tests := []struct {
		name string
		size int64
		want bool
	}{
		{"under limit", 5 * 1024 * 1024, true},
		{"at limit", 10 * 1024 * 1024, true},
		{"over limit", 15 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := info.CanUploadSize(tt.size); got != tt.want {
				t.Errorf("CanUploadSize(%d) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoUsagePercent(t *testing.T) {
	tests := []struct {
		name string
		info SubscriptionInfo
		want int
	}{
		{
			name: "0 percent",
			info: SubscriptionInfo{FilesUsed: 0, FilesLimit: 100},
			want: 0,
		},
		{
			name: "50 percent",
			info: SubscriptionInfo{FilesUsed: 50, FilesLimit: 100},
			want: 50,
		},
		{
			name: "100 percent",
			info: SubscriptionInfo{FilesUsed: 100, FilesLimit: 100},
			want: 100,
		},
		{
			name: "over 100 percent",
			info: SubscriptionInfo{FilesUsed: 150, FilesLimit: 100},
			want: 150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.UsagePercent(); got != tt.want {
				t.Errorf("UsagePercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoTrialDaysRemaining(t *testing.T) {
	now := time.Now()
	future3Days := now.Add(3 * 24 * time.Hour)
	past := now.Add(-1 * 24 * time.Hour)

	tests := []struct {
		name    string
		info    SubscriptionInfo
		wantMin int
		wantMax int
	}{
		{
			name:    "not trialing",
			info:    SubscriptionInfo{Status: db.SubscriptionStatusActive},
			wantMin: 0,
			wantMax: 0,
		},
		{
			name: "trialing with 3 days left",
			info: SubscriptionInfo{
				Status:      db.SubscriptionStatusTrialing,
				TrialEndsAt: &future3Days,
			},
			wantMin: 3,
			wantMax: 4,
		},
		{
			name: "trial ended",
			info: SubscriptionInfo{
				Status:      db.SubscriptionStatusTrialing,
				TrialEndsAt: &past,
			},
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.TrialDaysRemaining()
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("TrialDaysRemaining() = %d, want between %d and %d", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSubscriptionInfoRemainingFiles(t *testing.T) {
	tests := []struct {
		name string
		info SubscriptionInfo
		want int64
	}{
		{
			name: "50 remaining",
			info: SubscriptionInfo{FilesUsed: 50, FilesLimit: 100},
			want: 50,
		},
		{
			name: "0 remaining",
			info: SubscriptionInfo{FilesUsed: 100, FilesLimit: 100},
			want: 0,
		},
		{
			name: "negative returns 0",
			info: SubscriptionInfo{FilesUsed: 150, FilesLimit: 100},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.RemainingFiles(); got != tt.want {
				t.Errorf("RemainingFiles() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCanTransform(t *testing.T) {
	tests := []struct {
		name    string
		current int
		limit   int
		want    bool
	}{
		{"under_limit", 50, 100, true},
		{"at_limit", 100, 100, false},
		{"over_limit", 150, 100, false},
		{"unlimited", 10000, -1, true},
		{"zero_current_unlimited", 0, -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransform(tt.current, tt.limit); got != tt.want {
				t.Errorf("CanTransform(%d, %d) = %v, want %v", tt.current, tt.limit, got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoCanTransform(t *testing.T) {
	tests := []struct {
		name string
		info SubscriptionInfo
		want bool
	}{
		{
			name: "under_limit",
			info: SubscriptionInfo{TransformationsUsed: 50, TransformationsLimit: 100},
			want: true,
		},
		{
			name: "at_limit",
			info: SubscriptionInfo{TransformationsUsed: 100, TransformationsLimit: 100},
			want: false,
		},
		{
			name: "unlimited",
			info: SubscriptionInfo{TransformationsUsed: 10000, TransformationsLimit: -1},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.CanTransform(); got != tt.want {
				t.Errorf("CanTransform() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoRemainingTransformations(t *testing.T) {
	tests := []struct {
		name string
		info SubscriptionInfo
		want int
	}{
		{
			name: "50_remaining",
			info: SubscriptionInfo{TransformationsUsed: 50, TransformationsLimit: 100},
			want: 50,
		},
		{
			name: "0_remaining",
			info: SubscriptionInfo{TransformationsUsed: 100, TransformationsLimit: 100},
			want: 0,
		},
		{
			name: "negative_returns_0",
			info: SubscriptionInfo{TransformationsUsed: 150, TransformationsLimit: 100},
			want: 0,
		},
		{
			name: "unlimited_returns_negative_1",
			info: SubscriptionInfo{TransformationsUsed: 1000, TransformationsLimit: -1},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.RemainingTransformations(); got != tt.want {
				t.Errorf("RemainingTransformations() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSubscriptionInfoTransformationsUsagePercent(t *testing.T) {
	tests := []struct {
		name string
		info SubscriptionInfo
		want int
	}{
		{
			name: "50_percent",
			info: SubscriptionInfo{TransformationsUsed: 50, TransformationsLimit: 100},
			want: 50,
		},
		{
			name: "100_percent",
			info: SubscriptionInfo{TransformationsUsed: 100, TransformationsLimit: 100},
			want: 100,
		},
		{
			name: "0_percent",
			info: SubscriptionInfo{TransformationsUsed: 0, TransformationsLimit: 100},
			want: 0,
		},
		{
			name: "unlimited_returns_0",
			info: SubscriptionInfo{TransformationsUsed: 1000, TransformationsLimit: -1},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.TransformationsUsagePercent(); got != tt.want {
				t.Errorf("TransformationsUsagePercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetTierLimitsTransformations(t *testing.T) {
	tests := []struct {
		name      string
		tier      db.SubscriptionTier
		wantLimit int
	}{
		{"free_tier", db.SubscriptionTierFree, FreeTransformationsLimit},
		{"pro_tier", db.SubscriptionTierPro, ProTransformationsLimit},
		{"enterprise_tier", db.SubscriptionTierEnterprise, EnterpriseTransformationsLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := GetTierLimits(tt.tier)
			if limits.TransformationsLimit != tt.wantLimit {
				t.Errorf("TransformationsLimit = %d, want %d", limits.TransformationsLimit, tt.wantLimit)
			}
		})
	}
}
