package reporting

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/auth"
	"github.com/Udehlee/Prompt-Bill/internal/database"
)

// ReportingRepository defines the persistence interface required for reporting queries.
type ReportingRepository interface {
	GetSummary(ctx context.Context, tenantID string, from, to time.Time) (*database.SummaryResult, error)
	GetDailyCosts(ctx context.Context, tenantID string, from, to time.Time) ([]database.DailyCostItem, error)
	GetTopSpenders(ctx context.Context, tenantID string, dimension string, from, to time.Time, limit int) ([]database.TopSpenderItem, error)
	GetRequests(ctx context.Context, tenantID string, from, to time.Time, limit int, cursorCreatedAt *time.Time, cursorID *string) ([]database.RequestItem, error)
	GetRequestByID(ctx context.Context, tenantID string, id string) (*database.RequestItem, error)
	GetModelBreakdown(ctx context.Context, tenantID string, from, to time.Time) ([]database.ModelBreakdownItem, error)
}

type handler struct {
	repo   ReportingRepository
	logger *slog.Logger
}

// SummaryResponse represents the JSON output for GET /api/v1/summary.
type SummaryResponse struct {
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
	EstimatedCostUSD          string    `json:"estimated_cost_usd"`
	EstimatedCostMicrodollars int64     `json:"estimated_cost_microdollars"`
	PricingStatus             string    `json:"pricing_status"`
	IncompleteUsageCount      int64     `json:"incomplete_usage_count"`
	AvgLatencyMs              int64     `json:"avg_latency_ms"`
	MinLatencyMs              int64     `json:"min_latency_ms"`
	MaxLatencyMs              int64     `json:"max_latency_ms"`
	P95LatencyMs              int64     `json:"p95_latency_ms"`
}

// DailyCostItemResponse represents a single day's cost item in GET /api/v1/daily-costs.
type DailyCostItemResponse struct {
	Date                      string `json:"date"`
	Requests                  int64  `json:"requests"`
	InputTokens               int64  `json:"input_tokens"`
	OutputTokens              int64  `json:"output_tokens"`
	TotalTokens               int64  `json:"total_tokens"`
	EstimatedCostMicrodollars int64  `json:"estimated_cost_microdollars"`
	EstimatedCostUSD          string `json:"estimated_cost_usd"`
}

// DailyCostsResponse represents the JSON output for GET /api/v1/daily-costs.
type DailyCostsResponse struct {
	TenantID string                  `json:"tenant_id"`
	Items    []DailyCostItemResponse `json:"items"`
}

// TopSpenderItemResponse represents a ranked spender in GET /api/v1/top-spenders.
type TopSpenderItemResponse struct {
	Key                       string `json:"key"`
	Requests                  int64  `json:"requests"`
	TotalTokens               int64  `json:"total_tokens"`
	EstimatedCostMicrodollars int64  `json:"estimated_cost_microdollars"`
	EstimatedCostUSD          string `json:"estimated_cost_usd"`
}

// TopSpendersResponse represents the JSON output for GET /api/v1/top-spenders.
type TopSpendersResponse struct {
	TenantID  string                   `json:"tenant_id"`
	Dimension string                   `json:"dimension"`
	Items     []TopSpenderItemResponse `json:"items"`
}

// RequestItemResponse represents a single request detail in GET /api/v1/requests.
type RequestItemResponse struct {
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
	EstimatedCostUSD          *string   `json:"estimated_cost_usd"`
	PricingVersion            *string   `json:"pricing_version"`
	IsStreaming               bool      `json:"is_streaming"`
	ErrorCategory             *string   `json:"error_category"`
}

// RequestsResponse represents the paginated list output for GET /api/v1/requests.
type RequestsResponse struct {
	Items      []RequestItemResponse `json:"items"`
	NextCursor *string               `json:"next_cursor,omitempty"`
}

// ModelBreakdownItemResponse represents a model item in GET /api/v1/models.
type ModelBreakdownItemResponse struct {
	Model                     string `json:"model"`
	Requests                  int64  `json:"requests"`
	InputTokens               int64  `json:"input_tokens"`
	OutputTokens              int64  `json:"output_tokens"`
	TotalTokens               int64  `json:"total_tokens"`
	EstimatedCostMicrodollars int64  `json:"estimated_cost_microdollars"`
	EstimatedCostUSD          string `json:"estimated_cost_usd"`
}

// ModelsResponse represents the JSON output for GET /api/v1/models.
type ModelsResponse struct {
	TenantID string                       `json:"tenant_id"`
	Items    []ModelBreakdownItemResponse `json:"items"`
}

// NewHandler constructs and mounts the reporting HTTP router.
func NewHandler(repo ReportingRepository, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	h := &handler{repo: repo, logger: logger}

	mux.HandleFunc("GET /api/v1/summary", h.handleSummary)
	mux.HandleFunc("GET /api/v1/daily-costs", h.handleDailyCosts)
	mux.HandleFunc("GET /api/v1/top-spenders", h.handleTopSpenders)
	mux.HandleFunc("GET /api/v1/models", h.handleModels)
	mux.HandleFunc("GET /api/v1/requests", h.handleRequests)
	mux.HandleFunc("GET /api/v1/requests/{id}", h.handleRequestByID)

	return mux
}

// handleSummary processes GET /api/v1/summary.
func (h *handler) handleSummary(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing authenticated identity")
		return
	}

	from, to, err := parseDateRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	summary, err := h.repo.GetSummary(r.Context(), identity.TenantID, from, to)
	if err != nil {
		h.logger.Error("failed to query summary", "tenant_id", identity.TenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve summary")
		return
	}

	resp := SummaryResponse{
		TenantID:                  summary.TenantID,
		From:                      summary.From,
		To:                        summary.To,
		TotalRequests:             summary.TotalRequests,
		SuccessfulRequests:        summary.SuccessfulRequests,
		FailedRequests:            summary.FailedRequests,
		TotalInputTokens:          summary.TotalInputTokens,
		TotalOutputTokens:         summary.TotalOutputTokens,
		TotalCachedTokens:         summary.TotalCachedTokens,
		TotalTokens:               summary.TotalTokens,
		EstimatedCostUSD:          formatUSD(summary.EstimatedCostMicrodollars),
		EstimatedCostMicrodollars: summary.EstimatedCostMicrodollars,
		PricingStatus:             summary.PricingStatus,
		IncompleteUsageCount:      summary.IncompleteUsageCount,
		AvgLatencyMs:              summary.AvgLatencyMs,
		MinLatencyMs:              summary.MinLatencyMs,
		MaxLatencyMs:              summary.MaxLatencyMs,
		P95LatencyMs:              summary.P95LatencyMs,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDailyCosts processes GET /api/v1/daily-costs.
func (h *handler) handleDailyCosts(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing authenticated identity")
		return
	}

	from, to, err := parseDateRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	items, err := h.repo.GetDailyCosts(r.Context(), identity.TenantID, from, to)
	if err != nil {
		h.logger.Error("failed to query daily costs", "tenant_id", identity.TenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve daily costs")
		return
	}

	respItems := make([]DailyCostItemResponse, 0, len(items))
	for _, it := range items {
		respItems = append(respItems, DailyCostItemResponse{
			Date:                      it.Date,
			Requests:                  it.Requests,
			InputTokens:               it.InputTokens,
			OutputTokens:              it.OutputTokens,
			TotalTokens:               it.TotalTokens,
			EstimatedCostMicrodollars: it.EstimatedCostMicrodollars,
			EstimatedCostUSD:          formatUSD(it.EstimatedCostMicrodollars),
		})
	}

	writeJSON(w, http.StatusOK, DailyCostsResponse{
		TenantID: identity.TenantID,
		Items:    respItems,
	})
}

// handleTopSpenders processes GET /api/v1/top-spenders.
func (h *handler) handleTopSpenders(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing authenticated identity")
		return
	}

	from, to, err := parseDateRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	dimension := r.URL.Query().Get("by")
	if dimension == "" {
		dimension = "feature" // default dimension
	}
	if dimension != "feature" && dimension != "user" {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid dimension: 'by' must be 'feature' or 'user'")
		return
	}

	limit, err := parseLimit(r, 10, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	items, err := h.repo.GetTopSpenders(r.Context(), identity.TenantID, dimension, from, to, limit)
	if err != nil {
		h.logger.Error("failed to query top spenders", "tenant_id", identity.TenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve top spenders")
		return
	}

	respItems := make([]TopSpenderItemResponse, 0, len(items))
	for _, it := range items {
		respItems = append(respItems, TopSpenderItemResponse{
			Key:                       it.Key,
			Requests:                  it.Requests,
			TotalTokens:               it.TotalTokens,
			EstimatedCostMicrodollars: it.EstimatedCostMicrodollars,
			EstimatedCostUSD:          formatUSD(it.EstimatedCostMicrodollars),
		})
	}

	writeJSON(w, http.StatusOK, TopSpendersResponse{
		TenantID:  identity.TenantID,
		Dimension: dimension,
		Items:     respItems,
	})
}

// handleRequests processes GET /api/v1/requests with keyset cursor pagination.
func (h *handler) handleRequests(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing authenticated identity")
		return
	}

	from, to, err := parseDateRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	limit, err := parseLimit(r, 20, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	var (
		cursorCreatedAt *time.Time
		cursorID        *string
	)

	cursorParam := r.URL.Query().Get("cursor")
	if cursorParam != "" {
		c, err := DecodeCursor(cursorParam)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid pagination cursor")
			return
		}
		cursorCreatedAt = &c.CreatedAt
		cursorID = &c.ID
	}

	rawItems, err := h.repo.GetRequests(r.Context(), identity.TenantID, from, to, limit, cursorCreatedAt, cursorID)
	if err != nil {
		h.logger.Error("failed to query requests", "tenant_id", identity.TenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve requests")
		return
	}

	var nextCursor *string
	if len(rawItems) > limit {
		// More rows exist; the limit+1 row is excluded from items
		lastVisible := rawItems[limit-1]
		encoded := EncodeCursor(lastVisible.CreatedAt, lastVisible.ID)
		nextCursor = &encoded
		rawItems = rawItems[:limit]
	}

	items := make([]RequestItemResponse, 0, len(rawItems))
	for _, it := range rawItems {
		items = append(items, RequestItemResponse{
			ID:                        it.ID,
			CreatedAt:                 it.CreatedAt,
			UserID:                    it.UserID,
			FeatureName:               it.FeatureName,
			Provider:                  it.Provider,
			Endpoint:                  it.Endpoint,
			RequestedModel:            it.RequestedModel,
			ReportedModel:             it.ReportedModel,
			StatusCode:                it.StatusCode,
			LatencyMs:                 it.LatencyMs,
			InputTokens:               it.InputTokens,
			OutputTokens:              it.OutputTokens,
			CachedInputTokens:         it.CachedInputTokens,
			TotalTokens:               it.TotalTokens,
			UsageStatus:               it.UsageStatus,
			EstimatedCostMicrodollars: it.EstimatedCostMicrodollars,
			EstimatedCostUSD:          formatUSDPtr(it.EstimatedCostMicrodollars),
			PricingVersion:            it.PricingVersion,
			IsStreaming:               it.IsStreaming,
			ErrorCategory:             it.ErrorCategory,
		})
	}

	writeJSON(w, http.StatusOK, RequestsResponse{
		Items:      items,
		NextCursor: nextCursor,
	})
}

// handleRequestByID processes GET /api/v1/requests/{id}.
func (h *handler) handleRequestByID(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing authenticated identity")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "missing request id parameter")
		return
	}

	item, err := h.repo.GetRequestByID(r.Context(), identity.TenantID, id)
	if err != nil {
		h.logger.Error("failed to query request by id", "tenant_id", identity.TenantID, "request_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve request")
		return
	}

	if item == nil {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}

	writeJSON(w, http.StatusOK, RequestItemResponse{
		ID:                        item.ID,
		TenantID:                  item.TenantID,
		CreatedAt:                 item.CreatedAt,
		UserID:                    item.UserID,
		FeatureName:               item.FeatureName,
		Provider:                  item.Provider,
		Endpoint:                  item.Endpoint,
		RequestedModel:            item.RequestedModel,
		ReportedModel:             item.ReportedModel,
		StatusCode:                item.StatusCode,
		LatencyMs:                 item.LatencyMs,
		InputTokens:               item.InputTokens,
		OutputTokens:              item.OutputTokens,
		CachedInputTokens:         item.CachedInputTokens,
		TotalTokens:               item.TotalTokens,
		UsageStatus:               item.UsageStatus,
		EstimatedCostMicrodollars: item.EstimatedCostMicrodollars,
		EstimatedCostUSD:          formatUSDPtr(item.EstimatedCostMicrodollars),
		PricingVersion:            item.PricingVersion,
		IsStreaming:               item.IsStreaming,
		ErrorCategory:             item.ErrorCategory,
	})
}

// handleModels processes GET /api/v1/models.
func (h *handler) handleModels(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing authenticated identity")
		return
	}

	from, to, err := parseDateRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	items, err := h.repo.GetModelBreakdown(r.Context(), identity.TenantID, from, to)
	if err != nil {
		h.logger.Error("failed to query model breakdown", "tenant_id", identity.TenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve model breakdown")
		return
	}

	respItems := make([]ModelBreakdownItemResponse, 0, len(items))
	for _, it := range items {
		respItems = append(respItems, ModelBreakdownItemResponse{
			Model:                     it.Model,
			Requests:                  it.Requests,
			InputTokens:               it.InputTokens,
			OutputTokens:              it.OutputTokens,
			TotalTokens:               it.TotalTokens,
			EstimatedCostMicrodollars: it.EstimatedCostMicrodollars,
			EstimatedCostUSD:          formatUSD(it.EstimatedCostMicrodollars),
		})
	}

	writeJSON(w, http.StatusOK, ModelsResponse{
		TenantID: identity.TenantID,
		Items:    respItems,
	})
}
