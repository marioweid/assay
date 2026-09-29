package httpserver

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// LocalModeGuard mitigates browser cross-site requests and DNS rebinding in local mode.
// It is not authentication: callers must restrict network access to the trusted host/network.
func LocalModeGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !localHost(request.Host) {
			http.Error(writer, "Local mode requires localhost, a loopback IP, or assayd", http.StatusForbidden)
			return
		}
		// Public assets remain navigable from documentation links; API reads also need protection.
		if strings.HasPrefix(request.URL.Path, "/v1/") && !localBrowserRequest(request) {
			http.Error(writer, "Local mode requires a same-origin API request", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func localHost(host string) bool {
	parsed, err := url.Parse("http://" + host)
	if err != nil || parsed.Host != host || parsed.User != nil || parsed.Path != "" {
		return false
	}
	name := strings.ToLower(parsed.Hostname())
	if name == "localhost" || name == "assayd" {
		return true
	}
	address := net.ParseIP(name)
	return address != nil && address.IsLoopback()
}

func localBrowserRequest(request *http.Request) bool {
	switch request.Header.Get("Sec-Fetch-Site") {
	case "", "none", "same-origin":
	default:
		return false
	}
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+request.Host
}
