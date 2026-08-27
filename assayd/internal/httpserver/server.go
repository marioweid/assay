package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// Serve listens on addr and serves requests until cancellation or failure.
func Serve(
	ctx context.Context,
	addr string,
	handler http.Handler,
	logger *slog.Logger,
) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen HTTP on %q: %w", addr, err)
	}
	return serve(ctx, listener, handler, logger)
}

func serve(
	ctx context.Context,
	listener net.Listener,
	handler http.Handler,
	logger *slog.Logger,
) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	logger.Info("HTTP server started", "address", listener.Addr().String())
	select {
	case err := <-serveResult:
		return serveError(err)
	case <-ctx.Done():
		logger.Info("HTTP server stopping")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	return errors.Join(serveError(<-serveResult), wrapShutdownError(shutdownErr))
}

func serveError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve HTTP: %w", err)
}

func wrapShutdownError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("shut down HTTP server: %w", err)
}
