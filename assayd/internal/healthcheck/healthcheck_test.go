package healthcheck

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunAcceptsReadyResponseAndNormalizesHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/readyz" {
				t.Errorf("path = %q, want /readyz", request.URL.Path)
			}
			writer.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split test server address: %v", err)
	}

	for _, address := range []string{":" + port, "0.0.0.0:" + port} {
		if err := Run(t.Context(), address, server.Client()); err != nil {
			t.Errorf("Run(%q): %v", address, err)
		}
	}
}

func TestRunReportsUnreadyResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		if _, err := writer.Write([]byte("database unavailable\n")); err != nil {
			t.Errorf("write test response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	err := Run(t.Context(), server.Listener.Addr().String(), server.Client())
	if err == nil {
		t.Fatal("Run unready response succeeded")
	}
	hasStatus := strings.Contains(err.Error(), "503")
	hasBody := strings.Contains(err.Error(), "database unavailable")
	if !hasStatus || !hasBody {
		t.Errorf("error = %q, want status and bounded response body", err)
	}
}

func TestRunRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if _, err := writer.Write([]byte(strings.Repeat("x", 1<<20))); err != nil {
			t.Errorf("write test response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	err := Run(t.Context(), server.Listener.Addr().String(), server.Client())
	if err == nil {
		t.Fatal("Run oversized response succeeded")
	}
	if !strings.Contains(err.Error(), "response exceeds") {
		t.Errorf("error = %q, want response size context", err)
	}
}

func TestRunReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved listener: %v", err)
	}

	client := &http.Client{Timeout: time.Second}
	err = Run(context.Background(), address, client)
	if err == nil {
		t.Fatal("Run connection failure succeeded")
	}
	if !strings.Contains(err.Error(), "request readiness endpoint") {
		t.Errorf("error = %q, want request context", err)
	}
}
