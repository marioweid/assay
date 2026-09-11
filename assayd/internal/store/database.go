// Package store owns Assay's Postgres connections and generated queries.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Database owns the application pool and the database/sql handle used by migrations.
type Database struct {
	pool        *pgxpool.Pool
	migrationDB *sql.DB
	queries     *db.Queries
}

type deleteJobLocker func(context.Context, *db.Queries) error

func (d *Database) deleteWithJobLock(
	ctx context.Context,
	operation string,
	lockJobs deleteJobLocker,
	remove func(*db.Queries) error,
) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s transaction: %w", operation, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	if err := lockJobs(ctx, queries); err != nil {
		return mapStoreError("lock jobs for deletion", err)
	}
	if err := remove(queries); err != nil {
		return mapStoreError(operation, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s transaction: %w", operation, err)
	}
	return nil
}

func lockApplicationJobs(applicationID uuid.UUID) deleteJobLocker {
	return func(ctx context.Context, queries *db.Queries) error {
		_, err := queries.LockApplicationJobsForDelete(ctx, applicationID)
		return err
	}
}

func lockDatasetJobs(datasetID uuid.UUID) deleteJobLocker {
	return func(ctx context.Context, queries *db.Queries) error {
		_, err := queries.LockDatasetJobsForDelete(ctx, datasetID)
		return err
	}
}

func lockEvalRunJobs(runID uuid.UUID) deleteJobLocker {
	return func(ctx context.Context, queries *db.Queries) error {
		_, err := queries.LockEvalRunJobsForDelete(ctx, nullableUUID(&runID))
		return err
	}
}

func lockProjectJobs(projectID uuid.UUID) deleteJobLocker {
	return func(ctx context.Context, queries *db.Queries) error {
		_, err := queries.LockProjectJobsForDelete(ctx, projectID)
		return err
	}
}

func lockTraceJobs(traceID uuid.UUID) deleteJobLocker {
	return func(ctx context.Context, queries *db.Queries) error {
		_, err := queries.LockTraceJobsForDelete(ctx, nullableUUID(&traceID))
		return err
	}
}

// Open connects both Postgres handles and verifies that the database is reachable.
func Open(ctx context.Context, dsn string) (*Database, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create application database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping application database pool: %w", err)
	}

	migrationDB := stdlib.OpenDB(*poolConfig.ConnConfig)
	if err := migrationDB.PingContext(ctx); err != nil {
		pool.Close()
		pingErr := fmt.Errorf("ping migration database handle: %w", err)
		if closeErr := migrationDB.Close(); closeErr != nil {
			return nil, errors.Join(
				pingErr,
				fmt.Errorf("close failed migration database handle: %w", closeErr),
			)
		}
		return nil, pingErr
	}

	return &Database{
		pool:        pool,
		migrationDB: migrationDB,
		queries:     db.New(pool),
	}, nil
}

// Ready verifies that Postgres can execute an application query.
func (d *Database) Ready(ctx context.Context) error {
	ready, err := d.queries.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("execute database health check: %w", err)
	}
	if !ready {
		return errors.New("database health check returned false")
	}
	return nil
}

// MigrationDB returns the database/sql handle owned by Database for migration operations.
func (d *Database) MigrationDB() *sql.DB {
	return d.migrationDB
}

// Close releases all database connections.
func (d *Database) Close() error {
	d.pool.Close()
	if err := d.migrationDB.Close(); err != nil {
		return fmt.Errorf("close migration database handle: %w", err)
	}
	return nil
}
