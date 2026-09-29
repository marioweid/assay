package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalModeGuard(t *testing.T) {
	tests := []struct {
		name, host, origin, site string
		allowed                  bool
	}{
		{"localhost", "localhost:8080", "", "", true},
		{"loopback", "127.0.0.1:8080", "http://127.0.0.1:8080", "same-origin", true},
		{"ipv6", "[::1]:8080", "http://[::1]:8080", "same-origin", true},
		{"compose SDK", "assayd:8080", "", "", true},
		{"rebound host", "attacker.test:8080", "", "same-origin", false},
		{"private IP", "192.168.1.2:8080", "", "", false},
		{"cross origin", "localhost:8080", "https://attacker.test", "", false},
		{"different port", "localhost:8080", "http://localhost:9999", "", false},
		{"different scheme", "localhost:8080", "https://localhost:8080", "", false},
		{"null origin", "localhost:8080", "null", "", false},
		{"origin with path", "localhost:8080", "http://localhost:8080/evil", "", false},
		{"cross site without origin", "localhost:8080", "", "cross-site", false},
		{"same site not same origin", "localhost:8080", "", "same-site", false},
		{"malformed host", "localhost:bad", "", "", false},
		{"userinfo host", "attacker@localhost", "", "", false},
		{"host path", "localhost/evil", "", "", false},
	}
	for _, test := range tests {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			t.Run(test.name+method, func(t *testing.T) {
				handler := LocalModeGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}))
				request := httptest.NewRequest(method, "/v1/projects", nil)
				request.Host = test.host
				request.Header.Set("Origin", test.origin)
				request.Header.Set("Sec-Fetch-Site", test.site)
				request.Header.Set("X-Forwarded-Host", "localhost:8080")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				want := http.StatusForbidden
				if test.allowed {
					want = http.StatusNoContent
				}
				if response.Code != want {
					t.Fatalf("status = %d, want %d", response.Code, want)
				}
			})
		}
	}
}

func TestLocalModeAllowsNavigationButNotCrossSiteAPIRead(t *testing.T) {
	handler := LocalModeGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatal("public UI navigation must remain usable")
	}
}
