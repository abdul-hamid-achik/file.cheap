package analytics

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExportUserAnalyticsJSON(t *testing.T) {
	data := &UserAnalytics{
		FilesUsed:         50,
		FilesLimit:        100,
		TransformsUsed:    75,
		TransformsLimit:   200,
		StorageUsedBytes:  1024 * 1024 * 500,
		StorageLimitBytes: 1024 * 1024 * 1024,
		PlanName:          "pro",
		DaysUntilRenewal:  15,
		DailyUsage: []DailyUsage{
			{Date: time.Now().AddDate(0, 0, -1), Uploads: 5, Transforms: 10},
			{Date: time.Now(), Uploads: 3, Transforms: 8},
		},
		TransformBreakdown: []TransformStat{
			{Type: "thumbnail", Count: 50, Percentage: 66.7},
			{Type: "webp", Count: 25, Percentage: 33.3},
		},
		TopFiles: []FileUsage{
			{FileID: "file-1", Filename: "image.jpg", Transforms: 20},
		},
	}

	result, err := ExportUserAnalyticsJSON(data)
	if err != nil {
		t.Fatalf("ExportUserAnalyticsJSON failed: %v", err)
	}

	var export ExportData
	if err := json.Unmarshal(result, &export); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if export.UserAnalytics.FilesUsed != 50 {
		t.Errorf("FilesUsed = %d, want 50", export.UserAnalytics.FilesUsed)
	}
	if export.UserAnalytics.PlanName != "pro" {
		t.Errorf("PlanName = %q, want %q", export.UserAnalytics.PlanName, "pro")
	}
	if len(export.DailyUsage) != 2 {
		t.Errorf("DailyUsage len = %d, want 2", len(export.DailyUsage))
	}
	if export.ExportedAt.IsZero() {
		t.Error("ExportedAt should not be zero")
	}
}

func TestExportUserAnalyticsCSV(t *testing.T) {
	data := &UserAnalytics{
		FilesUsed:         50,
		FilesLimit:        100,
		TransformsUsed:    75,
		TransformsLimit:   200,
		StorageUsedBytes:  1024 * 1024 * 500,
		StorageLimitBytes: 1024 * 1024 * 1024,
		PlanName:          "pro",
		DaysUntilRenewal:  15,
		DailyUsage: []DailyUsage{
			{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Uploads: 5, Transforms: 10},
		},
		TransformBreakdown: []TransformStat{
			{Type: "thumbnail", Count: 50, Percentage: 66.7},
		},
		TopFiles: []FileUsage{
			{FileID: "file-1", Filename: "image.jpg", Transforms: 20},
		},
	}

	result, err := ExportUserAnalyticsCSV(data)
	if err != nil {
		t.Fatalf("ExportUserAnalyticsCSV failed: %v", err)
	}

	csv := string(result)

	tests := []struct {
		name     string
		contains string
	}{
		{"metric header", "Metric,Value"},
		{"files used", "Files Used,50"},
		{"files limit", "Files Limit,100"},
		{"plan name", "Plan,pro"},
		{"daily usage header", "Date,Uploads,Transforms"},
		{"daily usage data", "2024-01-01,5,10"},
		{"transform breakdown", "thumbnail,50,66.7%"},
		{"top files", "image.jpg,20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(csv, tt.contains) {
				t.Errorf("CSV should contain %q", tt.contains)
			}
		})
	}
}

func TestExportAdminDashboardJSON(t *testing.T) {
	data := &AdminDashboard{
		MRR:              1500.00,
		MRRGrowth:        15.5,
		TotalUsers:       100,
		NewUsersThisWeek: 10,
		TotalFiles:       5000,
		JobsLast24h:      250,
		JobSuccessRate:   98.5,
		Churn: ChurnMetrics{
			ChurnedThisMonth: 5,
			MonthlyChurnRate: 2.5,
			RetentionRate:    97.5,
			CurrentActive:    95,
		},
		Revenue: RevenueMetrics{
			MRR:          1500.00,
			ARPU:         15.00,
			EstimatedLTV: 600.00,
			PayingUsers:  100,
		},
		NRR: NRRMetrics{
			NRRPercent: 105.0,
		},
		UsersByPlan: []PlanStats{
			{Plan: "free", Count: 50, Percentage: 50.0},
			{Plan: "pro", Count: 40, Percentage: 40.0},
			{Plan: "enterprise", Count: 10, Percentage: 10.0},
		},
	}

	result, err := ExportAdminDashboardJSON(data)
	if err != nil {
		t.Fatalf("ExportAdminDashboardJSON failed: %v", err)
	}

	var export AdminDashboard
	if err := json.Unmarshal(result, &export); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if export.MRR != 1500.00 {
		t.Errorf("MRR = %f, want 1500.00", export.MRR)
	}
	if export.Churn.MonthlyChurnRate != 2.5 {
		t.Errorf("ChurnRate = %f, want 2.5", export.Churn.MonthlyChurnRate)
	}
	if export.Revenue.ARPU != 15.00 {
		t.Errorf("ARPU = %f, want 15.00", export.Revenue.ARPU)
	}
}

func TestExportAdminDashboardCSV(t *testing.T) {
	data := &AdminDashboard{
		MRR:              1500.00,
		MRRGrowth:        15.5,
		TotalUsers:       100,
		NewUsersThisWeek: 10,
		TotalFiles:       5000,
		JobsLast24h:      250,
		JobSuccessRate:   98.5,
		Churn: ChurnMetrics{
			ChurnedThisMonth: 5,
			MonthlyChurnRate: 2.5,
			RetentionRate:    97.5,
		},
		Revenue: RevenueMetrics{
			MRR:          1500.00,
			ARPU:         15.00,
			EstimatedLTV: 600.00,
			ARR:          18000.00,
		},
		NRR: NRRMetrics{
			NRRPercent: 105.0,
		},
		UsersByPlan: []PlanStats{
			{Plan: "free", Count: 50, Percentage: 50.0},
			{Plan: "pro", Count: 40, Percentage: 40.0},
		},
	}

	result, err := ExportAdminDashboardCSV(data)
	if err != nil {
		t.Fatalf("ExportAdminDashboardCSV failed: %v", err)
	}

	csv := string(result)

	tests := []struct {
		name     string
		contains string
	}{
		{"mrr", "MRR,$1500.00"},
		{"growth", "MRR Growth,15.5%"},
		{"total users", "Total Users,100"},
		{"churn rate", "Monthly Churn Rate,2.50%"},
		{"retention", "Retention Rate,97.50%"},
		{"arpu", "ARPU,$15.00"},
		{"ltv", "Estimated LTV,$600.00"},
		{"nrr", "NRR,105.0%"},
		{"free plan", "free,50,50.0%"},
		{"pro plan", "pro,40,40.0%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(csv, tt.contains) {
				t.Errorf("CSV should contain %q, got:\n%s", tt.contains, csv)
			}
		})
	}
}
