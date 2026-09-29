package db

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/marioweid/assay/assayd/internal/migrate"
	"github.com/marioweid/assay/assayd/internal/testutil"
)

func TestSessionQueriesOnLargeScopedFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("large disposable PostgreSQL query-plan fixture")
	}
	plans := sessionPlansOnLargeFixture(t)
	for _, plan := range []string{plans.recent, plans.first, plans.cursor, plans.generic} {
		if !strings.Contains(plan, "using traces_session_turn_idx") {
			t.Fatalf("long session must use bounded start-time index:\n%s", plan)
		}
	}
	for _, plan := range []string{plans.cursor, plans.generic} {
		assertSessionCursorIndexSeek(t, plan)
	}
}

type sessionPlans struct {
	recent, first, cursor, generic string
}

func sessionPlansOnLargeFixture(t *testing.T) sessionPlans {
	t.Helper()
	database, err := sql.Open("pgx", testutil.Postgres(t))
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close disposable database: %v", err)
		}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Up(t.Context(), database, logger); err != nil {
		t.Fatalf("apply migrations to disposable database: %v", err)
	}
	projectID, applicationID := seedLargeSessionFixture(t, database)
	if _, err := database.ExecContext(t.Context(), "ANALYZE traces"); err != nil {
		t.Fatalf("analyze session fixture: %v", err)
	}
	anchor := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	logSessionPlan(t, database, "list sessions", listProjectSessions,
		false, nil, "", 25, projectID, applicationID, anchor)
	lateCursor := time.Date(2026, time.September, 1, 10, 0, 9, 0, time.UTC)
	return sessionPlans{
		recent: logSessionPlan(t, database, "recent turns", listRecentSessionTurns,
			projectID, applicationID, "long-session", 10),
		first: logSessionPlan(t, database, "first turns", listProjectSessionTurns,
			projectID, applicationID, "long-session", 10),
		cursor: logSessionPlan(t, database, "late cursor turns", listProjectSessionTurnsAfterCursor,
			projectID, applicationID, "long-session", lateCursor, uuid.Nil, 10),
		generic: genericSessionTurnPlan(t, database, projectID, applicationID, lateCursor),
	}
}

func genericSessionTurnPlan(
	t *testing.T, database *sql.DB, projectID, applicationID uuid.UUID, lateCursor time.Time,
) string {
	t.Helper()
	connection, err := database.Conn(t.Context())
	if err != nil {
		t.Fatalf("reserve prepared-plan connection: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close prepared-plan connection: %v", err)
		}
	})
	if _, err := connection.ExecContext(
		t.Context(), "SET plan_cache_mode = force_generic_plan",
	); err != nil {
		t.Fatalf("force generic query plan: %v", err)
	}
	const prepare = `PREPARE session_turn_plan (uuid, uuid, text,
		timestamptz, uuid, integer) AS `
	if _, err := connection.ExecContext(
		t.Context(), prepare+listProjectSessionTurnsAfterCursor,
	); err != nil {
		t.Fatalf("prepare turn query: %v", err)
	}
	execute := fmt.Sprintf(
		`EXECUTE session_turn_plan ('%s'::uuid, '%s'::uuid, 'long-session',
		 '%s'::timestamptz, '%s'::uuid, 10)`,
		projectID, applicationID, lateCursor.Format(time.RFC3339), uuid.Nil,
	)
	return logSessionPlan(t, connection, "generic late cursor", execute)
}

func assertSessionCursorIndexSeek(t *testing.T, plan string) {
	t.Helper()
	for _, line := range strings.Split(plan, "\n") {
		if strings.Contains(line, "Index Cond:") &&
			strings.Contains(line, "ROW(start_time, id) >") {
			return
		}
	}
	t.Fatalf("late cursor must seek by start-time and ID:\n%s", plan)
}

func seedLargeSessionFixture(t *testing.T, database *sql.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	projectID, applicationID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	foreignProject, foreignApplication := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	for _, project := range []struct {
		id   uuid.UUID
		name string
	}{{projectID, "query-plan"}, {foreignProject, "other-query-plan"}} {
		if _, err := database.ExecContext(t.Context(),
			"INSERT INTO projects (id, name) VALUES ($1, $2)", project.id, project.name,
		); err != nil {
			t.Fatalf("seed project %s: %v", project.name, err)
		}
	}
	for _, application := range []struct {
		id      uuid.UUID
		project uuid.UUID
	}{{applicationID, projectID}, {foreignApplication, foreignProject}} {
		if _, err := database.ExecContext(t.Context(),
			`INSERT INTO applications (id, project_id, name, slug)
			 VALUES ($1, $2, 'Fixture', 'fixture')`, application.id, application.project,
		); err != nil {
			t.Fatalf("seed application %s: %v", application.id, err)
		}
	}
	// The target app has one 10k-turn session, 200 shorter sessions and 45k
	// untagged traces; 35k foreign tagged turns make scope material to the plan.
	const insert = `INSERT INTO traces (id, application_id, otel_trace_id, root_name,
		start_time, end_time, status, session_id)
		SELECT gen_random_uuid(), CASE WHEN n <= 65000 THEN $1::uuid ELSE $2::uuid END,
		decode(md5(n::text), 'hex'), 'answer',
		timestamptz '2026-09-01 10:00:00+00' + n * interval '1 millisecond',
		timestamptz '2026-09-01 10:00:00+00' + n * interval '1 millisecond'
			+ interval '10 milliseconds', 'ok',
		CASE WHEN n <= 10000 THEN 'long-session'
		     WHEN n <= 20000 THEN 'session-' || (n % 200)::text
		     WHEN n > 65000 THEN 'foreign-' || (n % 200)::text END
		FROM generate_series(1, 100000) AS n`
	if _, err := database.ExecContext(
		t.Context(), insert, applicationID, foreignApplication,
	); err != nil {
		t.Fatalf("seed 100k session traces: %v", err)
	}
	return projectID, applicationID
}

type planQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func logSessionPlan(
	t *testing.T, database planQuerier, name, query string, args ...any,
) string {
	t.Helper()
	rows, err := database.QueryContext(t.Context(),
		"EXPLAIN (ANALYZE, BUFFERS) "+query, args...,
	)
	if err != nil {
		t.Fatalf("explain %s: %v", name, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close %s plan rows: %v", name, err)
		}
	}()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("read %s plan: %v", name, err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s plan: %v", name, err)
	}
	t.Logf("%s on 100k traces:\n%s", name, plan.String())
	return plan.String()
}
