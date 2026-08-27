package store

import (
	"context"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/testutil"
)

func TestOpenRejectsMalformedDSN(t *testing.T) {
	t.Parallel()

	const secret = "super-secret-password"
	_, err := Open(t.Context(), "postgres://assay:"+secret+"@[::1")
	if err == nil {
		t.Fatal("Open malformed DSN succeeded")
	}
	if !strings.Contains(err.Error(), "parse database URL") {
		t.Errorf("error = %q, want parse database URL context", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error leaked database password: %q", err)
	}
}

func TestDatabaseReadyAndClose(t *testing.T) {
	dsn := testutil.Postgres(t)
	database, err := Open(t.Context(), dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	if err := database.Ready(t.Context()); err != nil {
		t.Fatalf("database readiness: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if err := database.Ready(context.Background()); err == nil {
		t.Fatal("readiness after close succeeded")
	}
}
