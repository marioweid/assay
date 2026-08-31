package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/api"
	"github.com/marioweid/assay/assayd/internal/auth"
	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/httpserver"
	"github.com/marioweid/assay/assayd/internal/migrate"
	"github.com/marioweid/assay/assayd/internal/store"
	"github.com/marioweid/assay/assayd/internal/testutil"

	"github.com/google/uuid"
)

const adminToken = "admin-secret"

func TestManagementRoutesRequireAdminToken(t *testing.T) {
	fixture := newAPIFixture(t)
	id := uuid.Must(uuid.NewV7()).String()
	routes := []requestSpec{
		{method: http.MethodPost, path: "/v1/projects", body: `{"name":"Project"}`},
		{method: http.MethodGet, path: "/v1/projects"},
		{method: http.MethodGet, path: "/v1/projects/" + id},
		{method: http.MethodPatch, path: "/v1/projects/" + id, body: `{"name":"Renamed"}`},
		{method: http.MethodDelete, path: "/v1/projects/" + id},
		{method: http.MethodPost, path: "/v1/projects/" + id + "/keys", body: `{"name":"CI"}`},
		{method: http.MethodGet, path: "/v1/projects/" + id + "/keys"},
		{method: http.MethodDelete, path: "/v1/projects/" + id + "/keys/" + id},
		{method: http.MethodPost, path: "/v1/applications", body: `{"name":"App"}`},
		{method: http.MethodGet, path: "/v1/applications"},
		{method: http.MethodGet, path: "/v1/applications/" + id},
		{method: http.MethodPatch, path: "/v1/applications/" + id, body: `{"name":"App"}`},
		{method: http.MethodDelete, path: "/v1/applications/" + id},
		{
			method: http.MethodPatch,
			path:   "/v1/applications/" + id + "/endpoint",
			body:   `{"clear":true}`,
		},
		{method: http.MethodPost, path: "/v1/datasets", body: `{}`},
		{method: http.MethodGet, path: "/v1/datasets"},
		{method: http.MethodGet, path: "/v1/datasets/" + id},
		{method: http.MethodDelete, path: "/v1/datasets/" + id},
		{method: http.MethodPost, path: "/v1/datasets/" + id + "/items", body: `{}`},
		{method: http.MethodGet, path: "/v1/datasets/" + id + "/items"},
		{method: http.MethodGet, path: "/v1/applications/" + id + "/scorers"},
		{
			method: http.MethodPut,
			path:   "/v1/applications/" + id + "/scorers/groundedness",
			body:   `{}`,
		},
		{method: http.MethodPost, path: "/v1/runs", body: `{}`},
		{method: http.MethodGet, path: "/v1/runs"},
		{method: http.MethodGet, path: "/v1/runs/" + id},
		{method: http.MethodGet, path: "/v1/runs/" + id + "/items"},
		{method: http.MethodGet, path: "/v1/runs/" + id + "/scores"},
		{method: http.MethodPost, path: "/v1/runs/" + id + "/cancel"},
	}
	for _, route := range routes {
		assertUnauthorized(t, fixture.handler, route)
	}
}

func TestHealthAndDocumentationRoutesArePublic(t *testing.T) {
	fixture := newAPIFixture(t)
	for _, path := range []string{"/healthz", "/readyz", "/openapi.json", "/docs"} {
		response := fixture.perform(requestSpec{method: http.MethodGet, path: path})
		if response.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, response.Code)
		}
	}
}

func TestOpenAPIDocumentsManagementSecurity(t *testing.T) {
	handler := newDocumentationHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var document openAPIDocument
	decodeResponse(t, response, &document)
	assertOpenAPISchemes(t, document)
	assertManagementOpenAPISecurity(t, document)
	assertTraceOpenAPISecurity(t, document)
}

func assertOpenAPISchemes(t *testing.T, document openAPIDocument) {
	t.Helper()
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("OpenAPI version = %q, want 3.1.0", document.OpenAPI)
	}
	if document.Components.SecuritySchemes["adminBearer"].Scheme != "bearer" {
		t.Fatal("OpenAPI is missing adminBearer security scheme")
	}
	if document.Components.SecuritySchemes["projectBearer"].Scheme != "bearer" {
		t.Fatal("OpenAPI is missing projectBearer security scheme")
	}
	if document.Components.SecuritySchemes["projectAPIKey"].Type != "apiKey" {
		t.Fatal("OpenAPI is missing projectAPIKey security scheme")
	}
}

func assertManagementOpenAPISecurity(t *testing.T, document openAPIDocument) {
	t.Helper()
	for _, path := range managementPaths {
		operations, found := document.Paths[path]
		if !found {
			t.Errorf("OpenAPI is missing path %s", path)
			continue
		}
		for method, operation := range operations {
			if !hasAdminSecurity(operation.Security) {
				t.Errorf("OpenAPI %s %s is missing adminBearer security", method, path)
			}
		}
	}
}

func assertTraceOpenAPISecurity(t *testing.T, document openAPIDocument) {
	t.Helper()
	for _, path := range []string{"/v1/traces", "/v1/traces/{id}"} {
		operations, found := document.Paths[path]
		if !found {
			t.Errorf("OpenAPI is missing path %s", path)
			continue
		}
		operation, found := operations["get"]
		if !found || !hasProjectSecurity(operation.Security) {
			t.Errorf("OpenAPI GET %s is missing project security", path)
		}
	}
}

func TestDomainAPIFlowRedactsSecretsAndReturnsKeyOnce(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("judge-secret")
	fixture.updateAndListProject(project.ID)
	key := fixture.createAndListKey(project.ID)
	projectID, err := fixture.service.AuthenticateAPIKey(t.Context(), key.Key)
	if err != nil || projectID.String() != project.ID {
		t.Fatalf("authenticate created key = %s, %v; want %s", projectID, err, project.ID)
	}
	application := fixture.createApplication(project.ID)
	fixture.updateListAndConfigureApplication(application.ID, project.ID)
	fixture.deleteKeyAndProject(key.ID, project.ID, key.Key)
}

type apiFixture struct {
	t       *testing.T
	handler http.Handler
	service *domain.Service
}

type requestSpec struct {
	method string
	path   string
	body   string
	token  string
}

type projectResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	JudgeConfig *struct {
		HasAPIKey bool `json:"has_api_key"`
	} `json:"judge_config"`
}

type keyResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	KeyPrefix string `json:"key_prefix"`
	Key       string `json:"key"`
}

type applicationResponse struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	TargetEndpoint *struct {
		HasSecret bool `json:"has_secret"`
	} `json:"target_endpoint"`
}

type openAPIDocument struct {
	OpenAPI    string                              `json:"openapi"`
	Paths      map[string]map[string]openOperation `json:"paths"`
	Components struct {
		SecuritySchemes map[string]struct {
			Type   string `json:"type"`
			Scheme string `json:"scheme"`
		} `json:"securitySchemes"`
	} `json:"components"`
}

func TestTraceReadRoutesRequireProjectKey(t *testing.T) {
	handler := newDocumentationHandler(t)
	for _, path := range []string{"/v1/traces", "/v1/traces/" + uuid.Must(uuid.NewV7()).String()} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s status = %d, want 401", path, response.Code)
		}
	}
}

func hasProjectSecurity(security []map[string][]string) bool {
	hasBearer := false
	hasAPIKey := false
	for _, requirement := range security {
		if _, found := requirement["projectBearer"]; found {
			hasBearer = true
		}
		if _, found := requirement["projectAPIKey"]; found {
			hasAPIKey = true
		}
	}
	return hasBearer && hasAPIKey
}

type openOperation struct {
	Security []map[string][]string `json:"security"`
}

var managementPaths = []string{
	"/v1/projects",
	"/v1/projects/{id}",
	"/v1/projects/{id}/keys",
	"/v1/projects/{id}/keys/{keyId}",
	"/v1/applications",
	"/v1/applications/{id}",
	"/v1/applications/{id}/endpoint",
	"/v1/applications/{application_id}/scorers",
	"/v1/applications/{application_id}/scorers/{scorer}",
	"/v1/datasets",
	"/v1/datasets/{id}",
	"/v1/datasets/{id}/items",
	"/v1/runs",
	"/v1/runs/{id}",
	"/v1/runs/{id}/items",
	"/v1/runs/{id}/scores",
	"/v1/runs/{id}/cancel",
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	database, err := store.Open(t.Context(), testutil.Postgres(t))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Up(t.Context(), database.MigrationDB(), logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	cipher, err := secretcrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create secret cipher: %v", err)
	}
	service := domain.NewService(database, cipher)
	traceService := domain.NewTraceService(database, service, 3)
	evaluations := domain.NewEvaluationService(database, cipher, 3)
	mux := httpserver.NewMux(database, logger)
	api.Register(mux, api.Dependencies{
		Service: service, Traces: traceService, Evaluations: evaluations,
		AdminToken: adminToken, Logger: logger,
	})
	return &apiFixture{t: t, handler: mux, service: service}
}

func newDocumentationHandler(t *testing.T) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := domain.NewService(nil, nil)
	traceService := domain.NewTraceService(nil, service, 3)
	evaluations := domain.NewEvaluationService(nil, nil, 3)
	mux := http.NewServeMux()
	api.Register(mux, api.Dependencies{
		Service: service, Traces: traceService, Evaluations: evaluations,
		AdminToken: adminToken, Logger: logger,
	})
	return mux
}

func (f *apiFixture) perform(spec requestSpec) *httptest.ResponseRecorder {
	f.t.Helper()
	request := httptest.NewRequest(spec.method, spec.path, strings.NewReader(spec.body))
	if spec.body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if spec.token != "" {
		request.Header.Set("Authorization", "Bearer "+spec.token)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func (f *apiFixture) createProject(secret string) projectResponse {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPost,
		path:   "/v1/projects",
		token:  adminToken,
		body: `{"name":"Support","judge_config":{"base_url":"https://judge.example.com/v1",` +
			`"model":"judge-model","api_key":"` + secret + `"}}`,
	})
	assertStatus(f.t, response, http.StatusCreated)
	assertRedacted(f.t, response.Body.String(), secret)
	var project projectResponse
	decodeResponse(f.t, response, &project)
	if project.ID == "" || project.JudgeConfig == nil || !project.JudgeConfig.HasAPIKey {
		f.t.Fatalf("created project = %#v", project)
	}
	return project
}

func (f *apiFixture) updateAndListProject(projectID string) {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPatch,
		path:   "/v1/projects/" + projectID,
		token:  adminToken,
		body:   `{"name":"Support Production"}`,
	})
	assertStatus(f.t, response, http.StatusOK)
	response = f.perform(requestSpec{method: http.MethodGet, path: "/v1/projects", token: adminToken})
	assertStatus(f.t, response, http.StatusOK)
	if !strings.Contains(response.Body.String(), "Support Production") {
		f.t.Fatalf("project list = %s", response.Body.String())
	}
}

func (f *apiFixture) createAndListKey(projectID string) keyResponse {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPost,
		path:   "/v1/projects/" + projectID + "/keys",
		token:  adminToken,
		body:   `{"name":"CI"}`,
	})
	assertStatus(f.t, response, http.StatusCreated)
	var key keyResponse
	decodeResponse(f.t, response, &key)
	if !auth.ValidAPIKeyFormat(key.Key) || key.ProjectID != projectID {
		f.t.Fatalf("created key = %#v", key)
	}
	response = f.perform(requestSpec{
		method: http.MethodGet,
		path:   "/v1/projects/" + projectID + "/keys",
		token:  adminToken,
	})
	assertStatus(f.t, response, http.StatusOK)
	assertRedacted(f.t, response.Body.String(), key.Key)
	return key
}

func (f *apiFixture) createApplication(projectID string) applicationResponse {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPost,
		path:   "/v1/applications",
		token:  adminToken,
		body: `{"project_id":"` + projectID + `","name":"Support Bot",` +
			`"slug":"support-bot","config":{"environment":"test"}}`,
	})
	assertStatus(f.t, response, http.StatusCreated)
	var application applicationResponse
	decodeResponse(f.t, response, &application)
	if application.ID == "" || application.ProjectID != projectID {
		f.t.Fatalf("created application = %#v", application)
	}
	return application
}

func (f *apiFixture) updateListAndConfigureApplication(applicationID string, projectID string) {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPatch,
		path:   "/v1/applications/" + applicationID,
		token:  adminToken,
		body:   `{"name":"Support Bot Production"}`,
	})
	assertStatus(f.t, response, http.StatusOK)
	response = f.perform(requestSpec{
		method: http.MethodGet,
		path:   "/v1/applications?project_id=" + projectID,
		token:  adminToken,
	})
	assertStatus(f.t, response, http.StatusOK)
	f.setAndClearEndpoint(applicationID, "endpoint-secret")
}

func (f *apiFixture) setAndClearEndpoint(applicationID string, secret string) {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodPatch,
		path:   "/v1/applications/" + applicationID + "/endpoint",
		token:  adminToken,
		body: `{"endpoint":{"url":"https://target.example.com/answer",` +
			`"response_mapping":{"output":"$.answer"},"secret":"` + secret + `"}}`,
	})
	assertStatus(f.t, response, http.StatusOK)
	assertRedacted(f.t, response.Body.String(), secret)
	var application applicationResponse
	decodeResponse(f.t, response, &application)
	if application.TargetEndpoint == nil || !application.TargetEndpoint.HasSecret {
		f.t.Fatalf("application endpoint = %#v", application.TargetEndpoint)
	}
	response = f.perform(requestSpec{
		method: http.MethodPatch,
		path:   "/v1/applications/" + applicationID + "/endpoint",
		token:  adminToken,
		body:   `{"clear":true}`,
	})
	assertStatus(f.t, response, http.StatusOK)
	var cleared applicationResponse
	decodeResponse(f.t, response, &cleared)
	if cleared.TargetEndpoint != nil {
		f.t.Fatalf("cleared endpoint = %#v, want nil", cleared.TargetEndpoint)
	}
}

func (f *apiFixture) deleteKeyAndProject(keyID string, projectID string, key string) {
	f.t.Helper()
	response := f.perform(requestSpec{
		method: http.MethodDelete,
		path:   "/v1/projects/" + projectID + "/keys/" + keyID,
		token:  adminToken,
	})
	assertStatus(f.t, response, http.StatusNoContent)
	if _, err := f.service.AuthenticateAPIKey(f.t.Context(), key); err == nil {
		f.t.Fatal("revoked API key still authenticates")
	}
	response = f.perform(requestSpec{
		method: http.MethodDelete,
		path:   "/v1/projects/" + projectID,
		token:  adminToken,
	})
	assertStatus(f.t, response, http.StatusNoContent)
}

func assertUnauthorized(t *testing.T, handler http.Handler, spec requestSpec) {
	t.Helper()
	for _, token := range []string{"", "wrong-token"} {
		spec.token = token
		fixture := apiFixture{t: t, handler: handler}
		response := fixture.perform(spec)
		assertStatus(t, response, http.StatusUnauthorized)
	}
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, want, response.Body.String())
	}
}

func assertRedacted(t *testing.T, body string, secret string) {
	t.Helper()
	if strings.Contains(body, secret) {
		t.Fatalf("response contains secret %q: %s", secret, body)
	}
	if strings.Contains(body, "ciphertext") || strings.Contains(body, "key_hash") {
		t.Fatalf("response exposes stored credential material: %s", body)
	}
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, output any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), output); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}

func hasAdminSecurity(requirements []map[string][]string) bool {
	for _, requirement := range requirements {
		if _, found := requirement["adminBearer"]; found {
			return true
		}
	}
	return false
}
