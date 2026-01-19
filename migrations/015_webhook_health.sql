-- +goose Up
-- Add webhook health tracking columns for circuit breaker pattern

ALTER TABLE webhooks ADD COLUMN consecutive_failures INTEGER DEFAULT 0;
ALTER TABLE webhooks ADD COLUMN last_failure_at TIMESTAMPTZ;
ALTER TABLE webhooks ADD COLUMN circuit_state VARCHAR(20) DEFAULT 'closed';

CREATE INDEX idx_webhooks_circuit_state ON webhooks(circuit_state) WHERE circuit_state = 'open';

-- +goose Down
DROP INDEX IF EXISTS idx_webhooks_circuit_state;
ALTER TABLE webhooks DROP COLUMN IF EXISTS circuit_state;
ALTER TABLE webhooks DROP COLUMN IF EXISTS last_failure_at;
ALTER TABLE webhooks DROP COLUMN IF EXISTS consecutive_failures;
