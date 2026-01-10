package analytics

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"
)

type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatJSON ExportFormat = "json"
)

func ExportUserAnalyticsJSON(data *UserAnalytics) ([]byte, error) {
	export := ExportData{
		UserAnalytics: data,
		DailyUsage:    data.DailyUsage,
		TopFiles:      data.TopFiles,
		ExportedAt:    time.Now().UTC(),
	}
	return json.MarshalIndent(export, "", "  ")
}

func ExportUserAnalyticsCSV(data *UserAnalytics) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	if err := w.Write([]string{"Metric", "Value"}); err != nil {
		return nil, err
	}

	rows := [][]string{
		{"Files Used", fmt.Sprintf("%d", data.FilesUsed)},
		{"Files Limit", fmt.Sprintf("%d", data.FilesLimit)},
		{"Transforms Used", fmt.Sprintf("%d", data.TransformsUsed)},
		{"Transforms Limit", fmt.Sprintf("%d", data.TransformsLimit)},
		{"Storage Used (bytes)", fmt.Sprintf("%d", data.StorageUsedBytes)},
		{"Storage Limit (bytes)", fmt.Sprintf("%d", data.StorageLimitBytes)},
		{"Plan", data.PlanName},
		{"Days Until Renewal", fmt.Sprintf("%d", data.DaysUntilRenewal)},
	}

	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	if err := w.Write([]string{"", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Daily Usage", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Date", "Uploads", "Transforms"}); err != nil {
		return nil, err
	}

	for _, d := range data.DailyUsage {
		row := []string{
			d.Date.Format("2006-01-02"),
			fmt.Sprintf("%d", d.Uploads),
			fmt.Sprintf("%d", d.Transforms),
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	if err := w.Write([]string{"", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Transform Breakdown", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Type", "Count", "Percentage"}); err != nil {
		return nil, err
	}

	for _, t := range data.TransformBreakdown {
		row := []string{
			t.Type,
			fmt.Sprintf("%d", t.Count),
			fmt.Sprintf("%.1f%%", t.Percentage),
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	if err := w.Write([]string{"", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Top Files", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Filename", "Transforms"}); err != nil {
		return nil, err
	}

	for _, f := range data.TopFiles {
		row := []string{
			f.Filename,
			fmt.Sprintf("%d", f.Transforms),
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func ExportAdminDashboardJSON(data *AdminDashboard) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

func ExportAdminDashboardCSV(data *AdminDashboard) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	if err := w.Write([]string{"Admin Dashboard Export", time.Now().UTC().Format(time.RFC3339)}); err != nil {
		return nil, err
	}

	if err := w.Write([]string{"", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Key Metrics", ""}); err != nil {
		return nil, err
	}

	rows := [][]string{
		{"MRR", fmt.Sprintf("$%.2f", data.MRR)},
		{"MRR Growth", fmt.Sprintf("%.1f%%", data.MRRGrowth)},
		{"Total Users", fmt.Sprintf("%d", data.TotalUsers)},
		{"New Users This Week", fmt.Sprintf("%d", data.NewUsersThisWeek)},
		{"Total Files", fmt.Sprintf("%d", data.TotalFiles)},
		{"Jobs Last 24h", fmt.Sprintf("%d", data.JobsLast24h)},
		{"Job Success Rate", fmt.Sprintf("%.1f%%", data.JobSuccessRate)},
		{"", ""},
		{"Churn & Retention", ""},
		{"Monthly Churn Rate", fmt.Sprintf("%.2f%%", data.Churn.MonthlyChurnRate)},
		{"Retention Rate", fmt.Sprintf("%.2f%%", data.Churn.RetentionRate)},
		{"Churned This Month", fmt.Sprintf("%d", data.Churn.ChurnedThisMonth)},
		{"Current Active Users", fmt.Sprintf("%d", data.Churn.CurrentActive)},
		{"", ""},
		{"Revenue Metrics", ""},
		{"ARPU", fmt.Sprintf("$%.2f", data.Revenue.ARPU)},
		{"Estimated LTV", fmt.Sprintf("$%.2f", data.Revenue.EstimatedLTV)},
		{"ARR", fmt.Sprintf("$%.2f", data.Revenue.ARR)},
		{"NRR", fmt.Sprintf("%.1f%%", data.NRR.NRRPercent)},
	}

	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	if err := w.Write([]string{"", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Users by Plan", ""}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{"Plan", "Count", "Percentage"}); err != nil {
		return nil, err
	}

	for _, p := range data.UsersByPlan {
		row := []string{
			p.Plan,
			fmt.Sprintf("%d", p.Count),
			fmt.Sprintf("%.1f%%", p.Percentage),
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
