// Package migrate applies Assay's embedded database migrations.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/marioweid/assay/assayd/db/migrations"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Up applies every pending migration under a Postgres session lock.
func Up(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	sessionLocker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration session locker: %w", err)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrations.Files,
		goose.WithSlog(logger),
		goose.WithSessionLocker(sessionLocker),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	return nil
}
