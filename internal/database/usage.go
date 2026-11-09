package database

import (
	"context"
	"fmt"
)

// UsageRecord contains normalized metadata and metrics for an LLM proxy request.
type UsageRecord struct {
	TenantID         string
	APIKeyID         string
	UserID           *string
	FeatureName      *string
	Provider         string
	Endpoint         string
	RequestedModel   string
	ReportedModel    *string
	StatusCode       int
	LatencyMs        int64
	InputTokens      *int64
	OutputTokens     *int64
	CachedTokens     *int64
	TotalTokens      *int64
	UsageStatus      string
	CostMicrodollars *int64
	PricingVersion   *string
	IsStreaming      bool
	ErrorCategory    *string
}

// RecordUsage persists an accounting event into the usage_requests ledger.
func (r *Repository) RecordUsage(ctx context.Context, rec *UsageRecord) error {
	query := `
		INSERT INTO usage_requests (
			tenant_id,
			api_key_id,
			user_id,
			feature_name,
			provider,
			endpoint,
			requested_model,
			reported_model,
			status_code,
			latency_ms,
			input_tokens,
			output_tokens,
			cached_input_tokens,
			total_tokens,
			usage_status,
			estimated_cost_microdollars,
			pricing_version,
			is_streaming,
			error_category
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		rec.TenantID,
		rec.APIKeyID,
		rec.UserID,
		rec.FeatureName,
		rec.Provider,
		rec.Endpoint,
		rec.RequestedModel,
		rec.ReportedModel,
		rec.StatusCode,
		rec.LatencyMs,
		rec.InputTokens,
		rec.OutputTokens,
		rec.CachedTokens,
		rec.TotalTokens,
		rec.UsageStatus,
		rec.CostMicrodollars,
		rec.PricingVersion,
		rec.IsStreaming,
		rec.ErrorCategory,
	)
	if err != nil {
		return fmt.Errorf("failed to insert usage request: %w", err)
	}

	return nil
}
