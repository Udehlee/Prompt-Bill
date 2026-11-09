package pricing

import (
	"context"
	"testing"
	"time"
)

type mockRateResolver struct {
	rates map[string][]*ModelRate
}

func (m *mockRateResolver) ResolveRate(ctx context.Context, provider, model string, at time.Time) (*ModelRate, error) {
	rates, exists := m.rates[provider+":"+model]
	if !exists || len(rates) == 0 {
		return nil, ErrRateNotFound
	}

	// Select the latest rate effective on or before 'at'
	var selected *ModelRate
	for _, r := range rates {
		if !r.EffectiveFrom.After(at) {
			if selected == nil || r.EffectiveFrom.After(selected.EffectiveFrom) {
				selected = r
			}
		}
	}

	if selected == nil {
		return nil, ErrRateNotFound
	}
	return selected, nil
}

func TestCalculator_CalculateCost(t *testing.T) {
	cachedRate := int64(75000) // $0.075 / 1M

	resolver := &mockRateResolver{
		rates: map[string][]*ModelRate{
			"openai:gpt-4o-mini": {
				{
					Version:                    "2024-07-18",
					Provider:                   "openai",
					Model:                      "gpt-4o-mini",
					PromptRatePerMillion:       150000, // $0.150 / 1M
					CompletionRatePerMillion:   600000, // $0.600 / 1M
					CachedPromptRatePerMillion: &cachedRate,
					EffectiveFrom:              time.Date(2024, 7, 18, 0, 0, 0, 0, time.UTC),
				},
			},
			"openai:gpt-4o-no-cache-discount": {
				{
					Version:                    "2024-05-13",
					Provider:                   "openai",
					Model:                      "gpt-4o-no-cache-discount",
					PromptRatePerMillion:       5000000,  // $5.00 / 1M
					CompletionRatePerMillion:   15000000, // $15.00 / 1M
					CachedPromptRatePerMillion: nil,      // No discount for cached
					EffectiveFrom:              time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	calc := NewCalculator(resolver)
	ctx := context.Background()
	reqTime := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)

	t.Run("deterministic calculation for standard usage", func(t *testing.T) {
		input := int64(1000)
		output := int64(500)

		res1, err := calc.CalculateCost(ctx, "openai", "gpt-4o-mini", &input, &output, nil, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res2, err := calc.CalculateCost(ctx, "openai", "gpt-4o-mini", &input, &output, nil, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 1000 * 150000 / 1000000 = 150
		// 500 * 600000 / 1000000 = 300
		// Total = 450 microdollars ($0.000450)
		expected := int64(450)
		if res1.CostMicrodollars == nil || *res1.CostMicrodollars != expected {
			t.Fatalf("expected cost %d, got %v", expected, res1.CostMicrodollars)
		}
		if *res1.CostMicrodollars != *res2.CostMicrodollars {
			t.Fatalf("non-deterministic results: %d vs %d", *res1.CostMicrodollars, *res2.CostMicrodollars)
		}
		if res1.PricingVersion == nil || *res1.PricingVersion != "2024-07-18" {
			t.Errorf("expected version 2024-07-18, got %v", res1.PricingVersion)
		}
	})

	t.Run("cached token discount applied without double counting", func(t *testing.T) {
		input := int64(1000)
		output := int64(500)
		cached := int64(400) // 400 cached, 600 uncached

		res, err := calc.CalculateCost(ctx, "openai", "gpt-4o-mini", &input, &output, &cached, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// uncached prompt: 600 * 150000 / 1000000 = 90 microdollars
		// cached prompt:   400 * 75000 / 1000000  = 30 microdollars
		// completion:      500 * 600000 / 1000000 = 300 microdollars
		// total:           90 + 30 + 300 = 420 microdollars ($0.000420)
		expected := int64(420)
		if res.CostMicrodollars == nil || *res.CostMicrodollars != expected {
			t.Fatalf("expected cost %d, got %v", expected, res.CostMicrodollars)
		}
	})

	t.Run("cached tokens reported but model has no cached discount", func(t *testing.T) {
		input := int64(1000)
		output := int64(500)
		cached := int64(400)

		res, err := calc.CalculateCost(ctx, "openai", "gpt-4o-no-cache-discount", &input, &output, &cached, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// All 1000 prompt tokens charged at base $5.00/1M = 5,000 microdollars
		// 500 completion charged at $15.00/1M = 7,500 microdollars
		// Total = 12,500 microdollars
		expected := int64(12500)
		if res.CostMicrodollars == nil || *res.CostMicrodollars != expected {
			t.Fatalf("expected cost %d, got %v", expected, res.CostMicrodollars)
		}
	})

	t.Run("unknown model returns unpriced status without default fallback", func(t *testing.T) {
		input := int64(1000)
		output := int64(500)

		res, err := calc.CalculateCost(ctx, "openai", "unknown-future-model", &input, &output, nil, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.CostMicrodollars != nil {
			t.Fatalf("expected CostMicrodollars to be nil for unknown model, got %d", *res.CostMicrodollars)
		}
		if res.PricingVersion != nil {
			t.Fatalf("expected PricingVersion to be nil, got %v", res.PricingVersion)
		}
		if res.UsageStatus != "unpriced_model" {
			t.Fatalf("expected UsageStatus 'unpriced_model', got %q", res.UsageStatus)
		}
	})

	t.Run("missing tokens returns missing_usage status", func(t *testing.T) {
		res, err := calc.CalculateCost(ctx, "openai", "gpt-4o-mini", nil, nil, nil, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.CostMicrodollars != nil {
			t.Fatalf("expected CostMicrodollars to be nil for missing usage, got %d", *res.CostMicrodollars)
		}
		if res.UsageStatus != "missing_usage" {
			t.Fatalf("expected UsageStatus 'missing_usage', got %q", res.UsageStatus)
		}
	})

	t.Run("fractional microdollar clamps to 1 microdollar minimum floor", func(t *testing.T) {
		// 1 token @ $0.150/1M = 0.15 microdollars
		input := int64(1)
		output := int64(0)

		res, err := calc.CalculateCost(ctx, "openai", "gpt-4o-mini", &input, &output, nil, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := int64(1)
		if res.CostMicrodollars == nil || *res.CostMicrodollars != expected {
			t.Fatalf("expected cost clamped to %d microdollar, got %v", expected, res.CostMicrodollars)
		}
	})

	t.Run("half-up rounding at arithmetic boundary", func(t *testing.T) {
		// 10 tokens @ $0.150 / 1M = 1.50 microdollars -> half-up rounds to 2
		input := int64(10)
		output := int64(0)

		res, err := calc.CalculateCost(ctx, "openai", "gpt-4o-mini", &input, &output, nil, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := int64(2)
		if res.CostMicrodollars == nil || *res.CostMicrodollars != expected {
			t.Fatalf("expected cost 2 microdollars, got %v", res.CostMicrodollars)
		}
	})

	t.Run("extreme volume arithmetic with zero overflow", func(t *testing.T) {
		// 100,000,000 tokens @ $15.00 / 1M (15,000,000 microdollars)
		// 100M * 15M / 1M = 1,500,000,000 microdollars ($1,500.00 USD)
		input := int64(100000000)
		output := int64(100000000)

		res, err := calc.CalculateCost(ctx, "openai", "gpt-4o-no-cache-discount", &input, &output, nil, reqTime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Prompt: 100M * 5M / 1M = 500,000,000
		// Completion: 100M * 15M / 1M = 1,500,000,000
		// Total: 2,000,000,000 microdollars ($2,000 USD)
		expected := int64(2000000000)
		if res.CostMicrodollars == nil || *res.CostMicrodollars != expected {
			t.Fatalf("expected cost %d, got %v", expected, res.CostMicrodollars)
		}
	})
}

func TestCalculator_HistoricalPricingVersions(t *testing.T) {
	resolver := &mockRateResolver{
		rates: map[string][]*ModelRate{
			"openai:model-v": {
				{
					Version:                  "2023-01-01",
					Provider:                 "openai",
					Model:                    "model-v",
					PromptRatePerMillion:     20000000, // $20.00 / 1M
					CompletionRatePerMillion: 40000000, // $40.00 / 1M
					EffectiveFrom:            time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					Version:                  "2024-01-01",
					Provider:                 "openai",
					Model:                    "model-v",
					PromptRatePerMillion:     10000000, // $10.00 / 1M (price cut)
					CompletionRatePerMillion: 20000000, // $20.00 / 1M
					EffectiveFrom:            time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	calc := NewCalculator(resolver)
	ctx := context.Background()

	input := int64(1000)
	output := int64(1000)

	// 1. Request dated in 2023 must receive the 2023 rate
	time2023 := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	res2023, err := calc.CalculateCost(ctx, "openai", "model-v", &input, &output, nil, time2023)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *res2023.PricingVersion != "2023-01-01" {
		t.Errorf("expected version 2023-01-01, got %s", *res2023.PricingVersion)
	}
	// 1000 * 20 + 1000 * 40 = 60,000 microdollars ($0.06)
	if *res2023.CostMicrodollars != 60000 {
		t.Errorf("expected 60000 microdollars, got %d", *res2023.CostMicrodollars)
	}

	// 2. Request dated in 2024 must receive the 2024 rate
	time2024 := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	res2024, err := calc.CalculateCost(ctx, "openai", "model-v", &input, &output, nil, time2024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *res2024.PricingVersion != "2024-01-01" {
		t.Errorf("expected version 2024-01-01, got %s", *res2024.PricingVersion)
	}
	// 1000 * 10 + 1000 * 20 = 30,000 microdollars ($0.03)
	if *res2024.CostMicrodollars != 30000 {
		t.Errorf("expected 30000 microdollars, got %d", *res2024.CostMicrodollars)
	}
}
