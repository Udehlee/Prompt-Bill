package openai

import (
	"encoding/json"
	"errors"
)

var (
	ErrMalformedJSON      = errors.New("malformed response json")
	ErrNegativeTokenCount = errors.New("negative token count in usage payload")
)

// ExtractedUsage holds validated token counts and status from an OpenAI Chat Completion response.
type ExtractedUsage struct {
	ReportedModel *string
	InputTokens   *int64
	OutputTokens  *int64
	CachedTokens  *int64
	TotalTokens   *int64
	UsageStatus   string // "complete", "missing_usage", "unparseable_response", "invalid_tokens"
}

// rawResponse matches the documented OpenAI Chat Completion response structure.
type rawResponse struct {
	Model string    `json:"model"`
	Usage *rawUsage `json:"usage"`
}

type rawUsage struct {
	PromptTokens        *int64 `json:"prompt_tokens"`
	CompletionTokens    *int64 `json:"completion_tokens"`
	TotalTokens         *int64 `json:"total_tokens"`
	PromptTokensDetails *struct {
		CachedTokens *int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// ExtractUsage parses and validates token counts from an OpenAI response body
//
// ANTI-HALLUCINATION and INTEGRITY RULES:
// Missing or null usage objects are explicitly recorded as "missing_usage" NOT zero.
// Token counts are never estimated by counting words or characters
// Negative token counts are flagged as "invalid_tokens" and rejected
func ExtractUsage(bodyBytes []byte) (*ExtractedUsage, error) {
	var resp rawResponse
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		return &ExtractedUsage{
			UsageStatus: "unparseable_response",
		}, ErrMalformedJSON
	}

	var reportedModel *string
	if resp.Model != "" {
		reportedModel = &resp.Model
	}

	if resp.Usage == nil {
		return &ExtractedUsage{
			ReportedModel: reportedModel,
			UsageStatus:   "missing_usage",
		}, nil
	}

	return validateUsage(resp.Model, resp.Usage)
}

// validateUsage validates and normalizes an OpenAI usage object.
// Shared across non-streaming JSON responses and streaming SSE usage chunks.
func validateUsage(model string, u *rawUsage) (*ExtractedUsage, error) {
	var reportedModel *string
	if model != "" {
		reportedModel = &model
	}

	if u == nil {
		return &ExtractedUsage{
			ReportedModel: reportedModel,
			UsageStatus:   "missing_usage",
		}, nil
	}

	// Validate non-negative numbers
	if (u.PromptTokens != nil && *u.PromptTokens < 0) ||
		(u.CompletionTokens != nil && *u.CompletionTokens < 0) ||
		(u.TotalTokens != nil && *u.TotalTokens < 0) {
		return &ExtractedUsage{
			ReportedModel: reportedModel,
			UsageStatus:   "invalid_tokens",
		}, ErrNegativeTokenCount
	}

	var cachedTokens *int64
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens != nil {
		if *u.PromptTokensDetails.CachedTokens < 0 {
			return &ExtractedUsage{
				ReportedModel: reportedModel,
				UsageStatus:   "invalid_tokens",
			}, ErrNegativeTokenCount
		}
		cachedTokens = u.PromptTokensDetails.CachedTokens
	}

	// Verify internal consistency
	//  cached tokens must not exceed prompt tokens
	if u.PromptTokens != nil && cachedTokens != nil && *cachedTokens > *u.PromptTokens {
		return &ExtractedUsage{
			ReportedModel: reportedModel,
			InputTokens:   u.PromptTokens,
			OutputTokens:  u.CompletionTokens,
			CachedTokens:  cachedTokens,
			TotalTokens:   u.TotalTokens,
			UsageStatus:   "inconsistent_cached_tokens",
		}, nil
	}

	return &ExtractedUsage{
		ReportedModel: reportedModel,
		InputTokens:   u.PromptTokens,
		OutputTokens:  u.CompletionTokens,
		CachedTokens:  cachedTokens,
		TotalTokens:   u.TotalTokens,
		UsageStatus:   "complete",
	}, nil
}
