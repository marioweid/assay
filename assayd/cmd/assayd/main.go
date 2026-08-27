// Command assayd runs the Assay service and its operational healthcheck.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marioweid/assay/assayd/internal/app"
	"github.com/marioweid/assay/assayd/internal/config"
	"github.com/marioweid/assay/assayd/internal/healthcheck"
)

const (
	defaultHTTPAddr  = ":8080"
	probeHTTPTimeout = 5 * time.Second
)

type dependencies struct {
	loadConfig     func() (config.Config, error)
	runServer      func(context.Context, config.Config, *slog.Logger) error
	runHealthcheck func(context.Context, string, *http.Client) error
	getenv         func(string) string
	stdout         io.Writer
	stderr         io.Writer
}

func main() {
	os.Exit(execute())
}

func execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], productionDependencies())
}

func run(ctx context.Context, args []string, deps dependencies) int {
	if len(args) > 1 {
		return usageError(deps.stderr)
	}
	if len(args) == 1 {
		switch args[0] {
		case "healthcheck":
			return runHealthcheck(ctx, deps)
		case "serve":
		default:
			return usageError(deps.stderr)
		}
	}

	cfg, err := deps.loadConfig()
	if err != nil {
		return reportError(deps.stderr, err)
	}
	logger := newLogger(cfg.LogFormat, deps.stdout)
	if err := deps.runServer(ctx, cfg, logger); err != nil {
		return reportError(deps.stderr, err)
	}
	return 0
}

func runHealthcheck(ctx context.Context, deps dependencies) int {
	addr := deps.getenv("ASSAY_HTTP_ADDR")
	if addr == "" {
		addr = defaultHTTPAddr
	}
	client := &http.Client{Timeout: probeHTTPTimeout}
	if err := deps.runHealthcheck(ctx, addr, client); err != nil {
		return reportError(deps.stderr, err)
	}
	return 0
}

func productionDependencies() dependencies {
	return dependencies{
		loadConfig:     config.Load,
		runServer:      serve,
		runHealthcheck: healthcheck.Run,
		getenv:         os.Getenv,
		stdout:         os.Stdout,
		stderr:         os.Stderr,
	}
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) (returnErr error) {
	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, application.Close())
	}()
	return application.Serve(ctx)
}

func newLogger(format string, output io.Writer) *slog.Logger {
	if format == "text" {
		return slog.New(slog.NewTextHandler(output, nil))
	}
	return slog.New(slog.NewJSONHandler(output, nil))
}

func usageError(writer io.Writer) int {
	if _, err := fmt.Fprintln(writer, "usage: assayd [serve|healthcheck]"); err != nil {
		return 1
	}
	return 2
}

func reportError(writer io.Writer, err error) int {
	if _, writeErr := fmt.Fprintf(writer, "assayd: %v\n", err); writeErr != nil {
		return 1
	}
	return 1
}
