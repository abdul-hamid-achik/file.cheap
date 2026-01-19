-- Add permissions column to api_tokens
ALTER TABLE api_tokens ADD COLUMN permissions TEXT[] NOT NULL DEFAULT '{}';

-- Create GIN index for efficient permission lookups
CREATE INDEX idx_api_tokens_permissions ON api_tokens USING GIN (permissions);

-- Backfill existing tokens with full access
UPDATE api_tokens SET permissions = ARRAY[
    'files:read',
    'files:write',
    'files:delete',
    'transform',
    'shares:read',
    'shares:write',
    'webhooks:read',
    'webhooks:write'
];
