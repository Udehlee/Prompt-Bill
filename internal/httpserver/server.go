package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/config"
)

// ReadinessChecker verifies database connectivity for readiness probes.
type ReadinessChecker interface {
	Ping(ctx context.Context) error
}

// Server wraps the standard library http.Server and provides controlled lifecycle management.
type Server struct {
	httpServer       *http.Server
	logger           *slog.Logger
	cfg              *config.Config
	mux              *http.ServeMux
	readyChecker     ReadinessChecker
	reportingHandler http.Handler
	dashboardHandler http.Handler
}

// ServerOption configures optional server features.
type ServerOption func(*Server)

// WithReadinessChecker configures the database connectivity checker for GET /readyz.
func WithReadinessChecker(checker ReadinessChecker) ServerOption {
	return func(s *Server) {
		s.readyChecker = checker
	}
}

// WithReportingHandler mounts the reporting API routes onto the server router.
func WithReportingHandler(handler http.Handler) ServerOption {
	return func(s *Server) {
		s.reportingHandler = handler
	}
}

// WithDashboardHandler mounts the minimal HTML dashboard routes onto the server router.
func WithDashboardHandler(handler http.Handler) ServerOption {
	return func(s *Server) {
		s.dashboardHandler = handler
	}
}

// New creates an HTTP server with timeouts to protect against slow or idle connections.
// WriteTimeout is disabled to allow long-running LLM streaming responses.
// Streaming connections are managed through context cancellation and upstream timeouts.
func New(cfg *config.Config, logger *slog.Logger, proxyHandler http.Handler, opts ...ServerOption) *Server {
	mux := http.NewServeMux()

	s := &Server{
		logger: logger,
		cfg:    cfg,
		mux:    mux,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.routes(proxyHandler)

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		// WriteTimeout is intentionally 0 to permit progressive SSE streaming without arbitrary cutoffs.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	return s
}

func (s *Server) routes(proxyHandler http.Handler) {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	if proxyHandler != nil {
		s.mux.Handle("/v1/chat/completions", proxyHandler)
	}
	if s.reportingHandler != nil {
		s.mux.Handle("/api/v1/", s.reportingHandler)
	}
	if s.dashboardHandler != nil {
		s.mux.Handle("GET /dashboard", s.dashboardHandler)
		s.mux.Handle("GET /dashboard/", s.dashboardHandler)
		s.mux.Handle("GET /{$}", s.dashboardHandler)
		s.mux.Handle("GET /style.css", s.dashboardHandler)
		s.mux.Handle("GET /dashboard.js", s.dashboardHandler)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}


func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.readyChecker != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.readyChecker.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "not_ready",
				"message": "database connectivity failed",
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	})
}

func (s *Server) Start() error {
	s.logger.Info("http server listening",
		"addr", s.httpServer.Addr,
		"read_header_timeout", s.cfg.ReadHeaderTimeout.String(),
		"read_timeout", s.cfg.ReadTimeout.String(),
		"idle_timeout", s.cfg.IdleTimeout.String(),
	)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Serve(l net.Listener) error {
	return s.httpServer.Serve(l)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down http server")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

