package reporting

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/auth"
	"github.com/Udehlee/Prompt-Bill/internal/database"
)

type mockReportingRepo struct {
	summaryFn        func(ctx context.Context, tenantID string, from, to time.Time) (*database.SummaryResult, error)
	dailyCostsFn     func(ctx context.Context, tenantID string, from, to time.Time) ([]database.DailyCostItem, error)
	topSpendersFn    func(ctx context.Context, tenantID string, dimension string, from, to time.Time, limit int) ([]database.TopSpenderItem, error)
	requestsFn       func(ctx context.Context, tenantID string, from, to time.Time, limit int, cursorCreatedAt *time.Time, cursorID *string) ([]database.RequestItem, error)
	requestByIDFn    func(ctx context.Context, tenantID string, id string) (*database.RequestItem, error)
	modelBreakdownFn func(ctx context.Context, tenantID string, from, to time.Time) ([]database.ModelBreakdownItem, error)
}

func (m *mockReportingRepo) GetSummary(ctx context.Context, tenantID string, from, to time.Time) (*database.SummaryResult, error) {
	if m.summaryFn != nil {
		return m.summaryFn(ctx, tenantID, from, to)
	}
	return &database.SummaryResult{}, nil
}

func (m *mockReportingRepo) GetDailyCosts(ctx context.Context, tenantID string, from, to time.Time) ([]database.DailyCostItem, error) {
	if m.dailyCostsFn != nil {
		return m.dailyCostsFn(ctx, tenantID, from, to)
	}
	return nil, nil
}

func (m *mockReportingRepo) GetTopSpenders(ctx context.Context, tenantID string, dimension string, from, to time.Time, limit int) ([]database.TopSpenderItem, error) {
	if m.topSpendersFn != nil {
		return m.topSpendersFn(ctx, tenantID, dimension, from, to, limit)
	}
	return nil, nil
}

func (m *mockReportingRepo) GetRequests(ctx context.Context, tenantID string, from, to time.Time, limit int, cursorCreatedAt *time.Time, cursorID *string) ([]database.RequestItem, error) {
	if m.requestsFn != nil {
		return m.requestsFn(ctx, tenantID, from, to, limit, cursorCreatedAt, cursorID)
	}
	return nil, nil
}

func (m *mockReportingRepo) GetRequestByID(ctx context.Context, tenantID string, id string) (*database.RequestItem, error) {
	if m.requestByIDFn != nil {
		return m.requestByIDFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *mockReportingRepo) GetModelBreakdown(ctx context.Context, tenantID string, from, to time.Time) ([]database.ModelBreakdownItem, error) {
	if m.modelBreakdownFn != nil {
		return m.modelBreakdownFn(ctx, tenantID, from, to)
	}
	return nil, nil
}

func withTenantContext(req *http.Request, tenantID string) *http.Request {
	ctx := auth.ContextWithIdentity(req.Context(), auth.CallerIdentity{
		TenantID: tenantID,
		APIKeyID: "key-test",
	})
	return req.WithContext(ctx)
}

func TestHandler_Summary(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := &mockReportingRepo{
		summaryFn: func(ctx context.Context, tenantID string, from, to time.Time) (*database.SummaryResult, error) {
			if tenantID != "tenant-1" {
				t.Errorf("expected tenant-1, got %s", tenantID)
			}
			return &database.SummaryResult{
				TenantID:                  tenantID,
				From:                      from,
				To:                        to,
				TotalRequests:             100,
				SuccessfulRequests:        95,
				FailedRequests:            5,
				TotalInputTokens:          50000,
				TotalOutputTokens:         15000,
				TotalCachedTokens:         5000,
				TotalTokens:               65000,
				EstimatedCostMicrodollars: 32500,
				IncompleteUsageCount:      2,
				PricingStatus:             "complete",
			}, nil
		},
	}

	h := NewHandler(repo, logger)

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/summary", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("authenticated returns summary", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/summary?from=2026-10-01&to=2026-10-27", nil)
		req = withTenantContext(req, "tenant-1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body: %s", rec.Code, rec.Body.String())
		}

		var resp SummaryResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.TenantID != "tenant-1" {
			t.Errorf("expected tenant-1, got %s", resp.TenantID)
		}
		if resp.TotalRequests != 100 {
			t.Errorf("expected 100 requests, got %d", resp.TotalRequests)
		}
		if resp.EstimatedCostUSD != "0.032500" {
			t.Errorf("expected cost $0.032500, got %s", resp.EstimatedCostUSD)
		}
		if resp.PricingStatus != "complete" {
			t.Errorf("expected pricing_status complete, got %s", resp.PricingStatus)
		}
	})
}

func TestHandler_DailyCosts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := &mockReportingRepo{
		dailyCostsFn: func(ctx context.Context, tenantID string, from, to time.Time) ([]database.DailyCostItem, error) {
			return []database.DailyCostItem{
				{
					Date:                      "2026-10-25",
					Requests:                  40,
					InputTokens:               20000,
					OutputTokens:              5000,
					TotalTokens:               25000,
					EstimatedCostMicrodollars: 12000,
				},
				{
					Date:                      "2026-10-26",
					Requests:                  60,
					InputTokens:               30000,
					OutputTokens:              10000,
					TotalTokens:               40000,
					EstimatedCostMicrodollars: 20500,
				},
			}, nil
		},
	}

	h := NewHandler(repo, logger)

	req := httptest.NewRequest("GET", "/api/v1/daily-costs?from=2026-10-25&to=2026-10-26", nil)
	req = withTenantContext(req, "tenant-daily")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp DailyCostsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].Date != "2026-10-25" || resp.Items[0].EstimatedCostUSD != "0.012000" {
		t.Errorf("unexpected item 0: %+v", resp.Items[0])
	}
}

func TestHandler_TopSpenders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var capturedDimension string
	repo := &mockReportingRepo{
		topSpendersFn: func(ctx context.Context, tenantID string, dimension string, from, to time.Time, limit int) ([]database.TopSpenderItem, error) {
			capturedDimension = dimension
			return []database.TopSpenderItem{
				{
					Key:                       "checkout",
					Requests:                  50,
					TotalTokens:               30000,
					EstimatedCostMicrodollars: 15000,
				},
			}, nil
		},
	}

	h := NewHandler(repo, logger)

	t.Run("default dimension is feature", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/top-spenders", nil)
		req = withTenantContext(req, "tenant-spenders")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if capturedDimension != "feature" {
			t.Errorf("expected default dimension 'feature', got %s", capturedDimension)
		}
	})

	t.Run("dimension user", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/top-spenders?by=user", nil)
		req = withTenantContext(req, "tenant-spenders")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if capturedDimension != "user" {
			t.Errorf("expected dimension 'user', got %s", capturedDimension)
		}
	})

	t.Run("invalid dimension returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/top-spenders?by=invalid", nil)
		req = withTenantContext(req, "tenant-spenders")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestHandler_Requests_Pagination(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	now := time.Now().UTC()
	cost := int64(150)

	repo := &mockReportingRepo{
		requestsFn: func(ctx context.Context, tenantID string, from, to time.Time, limit int, cursorCreatedAt *time.Time, cursorID *string) ([]database.RequestItem, error) {
			// Simulate limit+1 rows returned from DB when more rows exist
			return []database.RequestItem{
				{
					ID:                        "req-1",
					CreatedAt:                 now,
					RequestedModel:            "gpt-4o-mini",
					StatusCode:                200,
					UsageStatus:               "complete",
					EstimatedCostMicrodollars: &cost,
				},
				{
					ID:                        "req-2",
					CreatedAt:                 now.Add(-1 * time.Minute),
					RequestedModel:            "gpt-4o-mini",
					StatusCode:                200,
					UsageStatus:               "complete",
					EstimatedCostMicrodollars: &cost,
				},
				{
					ID:                        "req-3-extra",
					CreatedAt:                 now.Add(-2 * time.Minute),
					RequestedModel:            "gpt-4o-mini",
					StatusCode:                200,
					UsageStatus:               "complete",
					EstimatedCostMicrodollars: &cost,
				},
			}, nil
		},
	}

	h := NewHandler(repo, logger)

	// Ask for limit=2; mock returns 3 items (limit+1) to signal next page exists
	req := httptest.NewRequest("GET", "/api/v1/requests?limit=2", nil)
	req = withTenantContext(req, "tenant-reqs")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp RequestsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items after trimming limit+1 row, got %d", len(resp.Items))
	}
	if resp.NextCursor == nil {
		t.Fatal("expected next_cursor to be set when extra row was returned")
	}

	// Verify next_cursor decodes to req-2
	decoded, err := DecodeCursor(*resp.NextCursor)
	if err != nil {
		t.Fatalf("failed to decode generated next_cursor: %v", err)
	}
	if decoded.ID != "req-2" {
		t.Errorf("expected next cursor ID 'req-2', got %s", decoded.ID)
	}
}

func TestHandler_RequestByID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cost := int64(88)
	repo := &mockReportingRepo{
		requestByIDFn: func(ctx context.Context, tenantID string, id string) (*database.RequestItem, error) {
			if id == "req-exists" && tenantID == "tenant-owner" {
				return &database.RequestItem{
					ID:                        "req-exists",
					TenantID:                  "tenant-owner",
					CreatedAt:                 time.Now().UTC(),
					RequestedModel:            "gpt-4o-mini",
					StatusCode:                200,
					UsageStatus:               "complete",
					EstimatedCostMicrodollars: &cost,
				}, nil
			}
			return nil, nil // Not found or belonged to another tenant
		},
	}

	h := NewHandler(repo, logger)

	t.Run("found returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/requests/req-exists", nil)
		req = withTenantContext(req, "tenant-owner")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp RequestItemResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.ID != "req-exists" {
			t.Errorf("expected req-exists, got %s", resp.ID)
		}
		if resp.EstimatedCostUSD == nil || *resp.EstimatedCostUSD != "0.000088" {
			t.Errorf("expected cost 0.000088, got %v", resp.EstimatedCostUSD)
		}
	})

	t.Run("different tenant returns 404 (isolation)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/requests/req-exists", nil)
		req = withTenantContext(req, "tenant-intruder")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for cross-tenant request, got %d", rec.Code)
		}
	})

	t.Run("nonexistent request returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/requests/req-missing", nil)
		req = withTenantContext(req, "tenant-owner")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestHandler_Models(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := &mockReportingRepo{
		modelBreakdownFn: func(ctx context.Context, tenantID string, from, to time.Time) ([]database.ModelBreakdownItem, error) {
			if tenantID != "tenant-1" {
				t.Errorf("expected tenant-1, got %s", tenantID)
			}
			return []database.ModelBreakdownItem{
				{
					Model:                     "gpt-4o",
					Requests:                  50,
					InputTokens:               10000,
					OutputTokens:              3000,
					TotalTokens:               13000,
					EstimatedCostMicrodollars: 45000,
				},
				{
					Model:                     "gpt-4o-mini",
					Requests:                  200,
					InputTokens:               50000,
					OutputTokens:              15000,
					TotalTokens:               65000,
					EstimatedCostMicrodollars: 9750,
				},
			}, nil
		},
	}

	h := NewHandler(repo, logger)

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/models", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("valid model breakdown request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/models?from=2026-10-01&to=2026-10-27", nil)
		req = withTenantContext(req, "tenant-1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp ModelsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.TenantID != "tenant-1" {
			t.Errorf("expected tenant-1, got %s", resp.TenantID)
		}
		if len(resp.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.Items))
		}
		if resp.Items[0].Model != "gpt-4o" || resp.Items[0].EstimatedCostUSD != "0.045000" {
			t.Errorf("unexpected item 0: %+v", resp.Items[0])
		}
		if resp.Items[1].Model != "gpt-4o-mini" || resp.Items[1].EstimatedCostUSD != "0.009750" {
			t.Errorf("unexpected item 1: %+v", resp.Items[1])
		}
	})
}
