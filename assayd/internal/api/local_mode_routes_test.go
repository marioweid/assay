package api_test

import (
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/api"
	"github.com/marioweid/assay/assayd/internal/auth"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/httpserver"
	"github.com/marioweid/assay/assayd/internal/otlp"

	"github.com/google/uuid"
)

type localRepository struct {
	domain.Repository
	domain.TraceRepository
	domain.SessionRepository
	owner, keyProject, readProject uuid.UUID
	keyHash                        [sha256.Size]byte
}

func (r *localRepository) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{}, nil
}
func (r *localRepository) CreateProject(_ context.Context, p domain.Project) (domain.Project, error) {
	return p, nil
}
func (r *localRepository) GetApplication(context.Context, uuid.UUID) (domain.Application, error) {
	return domain.Application{ProjectID: r.owner}, nil
}
func (r *localRepository) UseActiveAPIKeyByHash(_ context.Context, hash [sha256.Size]byte) (domain.APIKey, error) {
	if hash != r.keyHash {
		return domain.APIKey{}, domain.ErrNotFound
	}
	return domain.APIKey{ProjectID: r.keyProject}, nil
}
func (r *localRepository) ListTraces(_ context.Context, project uuid.UUID, _ domain.TraceQuery) ([]domain.Trace, error) {
	r.readProject = project
	return []domain.Trace{}, nil
}
func (r *localRepository) ListSessions(_ context.Context, project uuid.UUID, _ domain.SessionQuery) ([]domain.Session, error) {
	r.readProject = project
	return []domain.Session{}, nil
}

func TestLocalRoutesKeepExplicitProjectKeysScopedAndIngestionProtected(t *testing.T) {
	key, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	r := &localRepository{owner: uuid.New(), keyProject: uuid.New(), keyHash: auth.HashAPIKey(key)}
	service := domain.NewService(r, nil)
	traces := domain.NewTraceService(r, service, 3)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	api.Register(mux, api.Dependencies{Service: service, Traces: traces,
		Sessions: domain.NewSessionService(r), LocalMode: true, AdminToken: "admin", Logger: logger})
	otlp.Register(mux, service, traces, false, logger)
	handler := httpserver.LocalModeGuard(mux)
	for _, route := range []string{"/v1/traces", "/v1/sessions"} {
		for _, header := range []string{"", "x-api-key", "Authorization"} {
			request := httptest.NewRequest(http.MethodGet,
				"http://localhost:8080"+route+"?application_id="+uuid.NewString(), nil)
			want := r.owner
			if header != "" {
				value := key
				if header == "Authorization" {
					value = "Bearer " + key
				}
				request.Header.Set(header, value)
				want = r.keyProject
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || r.readProject != want {
				t.Fatalf("%s header=%s: status %d project %s, want %s", route, header, response.Code, r.readProject, want)
			}
		}
	}
	for _, token := range []string{"", "Bearer admin", "Bearer invalid"} {
		request := httptest.NewRequest(http.MethodPost, "http://localhost:8080/v1/traces", strings.NewReader("{}"))
		request.Header.Set("Authorization", token)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("ingest without project key: %d", response.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "http://localhost:8080/v1/projects", strings.NewReader(`{"name":"Local project"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code < 200 || response.Code >= 300 {
			t.Fatalf("anonymous management %s: %d", method, response.Code)
		}
	}
}
