package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionRoutesRequireAuthAndDeclareScopeParameters(t *testing.T) {
	handler := newDocumentationHandler(t)
	for _, path := range []string{"/v1/sessions?application_id=00000000-0000-0000-0000-000000000001",
		"/v1/session-turns?application_id=00000000-0000-0000-0000-000000000001&session_id=..",
		"/v1/session-turns/recent?application_id=00000000-0000-0000-0000-000000000001&session_id=.."} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("GET %s status = %d, want 401", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var document openAPIDocument
	decodeResponse(t, response, &document)
	for path, names := range map[string][]string{
		"/v1/sessions":      {"application_id"},
		"/v1/session-turns": {"application_id", "session_id"},
		"/v1/session-turns/recent": {"application_id", "session_id"},
	} {
		required := make(map[string]bool)
		for _, parameter := range document.Paths[path]["get"].Parameters {
			required[parameter.Name] = parameter.Required
		}
		for _, name := range names {
			if !required[name] {
				t.Errorf("GET %s missing required %s parameter", path, name)
			}
		}
	}
}

func TestSessionReadRoutesBindOpaqueQueryAndRequireProjectScope(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("judge-one")
	application := fixture.createApplication(project.ID)
	other := fixture.createProjectNamed("Other", "judge-two")
	foreignKey := fixture.createAndListKey(other.ID)
	key := fixture.createAndListKey(project.ID)
	base := "/v1/session-turns?application_id=" + application.ID + "&session_id=a%252Fb"

	for _, test := range []struct {
		path   string
		token  string
		status int
	}{
		{"/v1/sessions?application_id=" + application.ID, key.Key, http.StatusOK},
		{"/v1/sessions?application_id=" + application.ID, foreignKey.Key, http.StatusOK},
		{base, key.Key, http.StatusNotFound},
		{base, foreignKey.Key, http.StatusNotFound},
		{"/v1/session-turns/recent?application_id=" + application.ID + "&session_id=a%252Fb",
			foreignKey.Key, http.StatusNotFound},
		{"/v1/session-turns?application_id=" + application.ID, key.Key, http.StatusUnprocessableEntity},
	} {
		response := fixture.perform(requestSpec{method: http.MethodGet, path: test.path, token: test.token})
		if response.Code != test.status {
			t.Errorf("GET %s status = %d, want %d: %s", test.path, response.Code, test.status,
				response.Body.String())
		}
		if strings.Contains(response.Body.String(), "judge-one") {
			t.Errorf("GET %s exposed unrelated project data", test.path)
		}
	}
}
