-- Admin alert configuration for threshold-based alerts
CREATE TABLE IF NOT EXISTS admin_alert_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_name TEXT NOT NULL UNIQUE,
    threshold_value FLOAT8 NOT NULL,
    enabled BOOLEAN DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert default alert thresholds (ignore if already exist)
INSERT INTO admin_alert_config (metric_name, threshold_value, enabled) VALUES
    ('api_latency_p95_ms', 100, true),
    ('failed_jobs_per_hour', 5, true),
    ('worker_queue_size', 500, true),
    ('churn_rate_percent', 5, true)
ON CONFLICT (metric_name) DO NOTHING;
