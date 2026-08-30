package otlp

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/marioweid/assay/assayd/internal/auth"
	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

const maxRequestBytes int64 = 64 << 20

var (
	errBodyTooLarge        = errors.New("OTLP request exceeds decompressed size limit")
	errUnsupportedEncoding = errors.New("unsupported OTLP content encoding")
)

// APIKeyAuthenticator resolves a plaintext project key to its owning project.
type APIKeyAuthenticator interface {
	AuthenticateAPIKey(context.Context, string) (uuid.UUID, error)
}

// TraceIngestor resolves applications and persists accepted traces.
type TraceIngestor interface {
	ResolveApplication(context.Context, uuid.UUID, string, bool) (domain.Application, error)
	Ingest(context.Context, uuid.UUID, []domain.Trace) error
}

type handler struct {
	authenticator APIKeyAuthenticator
	ingestor      TraceIngestor
	autoCreate    bool
	logger        *slog.Logger
}

// Register mounts JSON OTLP/HTTP trace ingestion on a standard-library mux.
func Register(
	router *http.ServeMux,
	authenticator APIKeyAuthenticator,
	ingestor TraceIngestor,
	autoCreate bool,
	logger *slog.Logger,
) {
	h := &handler{
		authenticator: authenticator,
		ingestor:      ingestor,
		autoCreate:    autoCreate,
		logger:        logger,
	}
	router.HandleFunc("POST /v1/traces", h.export)
}

func (h *handler) export(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := h.authenticate(writer, request)
	if !ok {
		return
	}
	if !isJSONContentType(request.Header.Get("Content-Type")) {
		writeStatus(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	payload, err := readRequestBody(request, maxRequestBytes)
	if err != nil {
		h.writeReadError(writer, err)
		return
	}
	export, err := DecodeJSON(payload)
	if err != nil {
		writeStatus(writer, http.StatusBadRequest, err.Error())
		return
	}
	traces, rejected, ok := h.mapGroups(writer, request, projectID, Groups(export))
	if !ok {
		return
	}
	if err := h.ingestor.Ingest(request.Context(), projectID, traces); err != nil {
		h.logger.Error("persist OTLP traces", "error", err)
		writeStatus(writer, http.StatusServiceUnavailable, "Trace storage is unavailable")
		return
	}
	writeSuccess(writer, rejected)
}

func (h *handler) authenticate(
	writer http.ResponseWriter,
	request *http.Request,
) (uuid.UUID, bool) {
	token, ok := auth.APIKeyFromHeaders(
		request.Header.Get("Authorization"),
		request.Header.Get("x-api-key"),
	)
	if !ok {
		writeStatus(writer, http.StatusUnauthorized, "Missing or conflicting API key")
		return uuid.Nil, false
	}
	projectID, err := h.authenticator.AuthenticateAPIKey(request.Context(), token)
	if errors.Is(err, domain.ErrUnauthorized) {
		writeStatus(writer, http.StatusUnauthorized, "Invalid API key")
		return uuid.Nil, false
	}
	if err != nil {
		h.logger.Error("authenticate OTLP request", "error", err)
		writeStatus(writer, http.StatusServiceUnavailable, "Authentication storage is unavailable")
		return uuid.Nil, false
	}
	return projectID, true
}

func (h *handler) mapGroups(
	writer http.ResponseWriter,
	request *http.Request,
	projectID uuid.UUID,
	groups []ResourceGroup,
) ([]domain.Trace, Rejections, bool) {
	traces := make([]domain.Trace, 0)
	var rejected Rejections
	for _, group := range groups {
		application, err := h.ingestor.ResolveApplication(
			request.Context(), projectID, group.Slug(), h.autoCreate,
		)
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalid) {
			rejected.Spans += int64(group.SpanCount())
			rejected.Reasons = append(rejected.Reasons, "application could not be resolved")
			continue
		}
		if err != nil {
			h.logger.Error("resolve OTLP application", "error", err)
			writeStatus(writer, http.StatusServiceUnavailable, "Application storage is unavailable")
			return nil, Rejections{}, false
		}
		mapped, mappingRejections, err := MapResourceSpans(group, application)
		if err != nil {
			h.logger.Error("map OTLP resource spans", "error", err)
			writeStatus(writer, http.StatusServiceUnavailable, "Trace mapping is unavailable")
			return nil, Rejections{}, false
		}
		traces = append(traces, mapped...)
		rejected.Spans += mappingRejections.Spans
		rejected.Reasons = append(rejected.Reasons, mappingRejections.Reasons...)
	}
	return traces, rejected, true
}

func (h *handler) writeReadError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBodyTooLarge):
		writeStatus(writer, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, errUnsupportedEncoding):
		writeStatus(writer, http.StatusUnsupportedMediaType, err.Error())
	default:
		writeStatus(writer, http.StatusBadRequest, "Invalid compressed OTLP request")
	}
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

func readRequestBody(request *http.Request, limit int64) ([]byte, error) {
	reader := io.Reader(request.Body)
	if encoding := request.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		if encoding != "gzip" {
			return nil, errUnsupportedEncoding
		}
		gzipReader, err := gzip.NewReader(request.Body)
		if err != nil {
			return nil, fmt.Errorf("open gzip OTLP request: %w", err)
		}
		defer func() {
			_ = gzipReader.Close()
		}()
		reader = gzipReader
	}
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read OTLP request: %w", err)
	}
	if int64(len(payload)) > limit {
		return nil, errBodyTooLarge
	}
	return payload, nil
}

func writeSuccess(writer http.ResponseWriter, rejected Rejections) {
	if rejected.Spans == 0 {
		writeJSON(writer, http.StatusOK, struct{}{})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"partialSuccess": map[string]string{
			"rejectedSpans": strconv.FormatInt(rejected.Spans, 10),
			"errorMessage":  strings.Join(uniqueReasons(rejected.Reasons), "; "),
		},
	})
}

func uniqueReasons(reasons []string) []string {
	unique := make([]string, 0, len(reasons))
	seen := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		if _, found := seen[reason]; found {
			continue
		}
		seen[reason] = struct{}{}
		unique = append(unique, reason)
	}
	return unique
}

func writeStatus(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]any{
		"code":    grpcStatusCode(status),
		"message": message,
	})
}

func grpcStatusCode(httpStatus int) int {
	switch httpStatus {
	case http.StatusBadRequest, http.StatusUnsupportedMediaType:
		return 3 // InvalidArgument
	case http.StatusUnauthorized:
		return 16 // Unauthenticated
	case http.StatusNotFound:
		return 5 // NotFound
	case http.StatusRequestEntityTooLarge:
		return 8 // ResourceExhausted
	case http.StatusServiceUnavailable:
		return 14 // Unavailable
	case http.StatusInternalServerError:
		return 13 // Internal
	default:
		return 2 // Unknown
	}
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
