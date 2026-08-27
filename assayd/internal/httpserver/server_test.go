package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeStopsAfterCancellation(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
		})
		result <- serve(ctx, listener, handler, discardLogger())
	}()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		cancel()
		t.Fatalf("request running server: %v", err)
	}
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Errorf("close response body: %v", closeErr)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop within one second")
	}
}

func TestServeReturnsListenerFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("accept failed")
	err := serve(t.Context(), failingListener{err: wantErr}, http.NewServeMux(), discardLogger())
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func TestServeRejectsInvalidAddress(t *testing.T) {
	t.Parallel()

	err := Serve(t.Context(), "invalid address", http.NewServeMux(), discardLogger())
	if err == nil {
		t.Fatal("Serve invalid address succeeded")
	}
}

type failingListener struct {
	err error
}

func (f failingListener) Accept() (net.Conn, error) {
	return nil, f.err
}

func (f failingListener) Close() error {
	return nil
}

func (f failingListener) Addr() net.Addr {
	return testAddress("failure")
}

type testAddress string

func (a testAddress) Network() string {
	return string(a)
}

func (a testAddress) String() string {
	return string(a)
}

var _ io.Closer = failingListener{}
