package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/httpapi"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/store/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const shutdownTimeout = 10 * time.Second

var (
	openDatabase         = sql.Open
	newOIDCAuthenticator = func(ctx context.Context, cfg config.Config, repository *postgres.Repository, now func() time.Time) (*auth.Authenticator, error) {
		return auth.NewOIDCAuthenticator(ctx, cfg, repository, now)
	}
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Getenv); err != nil {
		logStartupFailure(log.Default(), err)
		os.Exit(1)
	}
}

func logStartupFailure(logger *log.Logger, _ error) {
	logger.Print("api stopped")
}

func run(ctx context.Context, lookup func(string) string) (runErr error) {
	cfg, err := config.Load(lookup)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	db, err := openDatabase("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil && runErr == nil {
			runErr = fmt.Errorf("close database: %w", err)
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	handler, err := newHandler(ctx, cfg, db)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.ListenAndServe() }()

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve API: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shut down API: %w", err)
	}
	if err := <-serveErrors; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve API: %w", err)
	}
	return nil
}

func newHandler(ctx context.Context, cfg config.Config, db *sql.DB) (http.Handler, error) {
	repository := postgres.NewRepository(db)
	// Request routes are outside Task 6, but the executable owns the single
	// application service that will serve them alongside the auth routes.
	_ = requests.NewService(repository)
	authenticator, err := newOIDCAuthenticator(ctx, cfg, repository, time.Now)
	if err != nil {
		return nil, fmt.Errorf("initialize OIDC authenticator: %w", err)
	}
	selectionStore := auth.NewSelectionService(repository, cfg.SessionIdleTTL, cfg.SessionAbsoluteTTL)
	return httpapi.NewRouter(httpapi.Dependencies{
		Config:         cfg,
		Authenticator:  authenticator,
		SelectionStore: selectionStore,
		SessionStore:   repository,
		Now:            time.Now,
	}), nil
}
