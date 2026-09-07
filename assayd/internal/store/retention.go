package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaintainPartitions moves default rows to UTC monthly partitions and expires only spans.
// Maintenance is serialized across replicas. A zero TTL retains every span and all scores
// survive pruning. DDL and data moves share one transaction so failed maintenance rolls back.
func (d *Database) MaintainPartitions(ctx context.Context, now time.Time, days int) error {
	if days < 0 || days > 365000 {
		return fmt.Errorf("maintain partitions: retention days must be between 0 and 365000")
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin partition maintenance: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked bool
	err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(638927104)`).Scan(&locked)
	if err != nil {
		return fmt.Errorf("lock partition maintenance: %w", err)
	}
	if !locked {
		return nil
	}
	if err := maintainLockedPartitions(ctx, tx, now.UTC(), days); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit partition maintenance: %w", err)
	}
	return nil
}

func maintainLockedPartitions(ctx context.Context, tx pgx.Tx, now time.Time, days int) error {
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE spans, scores IN ACCESS EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock partition parents: %w", err)
	}
	if days > 0 {
		if err := pruneSpans(ctx, tx, now.AddDate(0, 0, -days)); err != nil {
			return err
		}
	}
	for _, table := range []string{"spans", "scores"} {
		if err := maintainTable(ctx, tx, table, now); err != nil {
			return err
		}
	}
	return nil
}

func maintainTable(ctx context.Context, tx pgx.Tx, table string, now time.Time) error {
	column := "created_at"
	if table == "spans" {
		column = "start_time"
	}
	query := fmt.Sprintf(`SELECT DISTINCT date_trunc('month', %s, 'UTC') FROM %s_default
  ORDER BY 1 LIMIT 24`, column, table)
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("read %s default months: %w", table, err)
	}
	months, err := pgx.CollectRows(rows, pgx.RowTo[time.Time])
	if err != nil {
		return fmt.Errorf("decode %s default months: %w", table, err)
	}
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	months = append(months, month, month.AddDate(0, 1, 0))
	for _, start := range months {
		if err := createMonth(ctx, tx, table, column, start); err != nil {
			return err
		}
	}
	return nil
}

func createMonth(ctx context.Context, tx pgx.Tx, table, column string, start time.Time) error {
	name := table + "_" + start.UTC().Format("200601")
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil {
		return fmt.Errorf("check partition %s: %w", name, err)
	}
	if exists {
		return nil
	}
	end := start.AddDate(0, 1, 0)
	create := fmt.Sprintf(
		`CREATE TABLE %s (LIKE %s INCLUDING DEFAULTS INCLUDING CONSTRAINTS)`, name, table)
	if _, err := tx.Exec(ctx, create); err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	move := fmt.Sprintf(`WITH moved AS (DELETE FROM %s_default WHERE %s >= $1 AND %s < $2
  RETURNING *) INSERT INTO %s SELECT * FROM moved`, table, column, column, name)
	if _, err := tx.Exec(ctx, move, start, end); err != nil {
		return fmt.Errorf("populate %s: %w", name, err)
	}
	attach := fmt.Sprintf(`ALTER TABLE %s ATTACH PARTITION %s FOR VALUES FROM ('%s') TO ('%s')`,
		table, name, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if _, err := tx.Exec(ctx, attach); err != nil {
		return fmt.Errorf("attach %s: %w", name, err)
	}
	return nil
}

func pruneSpans(ctx context.Context, tx pgx.Tx, cutoff time.Time) error {
	rows, err := tx.Query(ctx, `SELECT c.relname FROM pg_inherits i
 JOIN pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'spans'::regclass
 AND c.relname ~ '^spans_[0-9]{6}$'`)
	if err != nil {
		return fmt.Errorf("list span partitions: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("decode span partitions: %w", err)
	}
	for _, name := range names {
		if err := dropExpiredMonth(ctx, tx, name, cutoff); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM spans WHERE start_time < $1`, cutoff); err != nil {
		return fmt.Errorf("prune span boundary rows: %w", err)
	}
	return nil
}

func dropExpiredMonth(ctx context.Context, tx pgx.Tx, name string, cutoff time.Time) error {
	month, err := time.Parse("200601", name[len("spans_"):])
	if err != nil {
		return fmt.Errorf("parse partition %s: %w", name, err)
	}
	if month.AddDate(0, 1, 0).After(cutoff) {
		return nil
	}
	// Check actual rows as well as the name before removing a whole partition.
	var recent bool
	check := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE start_time >= $1)`, name)
	if err := tx.QueryRow(ctx, check, cutoff).Scan(&recent); err != nil {
		return fmt.Errorf("check retained rows in %s: %w", name, err)
	}
	if recent {
		return nil
	}
	if _, err := tx.Exec(ctx, "DROP TABLE "+name); err != nil {
		return fmt.Errorf("drop expired partition %s: %w", name, err)
	}
	return nil
}
