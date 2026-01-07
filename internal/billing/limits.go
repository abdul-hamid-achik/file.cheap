package billing

import (
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/db"
)

const (
	FreeFilesLimit    = 100
	FreeMaxFileSize   = 10 * 1024 * 1024 // 10 MB
	FreeRetentionDays = 7

	ProFilesLimit    = 2000
	ProMaxFileSize   = 100 * 1024 * 1024 // 100 MB
	ProRetentionDays = 365

	TrialDuration = 7 * 24 * time.Hour // 7 days
	GracePeriod   = 3 * 24 * time.Hour // 3 days for past_due
)

type TierLimits struct {
	FilesLimit        int
	MaxFileSize       int64
	MaxRetentionDays  int
	AllowedProcessing []string
	APIAccess         APIAccessLevel
	PriorityQueue     bool
	CustomWatermark   bool
}

type APIAccessLevel string

const (
	APIAccessNone     APIAccessLevel = "none"
	APIAccessReadOnly APIAccessLevel = "read_only"
	APIAccessFull     APIAccessLevel = "full"
)

func GetTierLimits(tier db.SubscriptionTier) TierLimits {
	switch tier {
	case db.SubscriptionTierPro, db.SubscriptionTierEnterprise:
		return TierLimits{
			FilesLimit:       ProFilesLimit,
			MaxFileSize:      ProMaxFileSize,
			MaxRetentionDays: ProRetentionDays,
			AllowedProcessing: []string{
				"thumbnail",
				"sm", "md", "lg", "xl",
				"og", "twitter", "instagram_square", "instagram_portrait", "instagram_story",
				"webp", "watermark",
			},
			APIAccess:       APIAccessFull,
			PriorityQueue:   true,
			CustomWatermark: true,
		}
	default:
		return TierLimits{
			FilesLimit:        FreeFilesLimit,
			MaxFileSize:       FreeMaxFileSize,
			MaxRetentionDays:  FreeRetentionDays,
			AllowedProcessing: []string{"thumbnail", "sm"},
			APIAccess:         APIAccessReadOnly,
			PriorityQueue:     false,
			CustomWatermark:   false,
		}
	}
}

type SubscriptionInfo struct {
	Tier        db.SubscriptionTier
	Status      db.SubscriptionStatus
	PeriodEnd   *time.Time
	TrialEndsAt *time.Time
	FilesLimit  int
	MaxFileSize int64
	FilesUsed   int64
}

func (s *SubscriptionInfo) IsActive() bool {
	switch s.Status {
	case db.SubscriptionStatusActive, db.SubscriptionStatusTrialing:
		return true
	case db.SubscriptionStatusPastDue:
		if s.PeriodEnd != nil {
			return time.Since(*s.PeriodEnd) < GracePeriod
		}
		return false
	default:
		return false
	}
}

func (s *SubscriptionInfo) IsPro() bool {
	if s.Tier != db.SubscriptionTierPro && s.Tier != db.SubscriptionTierEnterprise {
		return false
	}
	return s.IsActive()
}

func (s *SubscriptionInfo) CanUpload() bool {
	return s.FilesUsed < int64(s.FilesLimit)
}

func (s *SubscriptionInfo) CanUploadSize(size int64) bool {
	return size <= s.MaxFileSize
}

func (s *SubscriptionInfo) RemainingFiles() int64 {
	remaining := int64(s.FilesLimit) - s.FilesUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *SubscriptionInfo) UsagePercent() int {
	if s.FilesLimit == 0 {
		return 100
	}
	return int((s.FilesUsed * 100) / int64(s.FilesLimit))
}

func (s *SubscriptionInfo) TrialDaysRemaining() int {
	if s.Status != db.SubscriptionStatusTrialing || s.TrialEndsAt == nil {
		return 0
	}
	remaining := time.Until(*s.TrialEndsAt)
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Hours()/24) + 1
}

func CanUseFeature(tier db.SubscriptionTier, feature string) bool {
	limits := GetTierLimits(tier)
	for _, f := range limits.AllowedProcessing {
		if f == feature {
			return true
		}
	}
	return false
}

func CanUseAPI(tier db.SubscriptionTier, write bool) bool {
	limits := GetTierLimits(tier)
	switch limits.APIAccess {
	case APIAccessFull:
		return true
	case APIAccessReadOnly:
		return !write
	default:
		return false
	}
}
