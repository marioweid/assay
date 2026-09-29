package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

func TestLocalAdministratorNeverElevatesExplicitProjectCredentials(t *testing.T) {
	for _, local := range []bool{false, true} {
		h := &handler{localMode: local, adminToken: "admin"}
		for _, test := range []struct {
			authorization, key string
			want               bool
		}{
			{"", "", local},
			{"Bearer admin", "", true},
			{"Bearer project", "", false},
			{"", "project", false},
			{"Bearer wrong", "project", false},
			{"Basic admin", "", false},
		} {
			if got := h.isAdmin(test.authorization, test.key); got != test.want {
				t.Errorf("local=%v authorization=%q key=%q: %v, want %v",
					local, test.authorization, test.key, got, test.want)
			}
		}
	}
}

func TestLocalAdminMiddlewareAndPublicDiscovery(t *testing.T) {
	for _, local := range []bool{false, true} {
		mux := http.NewServeMux()
		h := &handler{
			localMode: local, adminToken: "admin",
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		h.api = humago.New(mux, huma.DefaultConfig("test", "1.0"))
		h.registerServerInfoRoute()
		assertLocalAdminRoutes(t, mux, h, local)
		assertLocalServerInfo(t, mux, local)
	}
}

func assertLocalAdminRoutes(t *testing.T, mux *http.ServeMux, h *handler, local bool) {
	t.Helper()
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		huma.Register(h.api, h.operation(method, "/admin", method, "test"),
			func(_ context.Context, _ *struct{}) (*struct{}, error) { return nil, nil })
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(method, "/admin", nil))
		want := http.StatusUnauthorized
		if local {
			want = http.StatusNoContent
		}
		if response.Code != want {
			t.Errorf("local=%v %s: %d, want %d", local, method, response.Code, want)
		}
	}
}

func assertLocalServerInfo(t *testing.T, mux *http.ServeMux, local bool) {
	t.Helper()
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/server-info", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("server info: status=%d headers=%v", response.Code, response.Header())
	}
	var body struct {
		LocalMode *bool `json:"local_mode"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.LocalMode == nil || *body.LocalMode != local {
		t.Fatalf("server info must disclose configured local_mode=%v: %s", local, response.Body)
	}
}
