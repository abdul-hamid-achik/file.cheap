# Dashboard Documentation

This document describes the dashboard features for users and administrators.

## User Dashboard

The user dashboard (`/dashboard`) provides an overview of the user's account and file processing activity.

### Features

#### Usage Bar
A horizontal bar at the top of the dashboard showing current usage against plan limits:
- **Files**: Number of files uploaded vs. plan limit
- **Transforms**: Number of transforms used this month vs. limit
- **Storage**: Storage used vs. plan limit

Color coding:
- Green (< 75%): Normal usage
- Orange (75-90%): Approaching limit
- Red (> 90%): Near or at limit

#### Upgrade Banner
Shown to free tier users or when approaching limits:
- Default variant: Shown to free users to encourage upgrade
- Urgent variant: Shown when usage exceeds 75% of any limit
- Dismissible with localStorage persistence

#### Stats Grid
Four stat cards showing:
- Total Files
- Processed Files
- Pending Jobs
- Storage Used

#### Recent Files
Table of the 5 most recently uploaded files with:
- Thumbnail preview
- File name and type
- Size
- Status badge (completed, processing, pending, failed)
- Upload date
- View action

### Onboarding Checklist
Shown to new users who haven't completed key actions:
1. Upload first file
2. Create first transform
3. Generate API token
4. Set up webhook (optional)

Progress tracked in user's `onboarding_steps` JSONB field.

## User Analytics (`/dashboard/analytics`)

Detailed analytics for the user's account.

### Features

- **Usage Statistics Cards**: Files, transforms, storage with limits and progress bars
- **Usage Chart**: Line chart showing uploads and transforms over 7/30/90 days
- **Transform Breakdown**: Pie chart and list of transform types used
- **Top Files by Transforms**: Bar chart of most-processed files
- **Recent Activity**: Real-time feed of uploads, transforms, and batch operations

### Data Export

Export analytics data in CSV or JSON format:
- `GET /dashboard/analytics/export?format=csv`
- `GET /dashboard/analytics/export?format=json`

## Admin Dashboard (`/admin`)

Platform-wide metrics for administrators.

### Key Metrics (Row 1)
- **MRR**: Monthly Recurring Revenue with growth indicator
- **Users**: Total users with new signups this week
- **Total Files**: Platform-wide file count with total storage
- **Jobs (24h)**: Jobs processed with success rate

### SaaS Metrics (Row 2)
- **Churn Rate**: Monthly churn percentage
- **Retention**: User retention rate
- **ARPU**: Average Revenue Per User
- **Est. LTV**: Estimated Lifetime Value with NRR indicator

### Charts & Widgets

#### Revenue & Users Chart
Line chart showing MRR history over 30/90 days or year.

#### Users by Plan
Distribution of users across subscription tiers (free, pro, enterprise) with conversion rate.

#### System Health
Real-time monitoring with 10-second refresh:
- API Latency (p95)
- Worker Queue Size
- Failed Jobs (1h)
- Storage Used
- DB Connections
- Redis Memory

Health indicators: green dot for healthy, amber for issues.

#### Top Users by Usage
List of power users by transform count with plan and monthly rate.

#### Recent Signups
Last 10 signups with auto-refresh every 30 seconds.

#### Failed Jobs
Recent failed jobs with retry button for those with < 3 attempts.

### Cohort Analysis
Monthly signup cohorts with retention tracking:
- Cohort size
- Retention by month (M0, M1, M2, etc.)
- Retention percentage

## Notifications

In-app notification system for users.

### Notification Types
- `limit_warning`: Usage approaching limits
- `job_failed`: Processing job failure
- `job_completed`: Processing complete
- `billing`: Subscription and payment updates

### Database Schema
```sql
CREATE TABLE notifications (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    link TEXT,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### API Endpoints
- `GET /notifications` - List notifications
- `POST /notifications/:id/read` - Mark as read
- `POST /notifications/mark-all-read` - Mark all as read
- `DELETE /notifications/:id` - Delete notification

## Alert Configuration (Admin)

Configurable thresholds for system alerts.

### Available Metrics
- `api_latency_p95_ms`: API latency threshold (default: 100ms)
- `failed_jobs_per_hour`: Failed job threshold (default: 5)
- `worker_queue_size`: Queue size threshold (default: 500)
- `churn_rate_percent`: Churn rate threshold (default: 5%)

### Database Schema
```sql
CREATE TABLE admin_alert_config (
    id UUID PRIMARY KEY,
    metric_name TEXT UNIQUE NOT NULL,
    threshold_value FLOAT8 NOT NULL,
    enabled BOOLEAN DEFAULT true,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

## Component Reference

### Usage Bar (`components/usage_bar.templ`)
```go
type UsageData struct {
    FilesUsed       int
    FilesLimit      int
    TransformsUsed  int
    TransformsLimit int
    StorageUsedMB   int64
    StorageLimitMB  int64
}

templ UsageBar(data UsageData)
```

### Upgrade Banner (`components/upgrade_banner.templ`)
```go
type UpgradeBannerData struct {
    Variant     UpgradeBannerVariant // default, urgent, discount
    CurrentPlan string
    Message     string
}

templ UpgradeBanner(data UpgradeBannerData)
```

### Onboarding Checklist (`components/onboarding_checklist.templ`)
```go
type OnboardingData struct {
    UploadedFirstFile bool
    CreatedTransform  bool
    GeneratedAPIToken bool
    SetupWebhook      bool
    CompletedAt       bool
}

templ OnboardingChecklist(data OnboardingData)
```

### Notification Dropdown (`components/notification_dropdown.templ`)
```go
type NotificationDropdownData struct {
    Notifications []NotificationItem
    UnreadCount   int
}

templ NotificationDropdown(data NotificationDropdownData)
```

## Analytics Types

### ChurnMetrics
```go
type ChurnMetrics struct {
    ChurnedThisMonth int64
    Churned30Days    int64
    CurrentActive    int64
    MonthlyChurnRate float64
    RetentionRate    float64
}
```

### RevenueMetrics
```go
type RevenueMetrics struct {
    MRR          float64
    ARR          float64
    ARPU         float64
    EstimatedLTV float64
    PayingUsers  int64
}
```

### CohortData
```go
type CohortData struct {
    CohortMonth  time.Time
    CohortSize   int64
    MonthsSince  int
    Retained     int64
    RetentionPct float64
}
```
