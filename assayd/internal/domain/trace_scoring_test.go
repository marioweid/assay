package domain_test

import (
	"errors"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestBuildTraceScoreInputExtractsOTelMessagesAndContext(t *testing.T) {
	trace := traceWithScorableSpan(map[string]any{
		"gen_ai.input.messages": []any{
			message("system", "instructions"), message("user", "old question"),
			map[string]any{"role": "user", "parts": []any{
				map[string]any{"type": "text", "content": "What is "},
				map[string]any{"type": "text", "content": "Assay?"},
			}},
		},
		"gen_ai.output.messages": `[{
			"role":"assistant","parts":[{"type":"text","content":"Assay evaluates AI."}]},{
			"role":"assistant","parts":[{"type":"text","content":"ignored"}]}]`,
		"assay.context.chunk.count":   2,
		"assay.context.chunks.0.id":   "k0",
		"assay.context.chunks.0.text": "first",
		"assay.context.chunks.1.id":   "k1",
		"assay.context.chunks.1.text": "second",
	})

	input, err := domain.BuildTraceScoreInput(trace, domain.ScorerGroundedness)
	if err != nil {
		t.Fatalf("build trace score input: %v", err)
	}
	if input.Input != "What is Assay?" || input.Output != "Assay evaluates AI." ||
		len(input.Context) != 2 || input.Context[1].ID != "k1" {
		t.Fatalf("trace score input = %#v", input)
	}
}

func TestBuildTraceScoreInputUsesRetrievalDocumentsAndReference(t *testing.T) {
	reference := "Expected answer"
	trace := traceWithScorableSpan(map[string]any{
		"gen_ai.input.messages":  `[{"role":"user","content":"question"}]`,
		"gen_ai.output.messages": []any{message("assistant", "answer")},
		"gen_ai.retrieval.documents": []any{
			"first", map[string]any{"id": "doc", "text": "second"},
		},
	})
	trace.ReferenceAnswer = &reference

	grounded, err := domain.BuildTraceScoreInput(trace, domain.ScorerGroundedness)
	if err != nil || len(grounded.Context) != 2 || grounded.Context[0].ID != "k0" ||
		grounded.Context[1].ID != "doc" {
		t.Fatalf("grounded input = %#v, %v", grounded, err)
	}
	correct, err := domain.BuildTraceScoreInput(trace, domain.ScorerCorrectness)
	if err != nil || correct.Reference != reference {
		t.Fatalf("correctness input = %#v, %v", correct, err)
	}
}

func TestBuildTraceScoreInputRejectsMalformedOrIncompleteTrace(t *testing.T) {
	valid := map[string]any{
		"gen_ai.input.messages":      []any{message("user", "question")},
		"gen_ai.output.messages":     []any{message("assistant", "answer")},
		"gen_ai.retrieval.documents": []any{"context"},
	}
	tests := []struct {
		name   string
		trace  domain.Trace
		scorer string
	}{
		{name: "no scorable span", trace: domain.Trace{}, scorer: domain.ScorerGroundedness},
		{name: "multiple scorable spans", trace: domain.Trace{Spans: []domain.Span{
			{IsScorable: true, Attributes: valid}, {IsScorable: true, Attributes: valid},
		}}, scorer: domain.ScorerGroundedness},
		{name: "invalid messages JSON", trace: traceWithScorableSpan(map[string]any{
			"gen_ai.input.messages": "not-json", "gen_ai.output.messages": valid["gen_ai.output.messages"],
		}), scorer: domain.ScorerGroundedness},
		{name: "trailing messages JSON", trace: traceWithScorableSpan(map[string]any{
			"gen_ai.input.messages":  `[] trailing`,
			"gen_ai.output.messages": valid["gen_ai.output.messages"],
		}), scorer: domain.ScorerGroundedness},
		{name: "missing assistant", trace: traceWithScorableSpan(map[string]any{
			"gen_ai.input.messages":  valid["gen_ai.input.messages"],
			"gen_ai.output.messages": []any{message("user", "answer")},
		}), scorer: domain.ScorerGroundedness},
		{name: "malformed flattened context", trace: traceWithScorableSpan(map[string]any{
			"gen_ai.input.messages":     valid["gen_ai.input.messages"],
			"gen_ai.output.messages":    valid["gen_ai.output.messages"],
			"assay.context.chunk.count": 2, "assay.context.chunks.0.id": "duplicate",
			"assay.context.chunks.0.text": "first", "assay.context.chunks.1.id": "duplicate",
			"assay.context.chunks.1.text": "second",
		}), scorer: domain.ScorerGroundedness},
		{name: "groundedness context required", trace: traceWithScorableSpan(map[string]any{
			"gen_ai.input.messages":  valid["gen_ai.input.messages"],
			"gen_ai.output.messages": valid["gen_ai.output.messages"],
		}), scorer: domain.ScorerGroundedness},
		{name: "correctness reference required", trace: traceWithScorableSpan(valid),
			scorer: domain.ScorerCorrectness},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.BuildTraceScoreInput(test.trace, test.scorer)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("trace score error = %v, want ErrInvalid", err)
			}
		})
	}
}

func message(role string, content string) map[string]any {
	return map[string]any{"role": role, "content": content}
}

func traceWithScorableSpan(attributes map[string]any) domain.Trace {
	return domain.Trace{Spans: []domain.Span{{IsScorable: true, Attributes: attributes}}}
}
