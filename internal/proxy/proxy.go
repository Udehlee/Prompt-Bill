package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/auth"
	"github.com/Udehlee/Prompt-Bill/internal/config"
	"github.com/Udehlee/Prompt-Bill/internal/database"
	"github.com/Udehlee/Prompt-Bill/internal/pricing"
	"github.com/Udehlee/Prompt-Bill/internal/provider/openai"
)

const (
	maxRequestBodyBytes  = 10 << 20 
	maxResponseBodyBytes = 10 << 20
	chatCompletionsPath  = "/v1/chat/completions"
)

type UsageRecorder interface {
	RecordUsage(ctx context.Context, rec *database.UsageRecord) error
}

type CostCalculator interface {
	CalculateCost(ctx context.Context, provider, model string, inputTokens, outputTokens, cachedTokens *int64, requestTime time.Time) (*pricing.CalculationResult, error)
}

type contextKey struct{}
var stateKey = contextKey{}

// requestState tracks metadata, timing, and token metrics across the proxy lifecycle.
type requestState struct {
	mu               sync.Mutex
	startTime        time.Time
	latency          time.Duration
	statusCode       int
	provider         string
	requestedModel   string
	reportedModel    string
	isStreaming      bool
	identity         auth.CallerIdentity
	bodyBytes        []byte
	inputTokens      *int64
	outputTokens     *int64
	cachedTokens     *int64
	totalTokens      *int64
	costMicrodollars *int64
	pricingVersion   *string
	usageStatus      string
	errorCategory    string
}

type proxyOptions struct {
	transport  http.RoundTripper
	calculator CostCalculator
}

type Option func(*proxyOptions)

// WithTransport configures a custom http.RoundTripper for the reverse proxy.
// Primarily used for fast, socketless mock testing in isolated sandbox environments.
func WithTransport(transport http.RoundTripper) Option {
	return func(o *proxyOptions) {
		o.transport = transport
	}
}

// WithCalculator configures the pricing calculator for estimated cost computation.
func WithCalculator(calc CostCalculator) Option {
	return func(o *proxyOptions) {
		o.calculator = calc
	}
}

// Handler constructs the reverse proxy HTTP handler for OpenAI Chat Completions.
func Handler(cfg *config.Config, recorder UsageRecorder, logger *slog.Logger, opts ...Option) (http.Handler, error) {
	targetURL, err := url.Parse(cfg.UpstreamBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream base URL: %w", err)
	}

	var fallbackURL *url.URL
	if cfg.FallbackBaseURL != "" {
		fallbackURL, err = url.Parse(cfg.FallbackBaseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid fallback base URL: %w", err)
		}
	}

	optConfig := &proxyOptions{}
	for _, opt := range opts {
		opt(optConfig)
	}

	baseTransport := optConfig.transport
	if baseTransport == nil {
		// Production-grade transport with explicit connection pooling and response header timeouts
		// to mitigate hung or slowloris-style upstream responses while supporting long SSE streams.
		baseTransport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}

	transport := &failoverRoundTripper{
		primary:        baseTransport,
		fallbackURL:    fallbackURL,
		fallbackAPIKey: cfg.FallbackAPIKey,
		fallbackModel:  cfg.FallbackModel,
		fallbackProv:   cfg.FallbackProvider,
	}

	proxy := &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: -1, // Flush immediately after each write to client for responsive progressive streaming

		Rewrite: func(pr *httputil.ProxyRequest) {
			// Set destination to validated upstream URL and intelligently join paths
			pr.SetURL(targetURL)
			pr.Out.URL.Path = joinProxyPath(targetURL.Path, pr.In.URL.Path)
			pr.Out.URL.RawPath = ""

			// Server-side credential injection: strip client auth, inject upstream secret
			pr.Out.Header.Del("Authorization")
			if cfg.UpstreamAPIKey != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+cfg.UpstreamAPIKey)
			}

			// Strip caller cookies and all internal tracker headers to avoid leaking caller metadata upstream
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("X-Tracker-Key")
			pr.Out.Header.Del("X-Tracker-User-Id")
			pr.Out.Header.Del("X-Tracker-Feature")
			pr.Out.Header.Del("X-Tracker-Tenant-Id")

			// Ensure Host header matches upstream target
			pr.Out.Host = targetURL.Host
			logger.Info("proxy forwarding", "url", pr.Out.URL.String(), "host", pr.Out.Host)
		},

		ModifyResponse: func(resp *http.Response) error {
			state, ok := resp.Request.Context().Value(stateKey).(*requestState)
			if !ok {
				return nil
			}

			state.mu.Lock()
			state.latency = time.Since(state.startTime)
			state.statusCode = resp.StatusCode
			isStreaming := state.isStreaming
			state.mu.Unlock()

			isSSE := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")

			if resp.StatusCode == http.StatusOK {
				if isStreaming || isSSE {
					state.mu.Lock()
					state.isStreaming = true
					state.mu.Unlock()

					parser := openai.NewStreamParser()
					resp.Body = newStreamObserver(resp.Body, parser, state, optConfig.calculator, resp.Request.Context())
					return nil
				}

				// Non-streaming JSON response observation
				bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
				if err != nil {
					return fmt.Errorf("failed to read response body: %w", err)
				}
				_ = resp.Body.Close()
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

				extracted, extractErr := openai.ExtractUsage(bodyBytes)
				state.mu.Lock()
				defer state.mu.Unlock()

				if extractErr != nil {
					state.usageStatus = extracted.UsageStatus
				} else {
					if extracted.ReportedModel != nil {
						state.reportedModel = *extracted.ReportedModel
					}
					state.inputTokens = extracted.InputTokens
					state.outputTokens = extracted.OutputTokens
					state.cachedTokens = extracted.CachedTokens
					state.totalTokens = extracted.TotalTokens
					state.usageStatus = extracted.UsageStatus

					// Calculate cost if pricing calculator is configured and usage is complete
					if optConfig.calculator != nil && state.usageStatus == "complete" {
						costRes, err := optConfig.calculator.CalculateCost(
							resp.Request.Context(),
							state.provider,
							state.requestedModel,
							state.inputTokens,
							state.outputTokens,
							state.cachedTokens,
							state.startTime,
						)
						if err == nil && costRes != nil {
							state.costMicrodollars = costRes.CostMicrodollars
							state.pricingVersion = costRes.PricingVersion
							if costRes.UsageStatus != "" && costRes.UsageStatus != "complete" {
								state.usageStatus = costRes.UsageStatus
							}
						}
					}
				}
			} else if resp.StatusCode >= 400 {
				state.mu.Lock()
				state.usageStatus = "upstream_error"
				if resp.StatusCode >= 500 {
					state.errorCategory = "upstream_5xx"
				} else {
					state.errorCategory = "upstream_4xx"
				}
				state.mu.Unlock()
			}

			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, proxyErr error) {
			state, ok := r.Context().Value(stateKey).(*requestState)
			if !ok {
				http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
				return
			}

			var clientMsg string
			var statusCode int
			var errCategory string

			switch {
			case errors.Is(proxyErr, context.Canceled) || r.Context().Err() == context.Canceled:
				// Client closed connection prematurely
				statusCode = 499
				errCategory = "client_disconnect"
				clientMsg = "client closed connection"
			case errors.Is(proxyErr, context.DeadlineExceeded) || isNetTimeout(proxyErr):
				statusCode = http.StatusGatewayTimeout
				errCategory = "upstream_timeout"
				clientMsg = "upstream provider timed out"
			default:
				statusCode = http.StatusBadGateway
				errCategory = "upstream_connection_error"
				clientMsg = "failed to connect to upstream provider"
			}

			state.mu.Lock()
			state.latency = time.Since(state.startTime)
			state.usageStatus = "upstream_error"
			state.statusCode = statusCode
			state.errorCategory = errCategory
			state.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "gateway_error",
				"message": clientMsg,
			})
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != chatCompletionsPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized","message":"missing authenticated identity"}`))
			return
		}

		// Read and bound inbound request body
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_, _ = w.Write([]byte(`{"error":"payload_too_large","message":"request body exceeds 10MB limit"}`))
				return
			}
			http.Error(w, `{"error":"bad_request","message":"failed to read request body"}`, http.StatusBadRequest)
			return
		}

		// Inspect request metadata (model and streaming flag) without logging prompt contents
		var reqMeta struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.Unmarshal(bodyBytes, &reqMeta); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad_request","message":"malformed JSON request body"}`))
			return
		}

		if reqMeta.Model == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad_request","message":"missing required field: model"}`))
			return
		}

		// Restore request body for reverse proxy forwarding
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))

		state := &requestState{
			startTime:      time.Now(),
			provider:       cfg.UpstreamProvider,
			requestedModel: reqMeta.Model,
			isStreaming:    reqMeta.Stream,
			identity:       identity,
			bodyBytes:      bodyBytes,
		}

		ctxWithState := context.WithValue(r.Context(), stateKey, state)
		proxy.ServeHTTP(w, r.WithContext(ctxWithState))

		// Persist usage record after proxy completion using detached context
		if recorder != nil {
			persistUsage(state, recorder, logger)
		}
	}), nil
}

func persistUsage(state *requestState, recorder UsageRecorder, logger *slog.Logger) {
	state.mu.Lock()
	var reportedModelPtr *string
	if state.reportedModel != "" {
		reportedModelPtr = &state.reportedModel
	}

	var userIDPtr *string
	if state.identity.UserID != "" {
		userIDPtr = &state.identity.UserID
	}

	var featurePtr *string
	if state.identity.Feature != "" {
		featurePtr = &state.identity.Feature
	}

	var errorCategoryPtr *string
	if state.errorCategory != "" {
		errorCategoryPtr = &state.errorCategory
	}

	rec := &database.UsageRecord{
		TenantID:         state.identity.TenantID,
		APIKeyID:         state.identity.APIKeyID,
		UserID:           userIDPtr,
		FeatureName:      featurePtr,
		Provider:         state.provider,
		Endpoint:         chatCompletionsPath,
		RequestedModel:   state.requestedModel,
		ReportedModel:    reportedModelPtr,
		StatusCode:       state.statusCode,
		LatencyMs:        state.latency.Milliseconds(),
		InputTokens:      state.inputTokens,
		OutputTokens:     state.outputTokens,
		CachedTokens:     state.cachedTokens,
		TotalTokens:      state.totalTokens,
		CostMicrodollars: state.costMicrodollars,
		PricingVersion:   state.pricingVersion,
		UsageStatus:      state.usageStatus,
		IsStreaming:      state.isStreaming,
		ErrorCategory:    errorCategoryPtr,
	}
	tenantID := state.identity.TenantID
	model := state.requestedModel
	statusCode := state.statusCode
	state.mu.Unlock()

	// Detached context with 3s deadline ensures persistence completes even if client disconnected
	persistCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := recorder.RecordUsage(persistCtx, rec); err != nil {
		// Log persistence failure with structured fields; never leak prompts/keys
		logger.Error("failed to persist usage record",
			"tenant_id", tenantID,
			"model", model,
			"status_code", statusCode,
			"error", err,
		)
	}
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

type failoverRoundTripper struct {
	primary        http.RoundTripper
	fallbackURL    *url.URL
	fallbackAPIKey string
	fallbackModel  string
	fallbackProv   string
}

func (f *failoverRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := f.primary.RoundTrip(req)
	if f.fallbackURL == nil {
		return resp, err
	}

	// Do not failover if the client canceled the request or its deadline expired
	if req.Context().Err() != nil {
		return resp, err
	}

	shouldFailover := false
	if err != nil {
		// Network dial, connection reset, or I/O failure communicating with primary
		shouldFailover = true
	} else if resp != nil {
		switch resp.StatusCode {
		case http.StatusTooManyRequests, // 429 Rate Limit
			http.StatusBadGateway,         // 502 Upstream Error
			http.StatusServiceUnavailable, // 503 Service Overloaded
			http.StatusGatewayTimeout:     // 504 Gateway Timeout
			shouldFailover = true
		}
	}

	if !shouldFailover {
		return resp, err
	}

	// Safely drain and close primary response body to release socket
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}

	state, _ := req.Context().Value(stateKey).(*requestState)

	// Construct fallback request
	fallbackReq := req.Clone(req.Context())
	fallbackReq.URL.Scheme = f.fallbackURL.Scheme
	fallbackReq.URL.Host = f.fallbackURL.Host
	fallbackReq.Host = f.fallbackURL.Host
	fallbackReq.URL.Path = joinProxyPath(f.fallbackURL.Path, req.URL.Path)
	fallbackReq.URL.RawPath = ""

	if f.fallbackAPIKey != "" {
		fallbackReq.Header.Set("Authorization", "Bearer "+f.fallbackAPIKey)
	}

	var fallbackBody []byte
	targetModel := ""
	if state != nil {
		state.mu.Lock()
		targetModel = state.requestedModel
		origBytes := state.bodyBytes
		state.mu.Unlock()

		if f.fallbackModel != "" && len(origBytes) > 0 {
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(origBytes, &payload); err == nil {
				if modelBytes, err := json.Marshal(f.fallbackModel); err == nil {
					payload["model"] = modelBytes
					if newBody, err := json.Marshal(payload); err == nil {
						fallbackBody = newBody
						targetModel = f.fallbackModel
					}
				}
			}
		}
		if fallbackBody == nil {
			fallbackBody = origBytes
		}
	}

	if len(fallbackBody) > 0 {
		fallbackReq.Body = io.NopCloser(bytes.NewReader(fallbackBody))
		fallbackReq.ContentLength = int64(len(fallbackBody))
		fallbackReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(fallbackBody)), nil
		}
		fallbackReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(fallbackBody)))
	}

	if state != nil {
		state.mu.Lock()
		state.provider = f.fallbackProv
		if targetModel != "" {
			state.requestedModel = targetModel
		}
		state.mu.Unlock()
	}

	return f.primary.RoundTrip(fallbackReq)
}

func joinProxyPath(basePath, reqPath string) string {
	b := strings.TrimRight(basePath, "/")
	r := reqPath
	if (strings.HasSuffix(b, "/openai") || strings.HasSuffix(b, "/v1")) && strings.HasPrefix(r, "/v1/") {
		r = strings.TrimPrefix(r, "/v1")
	}
	r = strings.TrimLeft(r, "/")
	if b == "" {
		return "/" + r
	}
	if r == "" {
		return b
	}
	return b + "/" + r
}
