// Package healthcheck probes a running assayd process from its container.
package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maxResponseBytes = 4 * 1024

// Run requests the local readiness endpoint and requires an HTTP 200 response.
func Run(ctx context.Context, addr string, client *http.Client) error {
	endpoint, err := readinessEndpoint(addr)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create readiness request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request readiness endpoint: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return errors.Join(
			wrapError("read readiness response", readErr),
			wrapError("close readiness response", closeErr),
		)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("readiness response exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"readiness endpoint returned %s: %s",
			response.Status,
			strings.TrimSpace(string(body)),
		)
	}
	return nil
}

func readinessEndpoint(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("parse HTTP address %q: %w", addr, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("parse HTTP address %q: invalid port", addr)
	}
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("127.0.0.1", port),
		Path:   "/readyz",
	}).String(), nil
}

func wrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
