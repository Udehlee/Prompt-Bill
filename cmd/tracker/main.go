package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/auth"
	"github.com/Udehlee/Prompt-Bill/internal/config"
	"github.com/Udehlee/Prompt-Bill/internal/dashboard"
	"github.com/Udehlee/Prompt-Bill/internal/database"
	"github.com/Udehlee/Prompt-Bill/internal/httpserver"
	"github.com/Udehlee/Prompt-Bill/internal/pricing"
	"github.com/Udehlee/Prompt-Bill/internal/proxy"
	"github.com/Udehlee/Prompt-Bill/internal/reporting"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	provisionKeyFlag := flag.Bool("provision-key", false, "Provision a new tracker API key and exit")
	tenantNameFlag := flag.String("tenant", "default-tenant", "Tenant name for provisioning")
	keyNameFlag := flag.String("key-name", "default-dev-key", "API key friendly name")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	if *provisionKeyFlag {
		return runProvisionKey(cfg, *tenantNameFlag, *keyNameFlag)
	}

	logger := setupLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("starting prompt-bill",
		"port", cfg.Port,
		"upstream_url", cfg.UpstreamBaseURL,
		"database_url", cfg.SafeDatabaseURL(),
		"log_level", cfg.LogLevel,
		"read_header_timeout", cfg.ReadHeaderTimeout.String(),
		"read_timeout", cfg.ReadTimeout.String(),
		"idle_timeout", cfg.IdleTimeout.String(),
		"shutdown_timeout", cfg.ShutdownTimeout.String(),
	)

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dbCancel()

	db, err := database.Connect(dbCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer db.Close()

	if err := database.Migrate(dbCtx, db); err != nil {
		return fmt.Errorf("failed to apply database migrations: %w", err)
	}

	repo := database.NewRepository(db)
	if err := repo.SeedDefaultRates(dbCtx); err != nil {
		logger.Warn("failed to seed default pricing rates", "error", err)
	}

	// Create reverse proxy and auth middleware
	calc := pricing.NewCalculator(repo)
	proxyHandler, err := proxy.Handler(cfg, repo, logger, proxy.WithCalculator(calc))
	if err != nil {
		return fmt.Errorf("failed to create reverse proxy: %w", err)
	}

	authMiddleware := auth.Middleware(repo)
	authenticatedProxy := authMiddleware(proxyHandler)

	reportingHandler := reporting.NewHandler(repo, logger)
	authenticatedReporting := authMiddleware(reportingHandler)
	dashHandler := dashboard.NewHandler()

	server := httpserver.New(
		cfg,
		logger,
		authenticatedProxy,
		httpserver.WithReadinessChecker(repo),
		httpserver.WithReportingHandler(authenticatedReporting),
		httpserver.WithDashboardHandler(dashHandler),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server startup failed: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received, initiating graceful drain")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info("server exited cleanly")
	return nil
}

// runProvisionKey connects to the database, provisions a new API key, prints plaintext once, and exits.
func runProvisionKey(cfg *config.Config, tenantName, keyName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database for provisioning: %w", err)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	repo := database.NewRepository(db)
	key, tenantID, err := repo.ProvisionKey(ctx, tenantName, keyName)
	if err != nil {
		return fmt.Errorf("failed to provision key: %w", err)
	}

	fmt.Println("TRACKER API KEY PROVISIONED\n")
	fmt.Printf("Tenant Name:   %s\n", tenantName)
	fmt.Printf("Tenant ID:     %s\n", tenantID)
	fmt.Printf("Key Name:      %s\n", keyName)
	fmt.Printf("Key Prefix:    %s\n", key.Prefix)
	fmt.Printf("Plaintext Key: %s\n", key.Plaintext)
	fmt.Println("IMPORTANT: Store this key securely")
	fmt.Println(". The plaintext key is NEVER stored in the database and cannot be retrieved again.")

	return nil
}

func setupLogger(levelStr string) *slog.Logger {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
