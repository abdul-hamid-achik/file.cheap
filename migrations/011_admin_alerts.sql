-- +goose Up
CREATE TABLE admin_alert_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_name TEXT NOT NULL UNIQUE,
    threshold_value FLOAT8 NOT NULL,
    enabled BOOLEAN DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO admin_alert_config (metric_name, threshold_value, enabled) VALUES
    ('api_latency_p95_ms', 100, true),
    ('failed_jobs_per_hour', 5, true),
    ('worker_queue_size', 500, true),
    ('churn_rate_percent', 5, true);

-- +goose Down
DROP TABLE IF EXISTS admin_alert_config;
