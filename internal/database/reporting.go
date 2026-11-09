package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SummaryResult holds aggregated usage and cost metrics for a tenant over a date range.
type SummaryResult struct {
	TenantID                  string    `json:"tenant_id"`
	From                      time.Time `json:"from"`
	To                        time.Time `json:"to"`
	TotalRequests             int64     `json:"total_requests"`
	SuccessfulRequests        int64     `json:"successful_requests"`
	FailedRequests            int64     `json:"failed_requests"`
	TotalInputTokens          int64     `json:"total_input_tokens"`
	TotalOutputTokens         int64     `json:"total_output_tokens"`
	TotalCachedTokens         int64     `json:"total_cached_tokens"`
	TotalTokens               int64     `json:"total_tokens"`
	EstimatedCostMicrodollars int64     `json:"estimated_cost_microdollars"`
	IncompleteUsageCount      int64     `json:"incomplete_usage_count"`
	PricingStatus             string    `json:"pricing_status"` // "complete", "partial"
	AvgLatencyMs              int64     `json:"avg_latency_ms"`
	MinLatencyMs              int64     `json:"min_latency_ms"`
	MaxLatencyMs              int64     `json:"max_latency_ms"`
	P95LatencyMs              int64     `json:"p95_latency_ms"`
}

// DailyCostItem represents aggregated metrics for a single UTC day.
type DailyCostItem struct {
	Date                      string `json:"date"` // YYYY-MM-DD
	Requests                  int64  `json:"requests"`
	InputTokens               int64  `json:"input_tokens"`
	OutputTokens              int64  `json:"output_tokens"`
	TotalTokens               int64  `json:"total_tokens"`
	EstimatedCostMicrodollars int64  `json:"estimated_cost_microdollars"`
}

// TopSpenderItem represents aggregated metrics for a specific dimension key (e.g. feature or user).
type TopSpenderItem struct {
	Key                       string `json:"key"`
	Requests                  int64  `json:"requests"`
	TotalTokens               int64  `json:"total_tokens"`
	EstimatedCostMicrodollars int64  `json:"estimated_cost_microdollars"`
}

// RequestItem represents an individual tracked request in the ledger.
type RequestItem struct {
	ID                        string    `json:"id"`
	TenantID                  string    `json:"tenant_id,omitempty"`
	CreatedAt                 time.Time `json:"created_at"`
	UserID                    *string   `json:"user_id"`
	FeatureName               *string   `json:"feature_name"`
	Provider                  string    `json:"provider"`
	Endpoint                  string    `json:"endpoint"`
	RequestedModel            string    `json:"requested_model"`
	ReportedModel             *string   `json:"reported_model"`
	StatusCode                int       `json:"status_code"`
	LatencyMs                 int64     `json:"latency_ms"`
	InputTokens               *int64    `json:"input_tokens"`
	OutputTokens              *int64    `json:"output_tokens"`
	CachedInputTokens         *int64    `json:"cached_input_tokens"`
	TotalTokens               *int64    `json:"total_tokens"`
	UsageStatus               string    `json:"usage_status"`
	EstimatedCostMicrodollars *int64    `json:"estimated_cost_microdollars"`
	PricingVersion            *string   `json:"pricing_version"`
	IsStreaming               bool      `json:"is_streaming"`
	ErrorCategory             *string   `json:"error_category"`
}

// Ping verifies database connectivity for readiness probes.
func (r *Repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

// GetSummary returns aggregated usage and financial totals for the given tenant and date range.
func (r *Repository) GetSummary(ctx context.Context, tenantID string, from, to time.Time) (*SummaryResult, error) {
	query := `
		SELECT
			COUNT(*) AS total_requests,
			COUNT(*) FILTER (WHERE status_code >= 200 AND status_code < 300) AS successful_requests,
			COUNT(*) FILTER (WHERE status_code >= 400) AS failed_requests,
			COALESCE(SUM(input_tokens), 0) AS total_input_tokens,
			COALESCE(SUM(output_tokens), 0) AS total_output_tokens,
			COALESCE(SUM(cached_input_tokens), 0) AS total_cached_tokens,
			COALESCE(SUM(total_tokens), 0) AS total_tokens,
			COALESCE(SUM(estimated_cost_microdollars), 0) AS estimated_cost_microdollars,
			COUNT(*) FILTER (WHERE usage_status != 'complete' OR input_tokens IS NULL) AS incomplete_usage_count,
			COUNT(*) FILTER (WHERE usage_status = 'unpriced_model' OR (status_code >= 200 AND status_code < 300 AND estimated_cost_microdollars IS NULL)) AS unpriced_count,
			COALESCE(ROUND(AVG(latency_ms)), 0)::BIGINT AS avg_latency_ms,
			COALESCE(MIN(latency_ms), 0) AS min_latency_ms,
			COALESCE(MAX(latency_ms), 0) AS max_latency_ms,
			COALESCE(ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms)), 0)::BIGINT AS p95_latency_ms
		FROM usage_requests
		WHERE tenant_id = $1
		  AND created_at >= $2
		  AND created_at <= $3
	`
	var (
		res           SummaryResult
		unpricedCount int64
	)
	res.TenantID = tenantID
	res.From = from
	res.To = to

	err := r.db.QueryRowContext(ctx, query, tenantID, from, to).Scan(
		&res.TotalRequests,
		&res.SuccessfulRequests,
		&res.FailedRequests,
		&res.TotalInputTokens,
		&res.TotalOutputTokens,
		&res.TotalCachedTokens,
		&res.TotalTokens,
		&res.EstimatedCostMicrodollars,
		&res.IncompleteUsageCount,
		&unpricedCount,
		&res.AvgLatencyMs,
		&res.MinLatencyMs,
		&res.MaxLatencyMs,
		&res.P95LatencyMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query summary: %w", err)
	}

	if unpricedCount > 0 {
		res.PricingStatus = "partial"
	} else {
		res.PricingStatus = "complete"
	}

	return &res, nil
}

// GetDailyCosts returns daily aggregated metrics grouped by UTC calendar day.
func (r *Repository) GetDailyCosts(ctx context.Context, tenantID string, from, to time.Time) ([]DailyCostItem, error) {
	query := `
		SELECT
			TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day_date,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(total_tokens), 0) AS total_tokens,
			COALESCE(SUM(estimated_cost_microdollars), 0) AS estimated_cost_microdollars
		FROM usage_requests
		WHERE tenant_id = $1
		  AND created_at >= $2
		  AND created_at <= $3
		GROUP BY day_date
		ORDER BY day_date ASC
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily costs: %w", err)
	}
	defer rows.Close()

	items := make([]DailyCostItem, 0)
	for rows.Next() {
		var item DailyCostItem
		if err := rows.Scan(
			&item.Date,
			&item.Requests,
			&item.InputTokens,
			&item.OutputTokens,
			&item.TotalTokens,
			&item.EstimatedCostMicrodollars,
		); err != nil {
			return nil, fmt.Errorf("failed to scan daily cost row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("daily cost rows iteration error: %w", err)
	}
	return items, nil
}

// GetTopSpenders returns top spending features or users ranked by cost and tokens.
func (r *Repository) GetTopSpenders(ctx context.Context, tenantID string, dimension string, from, to time.Time, limit int) ([]TopSpenderItem, error) {
	var query string
	switch dimension {
	case "feature":
		query = `
			SELECT
				feature_name AS spender_key,
				COUNT(*) AS requests,
				COALESCE(SUM(total_tokens), 0) AS total_tokens,
				COALESCE(SUM(estimated_cost_microdollars), 0) AS estimated_cost_microdollars
			FROM usage_requests
			WHERE tenant_id = $1
			  AND feature_name IS NOT NULL
			  AND created_at >= $2
			  AND created_at <= $3
			GROUP BY feature_name
			ORDER BY estimated_cost_microdollars DESC, total_tokens DESC
			LIMIT $4
		`
	case "user":
		query = `
			SELECT
				user_id AS spender_key,
				COUNT(*) AS requests,
				COALESCE(SUM(total_tokens), 0) AS total_tokens,
				COALESCE(SUM(estimated_cost_microdollars), 0) AS estimated_cost_microdollars
			FROM usage_requests
			WHERE tenant_id = $1
			  AND user_id IS NOT NULL
			  AND created_at >= $2
			  AND created_at <= $3
			GROUP BY user_id
			ORDER BY estimated_cost_microdollars DESC, total_tokens DESC
			LIMIT $4
		`
	default:
		return nil, fmt.Errorf("unsupported dimension: %s", dimension)
	}

	rows, err := r.db.QueryContext(ctx, query, tenantID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top spenders: %w", err)
	}
	defer rows.Close()

	items := make([]TopSpenderItem, 0)
	for rows.Next() {
		var item TopSpenderItem
		if err := rows.Scan(
			&item.Key,
			&item.Requests,
			&item.TotalTokens,
			&item.EstimatedCostMicrodollars,
		); err != nil {
			return nil, fmt.Errorf("failed to scan top spender row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("top spenders rows iteration error: %w", err)
	}
	return items, nil
}

// GetRequests returns a page of requests ordered by created_at DESC, id DESC.
// It fetches limit + 1 rows to allow the caller to detect if more rows exist.
func (r *Repository) GetRequests(ctx context.Context, tenantID string, from, to time.Time, limit int, cursorCreatedAt *time.Time, cursorID *string) ([]RequestItem, error) {
	var (
		query string
		args  []any
	)

	fetchLimit := limit + 1

	if cursorCreatedAt != nil && cursorID != nil {
		query = `
			SELECT
				id, user_id, feature_name, provider, endpoint,
				requested_model, reported_model, status_code, latency_ms,
				input_tokens, output_tokens, cached_input_tokens, total_tokens,
				usage_status, estimated_cost_microdollars, pricing_version,
				is_streaming, error_category, created_at
			FROM usage_requests
			WHERE tenant_id = $1
			  AND created_at >= $2
			  AND created_at <= $3
			  AND (created_at, id) < ($4, $5)
			ORDER BY created_at DESC, id DESC
			LIMIT $6
		`
		args = []any{tenantID, from, to, *cursorCreatedAt, *cursorID, fetchLimit}
	} else {
		query = `
			SELECT
				id, user_id, feature_name, provider, endpoint,
				requested_model, reported_model, status_code, latency_ms,
				input_tokens, output_tokens, cached_input_tokens, total_tokens,
				usage_status, estimated_cost_microdollars, pricing_version,
				is_streaming, error_category, created_at
			FROM usage_requests
			WHERE tenant_id = $1
			  AND created_at >= $2
			  AND created_at <= $3
			ORDER BY created_at DESC, id DESC
			LIMIT $4
		`
		args = []any{tenantID, from, to, fetchLimit}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query requests: %w", err)
	}
	defer rows.Close()

	items := make([]RequestItem, 0, fetchLimit)
	for rows.Next() {
		var item RequestItem
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.FeatureName,
			&item.Provider,
			&item.Endpoint,
			&item.RequestedModel,
			&item.ReportedModel,
			&item.StatusCode,
			&item.LatencyMs,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CachedInputTokens,
			&item.TotalTokens,
			&item.UsageStatus,
			&item.EstimatedCostMicrodollars,
			&item.PricingVersion,
			&item.IsStreaming,
			&item.ErrorCategory,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan request row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("requests rows iteration error: %w", err)
	}
	return items, nil
}

// GetRequestByID returns a single request detail, strictly enforcing tenant isolation.
func (r *Repository) GetRequestByID(ctx context.Context, tenantID string, id string) (*RequestItem, error) {
	query := `
		SELECT
			id, tenant_id, user_id, feature_name, provider, endpoint,
			requested_model, reported_model, status_code, latency_ms,
			input_tokens, output_tokens, cached_input_tokens, total_tokens,
			usage_status, estimated_cost_microdollars, pricing_version,
			is_streaming, error_category, created_at
		FROM usage_requests
		WHERE tenant_id = $1 AND id = $2
	`
	var item RequestItem
	err := r.db.QueryRowContext(ctx, query, tenantID, id).Scan(
		&item.ID,
		&item.TenantID,
		&item.UserID,
		&item.FeatureName,
		&item.Provider,
		&item.Endpoint,
		&item.RequestedModel,
		&item.ReportedModel,
		&item.StatusCode,
		&item.LatencyMs,
		&item.InputTokens,
		&item.OutputTokens,
		&item.CachedInputTokens,
		&item.TotalTokens,
		&item.UsageStatus,
		&item.EstimatedCostMicrodollars,
		&item.PricingVersion,
		&item.IsStreaming,
		&item.ErrorCategory,
		&item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found or belonged to another tenant
		}
		return nil, fmt.Errorf("failed to query request by id: %w", err)
	}
	return &item, nil
}

// ModelBreakdownItem represents aggregated usage metrics for a specific model.
type ModelBreakdownItem struct {
	Model                     string `json:"model"`
	Requests                  int64  `json:"requests"`
	InputTokens               int64  `json:"input_tokens"`
	OutputTokens              int64  `json:"output_tokens"`
	TotalTokens               int64  `json:"total_tokens"`
	EstimatedCostMicrodollars int64  `json:"estimated_cost_microdollars"`
}

// GetModelBreakdown returns aggregated usage metrics grouped by model for a tenant and date range.
func (r *Repository) GetModelBreakdown(ctx context.Context, tenantID string, from, to time.Time) ([]ModelBreakdownItem, error) {
	query := `
		SELECT
			COALESCE(reported_model, requested_model) AS model_name,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(total_tokens), 0) AS total_tokens,
			COALESCE(SUM(estimated_cost_microdollars), 0) AS estimated_cost_microdollars
		FROM usage_requests
		WHERE tenant_id = $1
		  AND created_at >= $2
		  AND created_at <= $3
		GROUP BY model_name
		ORDER BY estimated_cost_microdollars DESC, requests DESC
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query model breakdown: %w", err)
	}
	defer rows.Close()

	items := make([]ModelBreakdownItem, 0)
	for rows.Next() {
		var item ModelBreakdownItem
		if err := rows.Scan(
			&item.Model,
			&item.Requests,
			&item.InputTokens,
			&item.OutputTokens,
			&item.TotalTokens,
			&item.EstimatedCostMicrodollars,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model breakdown row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("model breakdown rows iteration error: %w", err)
	}
	return items, nil
}
