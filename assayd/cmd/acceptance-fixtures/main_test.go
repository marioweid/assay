package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFixtureEndpoints(t *testing.T) {
	t.Parallel()
	handler := newFixtureHandler()

	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "health", path: "/healthz", wantStatus: http.StatusOK, wantBody: "ok"},
		{
			name: "extract passing claim", path: "/v1/chat/completions",
			body: judgeRequest(map[string]any{
				"question": fixtureQuestion, "answer": fixturePassingAnswer,
			}),
			wantStatus: http.StatusOK, wantBody: `\"claims\"`,
		},
		{
			name: "verify unsupported claim", path: "/v1/chat/completions",
			body: judgeRequest(map[string]any{
				"claims":         []map[string]string{{"id": "c1", "text": fixtureFailingAnswer}},
				"context_chunks": []map[string]string{{"id": "k0", "text": fixtureContext}},
			}),
			wantStatus: http.StatusOK, wantBody: `\"verdict\":\"unsupported\"`,
		},
		{
			name: "correct answer", path: "/v1/chat/completions",
			body: judgeRequest(map[string]any{
				"question": fixtureQuestion, "generated": fixturePassingAnswer,
				"reference": fixtureReference,
			}),
			wantStatus: http.StatusOK, wantBody: `\"score\":1`,
		},
		{
			name: "incorrect answer", path: "/v1/chat/completions",
			body: judgeRequest(map[string]any{
				"question": fixtureQuestion, "generated": fixtureFailingAnswer,
				"reference": fixtureReference,
			}),
			wantStatus: http.StatusOK, wantBody: `\"score\":0`,
		},
		{
			name: "target answer", path: "/answer",
			body:       `{"query":"` + fixtureQuestion + `"}`,
			wantStatus: http.StatusOK, wantBody: fixturePassingAnswer,
		},
		{
			name: "target failed quality", path: "/answer",
			body:       `{"query":"` + fixtureFailingQuestion + `"}`,
			wantStatus: http.StatusOK, wantBody: fixtureFailingAnswer,
		},
		{
			name: "unknown judge input", path: "/v1/chat/completions",
			body:       judgeRequest(map[string]any{"answer": "unknown"}),
			wantStatus: http.StatusUnprocessableEntity, wantBody: "unknown synthetic judge input",
		},
		{
			name: "terminal target failure", path: "/answer",
			body:       `{"query":"` + fixtureTerminalQuestion + `"}`,
			wantStatus: http.StatusUnprocessableEntity, wantBody: "synthetic terminal failure",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method := http.MethodPost
			if test.body == "" {
				method = http.MethodGet
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(method, test.path, strings.NewReader(test.body))
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("response = %d %q, want %d containing %q", response.Code,
					response.Body.String(), test.wantStatus, test.wantBody)
			}
		})
	}
}

func TestTransientFixtureFailsOncePerRequest(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		path string
		body string
	}{
		{name: "target", path: "/answer", body: `{"query":"` + fixtureTransientQuestion + `"}`},
		{name: "judge", path: "/v1/chat/completions", body: judgeRequest(map[string]any{
			"question": fixtureTransientQuestion, "answer": fixturePassingAnswer,
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newFixtureHandler()
			for attempt, want := range []int{http.StatusServiceUnavailable, http.StatusOK} {
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
				handler.ServeHTTP(response, request)
				if response.Code != want {
					t.Fatalf("attempt %d status = %d, want %d", attempt+1, response.Code, want)
				}
			}
		})
	}
}

func TestJudgeTerminalFailure(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	body := judgeRequest(map[string]any{
		"question": fixtureTerminalQuestion, "answer": fixturePassingAnswer,
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	newFixtureHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnprocessableEntity)
	}
}

func judgeRequest(input map[string]any) string {
	user, _ := json.Marshal(input)
	body, _ := json.Marshal(map[string]any{
		"model": "fixture",
		"messages": []map[string]string{
			{"role": "system", "content": "test"},
			{"role": "user", "content": string(user)},
		},
		"response_format": map[string]string{"type": "json_object"},
	})
	return bytes.NewBuffer(body).String()
}
