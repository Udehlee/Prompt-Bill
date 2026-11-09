package proxy

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/provider/openai"
)

// streamObserver sits between the upstream response and the client. It watches
// the streaming (SSE) data as it passes through, without storing the whole response.
//
// How it works:
// No buffering: bytes read from upstream go straight to the ReverseProxy,
//   which sends them to the client immediately.
// Data is never changed: the parser only reads chunks, it never edits them.
// Parser problems stay hidden: if the parser errors or drops an oversized event,
//   the client's stream keeps working.
// Bounded memory: the parser keeps at most 64KB of an event in memory.
// On Close(): the stream duration, whether it was cancelled, and the token usage
//  are saved to requestState so they can be written to the database.
type streamObserver struct {
	upstream io.ReadCloser
	parser   *openai.StreamParser
	state    *requestState
	calc     CostCalculator
	reqCtx   context.Context

	mu      sync.Mutex
	closed  bool
	readErr error
}

func newStreamObserver(upstream io.ReadCloser, parser *openai.StreamParser, state *requestState, calc CostCalculator, reqCtx context.Context) io.ReadCloser {
	return &streamObserver{
		upstream: upstream,
		parser:   parser,
		state:    state,
		calc:     calc,
		reqCtx:   reqCtx,
	}
}

// Read delegates directly to the upstream response body, feeding incoming bytes
// into the bounded SSE stream parser while returning the unmodified slice to the caller.
func (s *streamObserver) Read(p []byte) (int, error) {
	n, err := s.upstream.Read(p)
	if n > 0 {
		// Feed exact bytes into parser. The parser runs synchronously on the reading goroutine
		// without holding back any bytes from the returned slice p[:n].
		s.parser.Feed(p[:n])
	}
	if err != nil && err != io.EOF {
		s.mu.Lock()
		s.readErr = err
		s.mu.Unlock()
	}
	return n, err
}

// Close finalizes stream accounting metrics and transfers parsed token and model
// metrics into the shared requestState.
func (s *streamObserver) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	readErr := s.readErr
	s.mu.Unlock()

	// Ensure upstream body is closed
	closeErr := s.upstream.Close()

	// Atomically update requestState
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	// Measure total streaming duration
	s.state.latency = time.Since(s.state.startTime)

	// Check for client cancellation
	if s.reqCtx.Err() == context.Canceled {
		s.state.statusCode = 499
		s.state.errorCategory = "client_disconnect"
		s.state.usageStatus = "client_cancelled"
		return closeErr
	}
	if s.reqCtx.Err() == context.DeadlineExceeded {
		s.state.statusCode = http.StatusGatewayTimeout
		s.state.errorCategory = "client_timeout"
		s.state.usageStatus = "timeout"
		return closeErr
	}

	// Check for upstream network interruption during reading
	if readErr != nil && readErr != io.EOF {
		s.state.errorCategory = "upstream_disconnect"
		s.state.usageStatus = "interrupted"
		return closeErr
	}

	// 3. Finalize SSE parser
	res := s.parser.Finalize()
	if res.ReportedModel != "" {
		s.state.reportedModel = res.ReportedModel
	}

	if res.ExtractedUsage != nil {
		s.state.inputTokens = res.ExtractedUsage.InputTokens
		s.state.outputTokens = res.ExtractedUsage.OutputTokens
		s.state.cachedTokens = res.ExtractedUsage.CachedTokens
		s.state.totalTokens = res.ExtractedUsage.TotalTokens
		s.state.usageStatus = res.ExtractedUsage.UsageStatus

		// Calculate cost if calculator is present and usage status is complete.
		// Use a short-lived context so rate lookup succeeds even if the client disconnected
		// at the very moment the final stream chunk completed.
		if s.calc != nil && s.state.usageStatus == "complete" {
			calcCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			costRes, err := s.calc.CalculateCost(
				calcCtx,
				s.state.provider,
				s.state.requestedModel,
				s.state.inputTokens,
				s.state.outputTokens,
				s.state.cachedTokens,
				s.state.startTime,
			)
			cancel()
			if err == nil && costRes != nil {
				s.state.costMicrodollars = costRes.CostMicrodollars
				s.state.pricingVersion = costRes.PricingVersion
				if costRes.UsageStatus != "" && costRes.UsageStatus != "complete" {
					s.state.usageStatus = costRes.UsageStatus
				}
			}
		}
	} else {
		switch {
		case res.OversizedEvent:
			s.state.usageStatus = "oversized_event"
		case res.MalformedJSON:
			s.state.usageStatus = "unparseable_stream"
		case !res.SawDone:
			// Stream ended without [DONE] and without usage
			s.state.errorCategory = "upstream_disconnect"
			s.state.usageStatus = "interrupted"
		default:
			// Stream completed normally with [DONE], but provider never sent a usage chunk
			s.state.usageStatus = "missing_usage"
		}
	}

	return closeErr
}
