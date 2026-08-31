package target

import (
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestMappingExtractsOneOutputAndOrderedContext(t *testing.T) {
	mapping, err := Compile(domain.ResponseMapping{
		Output: "$.answer", Context: "$.sources[*].text",
	})
	if err != nil {
		t.Fatalf("compile mapping: %v", err)
	}
	generation, err := mapping.Extract(map[string]any{
		"answer": "Assay evaluates AI.",
		"sources": []any{
			map[string]any{"text": "first"},
			map[string]any{"text": "second"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("extract mapping: %v", err)
	}
	if generation.Output != "Assay evaluates AI." || len(generation.Context) != 2 ||
		generation.Context[0] != (domain.Chunk{ID: "k0", Text: "first"}) ||
		generation.Context[1] != (domain.Chunk{ID: "k1", Text: "second"}) {
		t.Fatalf("generation = %#v", generation)
	}
}

func TestMappingUsesFallbackAndRejectsAmbiguousOutput(t *testing.T) {
	fallback := []domain.Chunk{{ID: "existing", Text: "context"}}
	mapping, err := Compile(domain.ResponseMapping{Output: "$.answers[*]"})
	if err != nil {
		t.Fatalf("compile mapping: %v", err)
	}
	if _, err := mapping.Extract(map[string]any{
		"answers": []any{"one", "two"},
	}, fallback); err == nil {
		t.Fatal("ambiguous output mapping succeeded")
	}
	generation, err := mapping.Extract(map[string]any{
		"answers": []any{"one"},
	}, fallback)
	if err != nil {
		t.Fatalf("extract fallback context: %v", err)
	}
	if len(generation.Context) != 1 || generation.Context[0] != fallback[0] {
		t.Fatalf("fallback context = %#v", generation.Context)
	}
}
