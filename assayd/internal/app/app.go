// Package app wires Assay's process-scoped dependencies and startup sequence.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/marioweid/assay/assayd/internal/config"
	"github.com/marioweid/assay/assayd/internal/httpserver"
	"github.com/marioweid/assay/assayd/internal/migrate"
	"github.com/marioweid/assay/assayd/internal/store"
)

// App contains Assay's process-scoped configuration and collaborators.
type App struct {
	config   config.Config
	logger   *slog.Logger
	database *store.Database
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
	return &App{config: cfg, logger: logger, database: database}, nil
}

// Serve runs the HTTP server until its context is canceled or serving fails.
func (a *App) Serve(ctx context.Context) error {
	handler := httpserver.NewHandler(a.database, a.logger)
	if err := httpserver.Serve(ctx, a.config.HTTPAddr, handler, a.logger); err != nil {
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
