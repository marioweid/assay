package target

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
)

//nolint:cyclop // The handler assertions describe one request contract.
func TestClientGeneratesFromRenderedJSONRequest(t *testing.T) {
	handler := func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer token" ||
			request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("target request method/headers = %s/%v", request.Method, request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode target request: %v", err)
		}
		if body["query"] != "What is Assay?" || body["top_k"] != float64(5) {
			t.Errorf("target request body = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"answer":"Assay evaluates AI.","sources":[{"text":"context"}]}`))
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	mapping, err := Compile(domain.ResponseMapping{
		Output: "$.answer", Context: "$.sources[*].text",
	})
	if err != nil {
		t.Fatalf("compile mapping: %v", err)
	}
	endpoint := domain.ResolvedTargetEndpoint{
		URL: server.URL, Method: http.MethodPost,
		Headers: map[string]string{"Authorization": "Bearer {{ .secret }}"},
		RequestTemplate: map[string]any{
			"query": "{{ .item.input.question }}", "top_k": 5,
		},
		Timeout: time.Second, Secret: "token",
	}

	generation, err := NewClient(http.DefaultTransport).Generate(
		t.Context(), endpoint, mapping,
		domain.DatasetItem{Input: map[string]any{"question": "What is Assay?"}},
	)
	if err != nil {
		t.Fatalf("generate target output: %v", err)
	}
	if generation.Output != "Assay evaluates AI." || len(generation.Context) != 1 {
		t.Fatalf("generation = %#v", generation)
	}
}

func TestClientClassifiesStatusesAndRedactsResponse(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, retryable: true},
		{name: "server failure", status: http.StatusBadGateway, retryable: true},
		{name: "bad request", status: http.StatusBadRequest, retryable: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				http.Error(writer, "private-response", test.status)
			}))
			t.Cleanup(server.Close)
			mapping, err := Compile(domain.ResponseMapping{Output: "$.answer"})
			if err != nil {
				t.Fatalf("compile mapping: %v", err)
			}
			_, err = NewClient(http.DefaultTransport).Generate(t.Context(), domain.ResolvedTargetEndpoint{
				URL: server.URL, Method: http.MethodPost, Timeout: time.Second,
				RequestTemplate: map[string]any{}, Secret: "private-secret",
			}, mapping, domain.DatasetItem{})
			if err == nil || IsRetryable(err) != test.retryable {
				t.Fatalf("target error/retryable = %v/%v", err, IsRetryable(err))
			}
			if strings.Contains(err.Error(), "private-response") ||
				strings.Contains(err.Error(), "private-secret") {
				t.Fatalf("target error leaks content: %v", err)
			}
		})
	}
}

func TestDecodeResponseRejectsTrailingJSON(t *testing.T) {
	_, err := decodeResponse(strings.NewReader(`{"answer":"ok"} trailing`), 1024)
	if err == nil {
		t.Fatal("decode response accepted trailing JSON")
	}
}
