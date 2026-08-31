package target

import (
	"strings"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
)

//nolint:cyclop // Assertions cover independent rendered leaf types in one result.
func TestRenderPreservesJSONTypesAndEscapesDatasetText(t *testing.T) {
	endpoint := domain.ResolvedTargetEndpoint{
		Headers: map[string]string{"Authorization": "Bearer {{ .secret }}"},
		RequestTemplate: map[string]any{
			"query":  "{{ .item.input.question }}",
			"top_k":  int64(5),
			"nested": []any{true, nil, "{{ .item.metadata.label }}"},
		},
		Secret:  "token",
		Timeout: time.Second,
	}
	item := domain.DatasetItem{
		Input: map[string]any{"question": `quote " and newline
kept`},
		Metadata: map[string]any{"label": "support"},
	}

	headers, body, err := Render(endpoint, item)
	if err != nil {
		t.Fatalf("render target: %v", err)
	}
	if headers["Authorization"] != "Bearer token" || body["top_k"] != int64(5) {
		t.Fatalf("rendered headers/body = %#v/%#v", headers, body)
	}
	if body["query"] != item.Input["question"] {
		t.Fatalf("rendered query = %#v", body["query"])
	}
	nested, ok := body["nested"].([]any)
	if !ok || nested[0] != true || nested[1] != nil || nested[2] != "support" {
		t.Fatalf("rendered nested value = %#v", body["nested"])
	}
}

func TestRenderRejectsInvalidTemplateWithoutLeakingData(t *testing.T) {
	endpoint := domain.ResolvedTargetEndpoint{
		RequestTemplate: map[string]any{"query": "{{ .item.input.question"},
		Secret:          "do-not-leak",
	}
	_, _, err := Render(endpoint, domain.DatasetItem{
		Input: map[string]any{"question": "private-question"},
	})
	if err == nil {
		t.Fatal("invalid target template rendered")
	}
	if containsSensitive(err.Error()) {
		t.Fatalf("target error leaks content: %v", err)
	}
}

func containsSensitive(message string) bool {
	return strings.Contains(message, "do-not-leak") || strings.Contains(message, "private-question")
}
