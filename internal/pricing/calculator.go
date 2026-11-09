package pricing

import (
	"context"
	"errors"
	"math/big"
	"time"
)

var (
	ErrRateNotFound = errors.New("pricing rate not found for model")
)

// ModelRate represents the versioned pricing matrix for an LLM model.
// Rates are defined as microdollars per 1,000,000 tokens ($1.00 USD = 1,000,000 microdollars).
type ModelRate struct {
	Version                    string
	Provider                   string
	Model                      string
	PromptRatePerMillion       int64
	CompletionRatePerMillion   int64
	CachedPromptRatePerMillion *int64
	EffectiveFrom              time.Time
}

// RateResolver retrieves the active pricing rate for a model at a specific point in time.
type RateResolver interface {
	ResolveRate(ctx context.Context, provider, model string, at time.Time) (*ModelRate, error)
}

// CalculationResult holds the calculated cost and the pricing version applied.
type CalculationResult struct {
	CostMicrodollars *int64
	PricingVersion   *string
	UsageStatus      string
}

// Calculator computes deterministic costs in integer microdollars.
type Calculator struct {
	resolver RateResolver
}

// NewCalculator constructs a pricing calculator with the given rate resolver.
func NewCalculator(resolver RateResolver) *Calculator {
	return &Calculator{resolver: resolver}
}

// CalculateCost computes deterministic estimated cost for a completed usage record.
//
// FINANCIAL INTEGRITY RULES:
//  Calculations use integer fixed-point microdollars to eliminate floating-point drift.
//  Input and output costs are computed independently with half-up integer rounding.
//  Cached input tokens are only discounted if the rate explicitly specifies a cached rate.
//  Cached tokens are subtracted from prompt tokens to prevent double-counting.
//  If pricing is unavailable for the model, cost is returned as NULL and status as "unpriced_model".
//  A default rate is NEVER silently invented.
//  If total tokens > 0, the result is clamped to at least 1 microdollar so fractional
//  usage does not round down to free.
func (c *Calculator) CalculateCost(ctx context.Context, provider, model string, inputTokens, outputTokens, cachedTokens *int64, requestTime time.Time) (*CalculationResult, error) {
	// If tokens are missing or nil, cost is unknown (NULL)
	if inputTokens == nil || outputTokens == nil {
		return &CalculationResult{
			CostMicrodollars: nil,
			PricingVersion:   nil,
			UsageStatus:      "missing_usage",
		}, nil
	}

	rate, err := c.resolver.ResolveRate(ctx, provider, model, requestTime)
	if err != nil {
		if errors.Is(err, ErrRateNotFound) {
			return &CalculationResult{
				CostMicrodollars: nil,
				PricingVersion:   nil,
				UsageStatus:      "unpriced_model",
			}, nil
		}
		return nil, err
	}

	input := *inputTokens
	output := *outputTokens

	var promptCost int64
	var cachedCost int64

	// Handle cached input pricing
	if cachedTokens != nil && *cachedTokens > 0 && rate.CachedPromptRatePerMillion != nil {
		cached := *cachedTokens
		if cached > input {
			cached = input // clamp to total prompt tokens to prevent negative uncached
		}
		uncached := input - cached

		promptCost = multiplyAndRound(uncached, rate.PromptRatePerMillion)
		cachedCost = multiplyAndRound(cached, *rate.CachedPromptRatePerMillion)
	} else {
		// No cached discount applicable; charge all prompt tokens at base prompt rate
		promptCost = multiplyAndRound(input, rate.PromptRatePerMillion)
	}

	completionCost := multiplyAndRound(output, rate.CompletionRatePerMillion)

	totalCost := promptCost + cachedCost + completionCost

	// If non-zero tokens were consumed, enforce minimum floor of 1 microdollar
	if (input+output) > 0 && totalCost == 0 {
		totalCost = 1
	}

	return &CalculationResult{
		CostMicrodollars: &totalCost,
		PricingVersion:   &rate.Version,
		UsageStatus:      "complete",
	}, nil
}

// multiplyAndRound computes (tokens * ratePerMillion + 500,000) / 1,000,000
// using big.Int to guarantee zero integer overflow on extreme token counts.
func multiplyAndRound(tokens, ratePerMillion int64) int64 {
	if tokens <= 0 || ratePerMillion <= 0 {
		return 0
	}

	t := big.NewInt(tokens)
	r := big.NewInt(ratePerMillion)
	prod := new(big.Int).Mul(t, r)

	// Half-up rounding offset: + 500,000
	prod.Add(prod, big.NewInt(500000))

	// Divide by 1,000,000
	quot := new(big.Int).Div(prod, big.NewInt(1000000))

	return quot.Int64()
}
