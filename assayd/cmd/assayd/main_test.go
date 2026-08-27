package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/config"
)

func TestRunDispatchesHealthcheckWithoutServerConfig(t *testing.T) {
	t.Parallel()

	called := false
	deps := testDependencies()
	deps.loadConfig = func() (config.Config, error) {
		t.Fatal("healthcheck loaded server configuration")
		return config.Config{}, nil
	}
	deps.runHealthcheck = func(_ context.Context, addr string, _ *http.Client) error {
		called = true
		if addr != ":9000" {
			t.Errorf("healthcheck address = %q, want :9000", addr)
		}
		return nil
	}
	deps.getenv = func(string) string { return ":9000" }

	if code := run(t.Context(), []string{"healthcheck"}, deps); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !called {
		t.Error("healthcheck was not called")
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	deps := testDependencies()
	var stderr bytes.Buffer
	deps.stderr = &stderr

	if code := run(t.Context(), []string{"unknown"}, deps); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunReportsConfigurationError(t *testing.T) {
	t.Parallel()

	deps := testDependencies()
	var stderr bytes.Buffer
	deps.stderr = &stderr
	deps.loadConfig = func() (config.Config, error) {
		return config.Config{}, errors.New("missing ASSAY_DATABASE_URL")
	}

	if code := run(t.Context(), nil, deps); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing ASSAY_DATABASE_URL") {
		t.Errorf("stderr = %q, want configuration error", stderr.String())
	}
}

func TestRunDoesNotPrintConfigurationSecrets(t *testing.T) {
	t.Parallel()

	deps := testDependencies()
	var stderr bytes.Buffer
	deps.stderr = &stderr
	deps.loadConfig = func() (config.Config, error) {
		return config.Config{
			AdminToken:  "admin-secret",
			JudgeAPIKey: "judge-secret",
			LogFormat:   "text",
		}, nil
	}
	deps.runServer = func(context.Context, config.Config, *slog.Logger) error {
		return errors.New("server failed")
	}

	if code := run(t.Context(), nil, deps); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	for _, secret := range []string{"admin-secret", "judge-secret"} {
		if strings.Contains(stderr.String(), secret) {
			t.Errorf("stderr leaked %q: %s", secret, stderr.String())
		}
	}
}

func testDependencies() dependencies {
	return dependencies{
		loadConfig:     func() (config.Config, error) { return config.Config{}, nil },
		runServer:      func(context.Context, config.Config, *slog.Logger) error { return nil },
		runHealthcheck: func(context.Context, string, *http.Client) error { return nil },
		getenv:         func(string) string { return "" },
		stdout:         io.Discard,
		stderr:         io.Discard,
	}
}
