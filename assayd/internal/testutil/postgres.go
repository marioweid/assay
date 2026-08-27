// Package testutil provides integration-test fixtures shared across backend packages.
package testutil

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres:18.6-trixie@sha256:" +
	"4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280"

// Postgres starts a disposable Postgres instance and returns its connection string.
func Postgres(t *testing.T) string {
	t.Helper()
	configureRyukForWindows(t)
	if os.Getenv("CI") == "" {
		testcontainers.SkipIfProviderIsNotHealthy(t)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	container, err := postgres.Run(ctx, postgresImage, postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start Postgres testcontainer: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get Postgres testcontainer connection string: %v", err)
	}
	return dsn
}

func configureRyukForWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" || os.Getenv("TESTCONTAINERS_RYUK_DISABLED") != "" {
		return
	}
	// Rancher Desktop cannot keep Ryuk alive through its Windows Docker proxy.
	if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
		t.Fatalf("disable testcontainers Ryuk on Windows: %v", err)
	}
}
