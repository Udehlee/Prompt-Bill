-- +goose Up
CREATE TABLE pricing_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version VARCHAR(32) NOT NULL,
    provider VARCHAR(64) NOT NULL,
    model VARCHAR(128) NOT NULL,
    prompt_rate_per_million BIGINT NOT NULL,
    completion_rate_per_million BIGINT NOT NULL,
    cached_prompt_rate_per_million BIGINT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_pricing_version_provider_model UNIQUE (version, provider, model)
);

CREATE INDEX idx_pricing_lookup ON pricing_rates(provider, model, effective_from DESC);

-- +goose Down
DROP TABLE IF EXISTS pricing_rates;
