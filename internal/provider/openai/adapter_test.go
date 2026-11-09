package openai

import (
	"errors"
	"testing"
)

func TestExtractUsage(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantStatus  string
		wantInput   *int64
		wantOutput  *int64
		wantCached  *int64
		wantTotal   *int64
		wantModel   string
		expectErrIs error
	}{
		{
			name: "valid standard usage",
			payload: `{
				"model": "gpt-4o-mini",
				"usage": {
					"prompt_tokens": 100,
					"completion_tokens": 50,
					"total_tokens": 150
				}
			}`,
			wantStatus:  "complete",
			wantInput:   int64Ptr(100),
			wantOutput:  int64Ptr(50),
			wantCached:  nil,
			wantTotal:   int64Ptr(150),
			wantModel:   "gpt-4o-mini",
			expectErrIs: nil,
		},
		{
			name: "valid usage with cached tokens",
			payload: `{
				"model": "gpt-4o-2024-08-06",
				"usage": {
					"prompt_tokens": 200,
					"completion_tokens": 80,
					"total_tokens": 280,
					"prompt_tokens_details": {
						"cached_tokens": 128
					}
				}
			}`,
			wantStatus:  "complete",
			wantInput:   int64Ptr(200),
			wantOutput:  int64Ptr(80),
			wantCached:  int64Ptr(128),
			wantTotal:   int64Ptr(280),
			wantModel:   "gpt-4o-2024-08-06",
			expectErrIs: nil,
		},
		{
			name: "missing usage object",
			payload: `{
				"model": "gpt-4o",
				"choices": [{"message": {"role": "assistant", "content": "hi"}}]
			}`,
			wantStatus:  "missing_usage",
			wantInput:   nil,
			wantOutput:  nil,
			wantCached:  nil,
			wantTotal:   nil,
			wantModel:   "gpt-4o",
			expectErrIs: nil,
		},
		{
			name:        "malformed JSON",
			payload:     `{not valid json`,
			wantStatus:  "unparseable_response",
			expectErrIs: ErrMalformedJSON,
		},
		{
			name: "negative prompt tokens",
			payload: `{
				"model": "gpt-4o",
				"usage": {
					"prompt_tokens": -10,
					"completion_tokens": 5,
					"total_tokens": 5
				}
			}`,
			wantStatus:  "invalid_tokens",
			expectErrIs: ErrNegativeTokenCount,
		},
		{
			name: "negative cached tokens",
			payload: `{
				"model": "gpt-4o",
				"usage": {
					"prompt_tokens": 100,
					"completion_tokens": 50,
					"total_tokens": 150,
					"prompt_tokens_details": {
						"cached_tokens": -5
					}
				}
			}`,
			wantStatus:  "invalid_tokens",
			expectErrIs: ErrNegativeTokenCount,
		},
		{
			name: "inconsistent cached tokens exceeding prompt tokens",
			payload: `{
				"model": "gpt-4o",
				"usage": {
					"prompt_tokens": 50,
					"completion_tokens": 20,
					"total_tokens": 70,
					"prompt_tokens_details": {
						"cached_tokens": 100
					}
				}
			}`,
			wantStatus:  "inconsistent_cached_tokens",
			wantInput:   int64Ptr(50),
			wantOutput:  int64Ptr(20),
			wantCached:  int64Ptr(100),
			wantTotal:   int64Ptr(70),
			wantModel:   "gpt-4o",
			expectErrIs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ExtractUsage([]byte(tt.payload))

			if tt.expectErrIs != nil {
				if !errors.Is(err, tt.expectErrIs) {
					t.Fatalf("expected error %v, got %v", tt.expectErrIs, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if res.UsageStatus != tt.wantStatus {
				t.Errorf("expected UsageStatus %q, got %q", tt.wantStatus, res.UsageStatus)
			}

			if tt.wantModel != "" {
				if res.ReportedModel == nil || *res.ReportedModel != tt.wantModel {
					t.Errorf("expected ReportedModel %q, got %v", tt.wantModel, res.ReportedModel)
				}
			}

			assertInt64PtrEqual(t, "InputTokens", tt.wantInput, res.InputTokens)
			assertInt64PtrEqual(t, "OutputTokens", tt.wantOutput, res.OutputTokens)
			assertInt64PtrEqual(t, "CachedTokens", tt.wantCached, res.CachedTokens)
			assertInt64PtrEqual(t, "TotalTokens", tt.wantTotal, res.TotalTokens)
		})
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func assertInt64PtrEqual(t *testing.T, field string, expected, actual *int64) {
	t.Helper()
	if expected == nil && actual == nil {
		return
	}
	if expected == nil || actual == nil {
		t.Errorf("field %s: expected %v, got %v", field, expected, actual)
		return
	}
	if *expected != *actual {
		t.Errorf("field %s: expected %d, got %d", field, *expected, *actual)
	}
}
