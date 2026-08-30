// Package app wires Assay's process-scoped dependencies and startup sequence.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/marioweid/assay/assayd/internal/api"
	"github.com/marioweid/assay/assayd/internal/config"
	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/httpserver"
	"github.com/marioweid/assay/assayd/internal/migrate"
	"github.com/marioweid/assay/assayd/internal/otlp"
	"github.com/marioweid/assay/assayd/internal/store"
	"github.com/marioweid/assay/assayd/internal/worker"

	"github.com/google/uuid"
)

// App contains Assay's process-scoped configuration and collaborators.
type App struct {
	config      config.Config
	logger      *slog.Logger
	database    *store.Database
	cipher      *secretcrypto.Cipher
	service     *domain.Service
	traces      *domain.TraceService
	evaluations *domain.EvaluationService
	workers     *worker.Pool
}

// New opens Postgres and applies migrations before returning a runnable application.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	database, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open application database: %w", err)
	}
	if err := migrate.Up(ctx, database.MigrationDB(), logger); err != nil {
		return nil, errors.Join(
			fmt.Errorf("migrate application database: %w", err),
			database.Close(),
		)
	}
	cipher, err := secretcrypto.New(cfg.EncryptionKey[:])
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create application secret cipher: %w", err),
			database.Close(),
		)
	}
	service := domain.NewService(database, cipher)
	evaluations := domain.NewEvaluationService(database, cipher, cfg.JobMaxAttempts)
	runner := worker.NewRunner(
		database,
		evaluations,
		&http.Client{Timeout: 60 * time.Second},
		domain.JudgeDefaults{
			BaseURL: cfg.JudgeBaseURL, APIKey: cfg.JudgeAPIKey, Model: cfg.JudgeModel,
		},
	)
	return &App{
		config:      cfg,
		logger:      logger,
		database:    database,
		cipher:      cipher,
		service:     service,
		traces:      domain.NewTraceService(database, service),
		evaluations: evaluations,
		workers: worker.NewPool(
			database, runner, logger, uuid.NewString(), cfg.WorkerConcurrency,
		),
	}, nil
}

// Serve runs the HTTP server until its context is canceled or serving fails.
func (a *App) Serve(ctx context.Context) error {
	serveCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		a.workers.Run(serveCtx)
	}()
	defer func() {
		cancel()
		workers.Wait()
	}()
	handler := httpserver.NewMux(a.database, a.logger)
	api.Register(handler, api.Dependencies{
		Service: a.service, Traces: a.traces, Evaluations: a.evaluations,
		AdminToken: a.config.AdminToken, Logger: a.logger,
	})
	otlp.Register(handler, a.service, a.traces, a.config.AutoCreateApps, a.logger)
	if err := httpserver.Serve(serveCtx, a.config.HTTPAddr, handler, a.logger); err != nil {
		return fmt.Errorf("run application HTTP server: %w", err)
	}
	return nil
}

// Close releases every process-scoped resource owned by App.
func (a *App) Close() error {
	if err := a.database.Close(); err != nil {
		return fmt.Errorf("close application database: %w", err)
	}
	return nil
}
