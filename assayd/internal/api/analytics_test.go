package api_test

import (
	"net/http"
	"testing"
)

func TestAnalyticsRoutesRequireAdmin(t *testing.T) {
	handler := newDocumentationHandler(t)
	for _, path := range []string{
		"/v1/applications/00000000-0000-0000-0000-000000000001/metrics",
		"/v1/scores?application_id=00000000-0000-0000-0000-000000000001",
	} {
		assertUnauthorized(t, handler, requestSpec{method: http.MethodGet, path: path})
	}
}

func TestAnalyticsValidatesFiltersAndReturnsEmptyCollections(t *testing.T) {
	fixture := newAPIFixture(t)
	project := fixture.createProject("judge-secret")
	application := fixture.createApplication(project.ID)
	metrics := "/v1/applications/" + application.ID + "/metrics"
	scores := "/v1/scores?application_id=" + application.ID
	tests := []struct {
		path   string
		status int
	}{
		{metrics, http.StatusOK},
		{scores, http.StatusOK},
		{scores + "&passed=false", http.StatusOK},
		{scores + "&passed=wrong", http.StatusUnprocessableEntity},
		{scores + "&scorer=unknown", http.StatusUnprocessableEntity},
		{scores + "&limit=501", http.StatusUnprocessableEntity},
		{scores + "&cursor=wrong", http.StatusBadRequest},
		{metrics + "?start=2026-09-02T00:00:00Z&end=2026-09-01T00:00:00Z",
			http.StatusUnprocessableEntity},
		{metrics + "?start=2020-01-01T00:00:00Z&end=2026-09-01T00:00:00Z",
			http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := fixture.perform(requestSpec{
				method: http.MethodGet, path: test.path, token: adminToken,
			})
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body)
			}
		})
	}
}
