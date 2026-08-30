package otlp

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

const validAPIKey = "asy_0123456789ABCDEFGHIJKLMNOPQRSTUV"

func TestHandlerAcceptsPlainAndGzipJSON(t *testing.T) {
	for _, encoding := range []string{"", "gzip"} {
		t.Run(encoding, func(t *testing.T) {
			ingestor := &traceIngestorFake{application: domain.Application{ID: uuid.Must(uuid.NewV7())}}
			handler := newOTLPTestHandler(&authenticatorFake{projectID: uuid.Must(uuid.NewV7())}, ingestor)
			body := []byte(traceFixture)
			if encoding == "gzip" {
				body = gzipBytes(t, body)
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+validAPIKey)
			if encoding != "" {
				request.Header.Set("Content-Encoding", encoding)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "{}" {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if len(ingestor.traces) != 1 || ingestor.slug != "support-bot" {
				t.Fatalf("ingested traces/slug = %#v/%q", ingestor.traces, ingestor.slug)
			}
		})
	}
}

func TestHandlerReturnsPartialSuccessForUnknownApplication(t *testing.T) {
	ingestor := &traceIngestorFake{resolveErr: domain.ErrNotFound}
	handler := newOTLPTestHandler(&authenticatorFake{projectID: uuid.Must(uuid.NewV7())}, ingestor)
	request := validOTLPRequest(traceFixture)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	partialSuccess := strings.Contains(response.Body.String(), `"rejectedSpans":"2"`)
	if response.Code != http.StatusOK || !partialSuccess {
		t.Fatalf("partial success = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerRejectsInvalidProtocolAndCredentials(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		encoding    string
		body        string
		withKey     bool
		wantStatus  int
	}{
		{name: "missing key", contentType: "application/json", body: `{}`, wantStatus: 401},
		{
			name: "wrong media", contentType: "application/x-protobuf", body: `{}`,
			withKey: true, wantStatus: 415,
		},
		{
			name: "wrong encoding", contentType: "application/json", encoding: "br",
			body: `{}`, withKey: true, wantStatus: 415,
		},
		{
			name: "malformed JSON", contentType: "application/json", body: `{`,
			withKey: true, wantStatus: 400,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newOTLPTestHandler(
				&authenticatorFake{projectID: uuid.Must(uuid.NewV7())},
				&traceIngestorFake{},
			)
			request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("Content-Encoding", test.encoding)
			if test.withKey {
				request.Header.Set("x-api-key", validAPIKey)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body = %s",
					response.Code,
					test.wantStatus,
					response.Body.String(),
				)
			}
			var statusBody struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &statusBody); err != nil {
				t.Fatalf("decode OTLP status: %v", err)
			}
			if statusBody.Code == 0 || statusBody.Message == "" {
				t.Fatalf("OTLP status body = %#v", statusBody)
			}
		})
	}
}

func TestReadRequestBodyEnforcesDecompressedLimit(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("12345"))
	if _, err := readRequestBody(request, 4); !errors.Is(err, errBodyTooLarge) {
		t.Fatalf("read oversized body error = %v, want errBodyTooLarge", err)
	}
}

func newOTLPTestHandler(authenticator APIKeyAuthenticator, ingestor TraceIngestor) http.Handler {
	mux := http.NewServeMux()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	Register(mux, authenticator, ingestor, false, logger)
	return mux
}

func validOTLPRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", validAPIKey)
	return request
}

func gzipBytes(t *testing.T, body []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(body); err != nil {
		t.Fatalf("write gzip body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip body: %v", err)
	}
	return compressed.Bytes()
}

type authenticatorFake struct {
	projectID uuid.UUID
	err       error
}

func (f *authenticatorFake) AuthenticateAPIKey(
	_ context.Context,
	_ string,
) (uuid.UUID, error) {
	return f.projectID, f.err
}

type traceIngestorFake struct {
	application domain.Application
	resolveErr  error
	slug        string
	traces      []domain.Trace
}

func (f *traceIngestorFake) ResolveApplication(
	_ context.Context,
	_ uuid.UUID,
	slug string,
	_ bool,
) (domain.Application, error) {
	f.slug = slug
	return f.application, f.resolveErr
}

func (f *traceIngestorFake) Ingest(
	_ context.Context,
	_ uuid.UUID,
	traces []domain.Trace,
) error {
	f.traces = traces
	return nil
}
