-- +goose Up
CREATE TABLE usage_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
    user_id VARCHAR(128) NULL,
    feature_name VARCHAR(128) NULL,
    provider VARCHAR(64) NOT NULL,
    endpoint VARCHAR(128) NOT NULL,
    requested_model VARCHAR(128) NOT NULL,
    reported_model VARCHAR(128) NULL,
    status_code INT NOT NULL,
    latency_ms BIGINT NOT NULL,
    input_tokens BIGINT NULL,
    output_tokens BIGINT NULL,
    cached_input_tokens BIGINT NULL,
    total_tokens BIGINT NULL,
    usage_status VARCHAR(32) NOT NULL,
    estimated_cost_microdollars BIGINT NULL,
    pricing_version VARCHAR(32) NULL,
    is_streaming BOOLEAN NOT NULL,
    error_category VARCHAR(64) NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

-- Index for tenant-scoped date range filtering (primary reporting query)
CREATE INDEX idx_usage_tenant_created 
    ON usage_requests(tenant_id, created_at DESC);

-- Partial index for top spender queries by feature
CREATE INDEX idx_usage_tenant_feature 
    ON usage_requests(tenant_id, feature_name, created_at DESC) 
    WHERE feature_name IS NOT NULL;

-- Partial index for top spender queries by user
CREATE INDEX idx_usage_tenant_user 
    ON usage_requests(tenant_id, user_id, created_at DESC) 
    WHERE user_id IS NOT NULL;

-- Cursor pagination index (stable sort across created_at and id)
CREATE INDEX idx_usage_cursor 
    ON usage_requests(tenant_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS usage_requests;
