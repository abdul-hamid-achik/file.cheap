package billing

import (
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
)

const (
	FreeFilesLimit           = 100
	FreeMaxFileSize          = 10 * 1024 * 1024 // 10 MB
	FreeRetentionDays        = 7
	FreeTransformationsLimit = 100

	ProFilesLimit           = 2000
	ProMaxFileSize          = 100 * 1024 * 1024 // 100 MB
	ProRetentionDays        = 365
	ProTransformationsLimit = 10000

	EnterpriseTransformationsLimit = -1 // unlimited

	TrialDuration = 7 * 24 * time.Hour // 7 days
	GracePeriod   = 3 * 24 * time.Hour // 3 days for past_due
)

type TierLimits struct {
	FilesLimit           int
	MaxFileSize          int64
	MaxRetentionDays     int
	TransformationsLimit int
	AllowedProcessing    []string
	APIAccess            APIAccessLevel
	PriorityQueue        bool
	CustomWatermark      bool
}

type APIAccessLevel string

const (
	APIAccessNone     APIAccessLevel = "none"
	APIAccessReadOnly APIAccessLevel = "read_only"
	APIAccessFull     APIAccessLevel = "full"
)

func GetTierLimits(tier db.SubscriptionTier) TierLimits {
	switch tier {
	case db.SubscriptionTierEnterprise:
		return TierLimits{
			FilesLimit:           ProFilesLimit,
			MaxFileSize:          ProMaxFileSize,
			MaxRetentionDays:     ProRetentionDays,
			TransformationsLimit: EnterpriseTransformationsLimit,
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
	case db.SubscriptionTierPro:
		return TierLimits{
			FilesLimit:           ProFilesLimit,
			MaxFileSize:          ProMaxFileSize,
			MaxRetentionDays:     ProRetentionDays,
			TransformationsLimit: ProTransformationsLimit,
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
			FilesLimit:           FreeFilesLimit,
			MaxFileSize:          FreeMaxFileSize,
			MaxRetentionDays:     FreeRetentionDays,
			TransformationsLimit: FreeTransformationsLimit,
			AllowedProcessing:    []string{"thumbnail", "sm"},
			APIAccess:            APIAccessReadOnly,
			PriorityQueue:        false,
			CustomWatermark:      false,
		}
	}
}

type SubscriptionInfo struct {
	Tier                   db.SubscriptionTier
	Status                 db.SubscriptionStatus
	PeriodEnd              *time.Time
	TrialEndsAt            *time.Time
	FilesLimit             int
	MaxFileSize            int64
	FilesUsed              int64
	TransformationsLimit   int
	TransformationsUsed    int
	TransformationsResetAt *time.Time
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

func CanTransform(current, limit int) bool {
	if limit == -1 {
		return true
	}
	return current < limit
}

func (s *SubscriptionInfo) CanTransform() bool {
	return CanTransform(s.TransformationsUsed, s.TransformationsLimit)
}

func (s *SubscriptionInfo) RemainingTransformations() int {
	if s.TransformationsLimit == -1 {
		return -1
	}
	remaining := s.TransformationsLimit - s.TransformationsUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *SubscriptionInfo) TransformationsUsagePercent() int {
	if s.TransformationsLimit <= 0 {
		return 0
	}
	return int((int64(s.TransformationsUsed) * 100) / int64(s.TransformationsLimit))
}

func (s *SubscriptionInfo) DaysUntilTransformationReset() int {
	if s.TransformationsResetAt == nil {
		return 0
	}
	remaining := time.Until(*s.TransformationsResetAt)
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Hours()/24) + 1
}
