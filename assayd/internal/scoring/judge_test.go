package scoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestHTTPJudgeUsesOpenAIJSONContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(judgeContractHandler(t)))
	defer server.Close()

	judge := NewHTTPJudge(server.Client(), domain.ResolvedJudgeConfig{
		BaseURL: server.URL + "/v1", APIKey: "secret", Model: "judge-model",
	})
	response, err := judge.Complete(t.Context(), JudgeRequest{
		System: "system", User: map[string]any{"q": "x"},
	})
	if err != nil {
		t.Fatalf("complete judge request: %v", err)
	}
	if response.Content != `{"ok":true}` || response.Tokens != 9 {
		t.Fatalf("judge response = %#v", response)
	}
}

func judgeContractHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		assertJudgeRequest(t, request)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}],` +
			`"usage":{"total_tokens":9}}`))
	}
}

func assertJudgeRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.URL.Path != "/v1/chat/completions" {
		t.Errorf("path = %q", request.URL.Path)
	}
	if request.Header.Get("Authorization") != "Bearer secret" {
		t.Errorf("authorization = %q", request.Header.Get("Authorization"))
	}
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Errorf("decode judge body: %v", err)
	}
	if body["model"] != "judge-model" || body["temperature"] != float64(0) {
		t.Errorf("judge body = %#v", body)
	}
}

func TestHTTPJudgeClassifiesRetryableStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "secret response body", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	judge := NewHTTPJudge(server.Client(), domain.ResolvedJudgeConfig{BaseURL: server.URL, Model: "m"})
	_, err := judge.Complete(t.Context(), JudgeRequest{System: "secret prompt"})
	if !IsRetryable(err) {
		t.Fatalf("error = %v, want retryable", err)
	}
	if err == nil || containsSensitive(err.Error()) {
		t.Fatalf("unsafe judge error = %v", err)
	}
}

func TestHTTPJudgeRetriesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "timeout", http.StatusRequestTimeout)
	}))
	defer server.Close()
	judge := NewHTTPJudge(server.Client(), domain.ResolvedJudgeConfig{BaseURL: server.URL, Model: "m"})
	_, err := judge.Complete(t.Context(), JudgeRequest{System: "prompt"})
	if !IsRetryable(err) {
		t.Fatalf("408 error = %v, want retryable", err)
	}
}

func containsSensitive(message string) bool {
	return message == "secret response body" || message == "secret prompt"
}
