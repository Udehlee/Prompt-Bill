package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/pricing"
)

// ResolveRate retrieves the active rate for a provider and model effective on or before 'at'.
func (r *Repository) ResolveRate(ctx context.Context, provider, model string, at time.Time) (*pricing.ModelRate, error) {
	query := `
		SELECT version, provider, model, prompt_rate_per_million,
		       completion_rate_per_million, cached_prompt_rate_per_million, effective_from
		FROM pricing_rates
		WHERE provider = $1 AND model = $2 AND effective_from <= $3
		ORDER BY effective_from DESC
		LIMIT 1
	`

	var (
		version        string
		resProvider    string
		resModel       string
		promptRate     int64
		completionRate int64
		cachedRate     *int64
		effectiveFrom  time.Time
	)

	err := r.db.QueryRowContext(ctx, query, provider, model, at).Scan(
		&version,
		&resProvider,
		&resModel,
		&promptRate,
		&completionRate,
		&cachedRate,
		&effectiveFrom,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, pricing.ErrRateNotFound
		}
		return nil, fmt.Errorf("failed to resolve pricing rate: %w", err)
	}

	return &pricing.ModelRate{
		Version:                    version,
		Provider:                   resProvider,
		Model:                      resModel,
		PromptRatePerMillion:       promptRate,
		CompletionRatePerMillion:   completionRate,
		CachedPromptRatePerMillion: cachedRate,
		EffectiveFrom:              effectiveFrom,
	}, nil
}

// SeedDefaultRates inserts initial baseline model pricing rates if not already present.
func (r *Repository) SeedDefaultRates(ctx context.Context) error {
	rates := []struct {
		version        string
		provider       string
		model          string
		promptRate     int64
		completionRate int64
		cachedRate     *int64
		effectiveFrom  time.Time
	}{
		{
			version:        "2024-07-18",
			provider:       "openai",
			model:          "gpt-4o-mini",
			promptRate:     150000,          // $0.150 / 1M
			completionRate: 600000,          // $0.600 / 1M
			cachedRate:     int64Ptr(75000), // $0.075 / 1M
			effectiveFrom:  time.Date(2024, 7, 18, 0, 0, 0, 0, time.UTC),
		},
		{
			version:        "2024-08-06",
			provider:       "openai",
			model:          "gpt-4o",
			promptRate:     2500000,           // $2.50 / 1M
			completionRate: 10000000,          // $10.00 / 1M
			cachedRate:     int64Ptr(1250000), // $1.25 / 1M
			effectiveFrom:  time.Date(2024, 8, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			version:        "2024-05-14",
			provider:       "gemini",
			model:          "gemini-1.5-flash",
			promptRate:     75000,           // $0.075 / 1M
			completionRate: 300000,          // $0.300 / 1M
			cachedRate:     int64Ptr(18750), // $0.01875 / 1M
			effectiveFrom:  time.Date(2024, 5, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			version:        "2024-12-11",
			provider:       "gemini",
			model:          "gemini-2.0-flash",
			promptRate:     100000,          // $0.100 / 1M
			completionRate: 400000,          // $0.400 / 1M
			cachedRate:     int64Ptr(25000), // $0.025 / 1M
			effectiveFrom:  time.Date(2024, 12, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			version:        "2024-12-01",
			provider:       "groq",
			model:          "llama-3.3-70b-versatile",
			promptRate:     590000, // $0.590 / 1M
			completionRate: 790000, // $0.790 / 1M
			cachedRate:     nil,
			effectiveFrom:  time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			version:        "2026-01-01",
			provider:       "groq",
			model:          "openai/gpt-oss-20b",
			promptRate:     75000,           // $0.075 / 1M
			completionRate: 300000,          // $0.300 / 1M
			cachedRate:     int64Ptr(37500), // $0.0375 / 1M
			effectiveFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			version:        "2026-01-01",
			provider:       "groq",
			model:          "openai/gpt-oss-120b",
			promptRate:     150000,          // $0.150 / 1M
			completionRate: 600000,          // $0.600 / 1M
			cachedRate:     int64Ptr(75000), // $0.075 / 1M
			effectiveFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			version:        "2026-01-01",
			provider:       "groq",
			model:          "qwen/qwen3.8-27b",
			promptRate:     800000,           // $0.800 / 1M
			completionRate: 4000000,          // $4.000 / 1M
			cachedRate:     int64Ptr(400000), // $0.400 / 1M
			effectiveFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	query := `
		INSERT INTO pricing_rates (
			version, provider, model, prompt_rate_per_million,
			completion_rate_per_million, cached_prompt_rate_per_million, effective_from
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (version, provider, model) DO NOTHING
	`

	for _, rt := range rates {
		_, err := r.db.ExecContext(ctx, query,
			rt.version,
			rt.provider,
			rt.model,
			rt.promptRate,
			rt.completionRate,
			rt.cachedRate,
			rt.effectiveFrom,
		)
		if err != nil {
			return fmt.Errorf("failed to seed rate for %s: %w", rt.model, err)
		}
	}

	return nil
}

func int64Ptr(v int64) *int64 {
	return &v
}

