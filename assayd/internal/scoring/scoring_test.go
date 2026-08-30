package scoring

import (
	"context"
	"strings"
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestGroundednessScoresJointVerdicts(t *testing.T) {
	judge := &scriptedJudge{responses: []JudgeResponse{
		{Content: `{"claims":[{"id":"c1","text":"one"},{"id":"c2","text":"two"}],` +
			`"excluded":[]}`, Tokens: 3},
		{Content: `{"verdicts":[{"id":"c1","verdict":"supported",` +
			`"supporting_chunk_ids":["k0"],"reason":"found"},` +
			`{"id":"c2","verdict":"unsupported","supporting_chunk_ids":[],` +
			`"reason":"missing"}]}`, Tokens: 5},
	}}
	input := ScoreInput{
		Input: "question", Output: "answer",
		Context: []domain.Chunk{{ID: "k0", Text: "context"}},
	}

	result, err := NewRegistry().mustGet(t, domain.ScorerGroundedness).Score(
		t.Context(), input, judge,
	)
	if err != nil {
		t.Fatalf("score groundedness: %v", err)
	}
	if result.Value != 0.5 || result.JudgeTokens != 8 ||
		!strings.Contains(result.Rationale, "1 of 2") {
		t.Fatalf("groundedness result = %#v", result)
	}
	if len(judge.requests) != 2 {
		t.Fatalf("judge calls = %d, want 2", len(judge.requests))
	}
}

func TestGroundednessSkipsVerificationForNoClaims(t *testing.T) {
	judge := &scriptedJudge{responses: []JudgeResponse{{
		Content: `{"claims":[],"excluded":[{"text":"hello","reason":"greeting"}]}`,
		Tokens:  2,
	}}}
	result, err := NewRegistry().mustGet(t, domain.ScorerGroundedness).Score(
		t.Context(), ScoreInput{Input: "q", Output: "hello"}, judge,
	)
	if err != nil {
		t.Fatalf("score no claims: %v", err)
	}
	if result.Value != 1 || len(judge.requests) != 1 {
		t.Fatalf("result/calls = %#v/%d", result, len(judge.requests))
	}
}

func TestGroundednessRejectsMissingClaimArrays(t *testing.T) {
	if err := validateExtraction(extractionResponse{}); err == nil {
		t.Fatal("missing claim arrays were accepted")
	}
}

func TestGroundednessRejectsMalformedExcludedContent(t *testing.T) {
	err := validateExtraction(extractionResponse{
		Claims: []claim{}, Excluded: []excludedContent{{Text: "", Reason: ""}},
	})
	if err == nil {
		t.Fatal("blank excluded content was accepted")
	}
}

func TestGroundednessRejectsInventedEvidence(t *testing.T) {
	err := validateVerification(
		verificationResponse{Verdicts: []verdict{{
			ID: "c1", Verdict: "supported", SupportingChunkIDs: []string{"invented"},
			Reason: "supported",
		}}},
		[]claim{{ID: "c1", Text: "claim"}},
		[]domain.Chunk{{ID: "k0", Text: "context"}},
	)
	if err == nil {
		t.Fatal("invented supporting chunk was accepted")
	}
}

func TestCorrectnessCapsContradictions(t *testing.T) {
	judge := &scriptedJudge{responses: []JudgeResponse{{
		Content: `{"reference_facts":[{"fact":"Paris is the capital",` +
			`"status":"contradicted"}],"contradictions":["London"],` +
			`"reasoning":"The capital is wrong.","score":0.9}`,
		Tokens: 7,
	}}}
	result, err := NewRegistry().mustGet(t, domain.ScorerCorrectness).Score(
		t.Context(), ScoreInput{Input: "q", Output: "London", Reference: "Paris"}, judge,
	)
	if err != nil {
		t.Fatalf("score correctness: %v", err)
	}
	if result.Value != 0.3 || result.Rationale != "The capital is wrong." ||
		result.JudgeTokens != 7 {
		t.Fatalf("correctness result = %#v", result)
	}
}

func TestCorrectnessRejectsEmptyFactAssessment(t *testing.T) {
	err := validateCorrectness(correctnessResponse{Reasoning: "matches", Score: 1})
	if err == nil {
		t.Fatal("empty reference fact assessment was accepted")
	}
}

func TestStructuredResponseRetriesOnce(t *testing.T) {
	judge := &scriptedJudge{responses: []JudgeResponse{
		{Content: `{"claims":"bad"}`},
		{Content: `{"claims":[],"excluded":[]}`},
	}}
	_, err := NewRegistry().mustGet(t, domain.ScorerGroundedness).Score(
		t.Context(), ScoreInput{Input: "q", Output: "answer"}, judge,
	)
	if err != nil {
		t.Fatalf("retry structured response: %v", err)
	}
	if len(judge.requests) != 2 || judge.requests[1].Correction == "" {
		t.Fatalf("retry requests = %#v", judge.requests)
	}
}

func TestStructuredRetryDoesNotReuseInvalidResponseFields(t *testing.T) {
	judge := &scriptedJudge{responses: []JudgeResponse{
		{Content: `{"reference_facts":[],"contradictions":[],` +
			`"reasoning":"stale","score":2}`},
		{Content: `{"reference_facts":[],"contradictions":[],"score":0.5}`},
	}}
	_, err := NewRegistry().mustGet(t, domain.ScorerCorrectness).Score(
		t.Context(), ScoreInput{Input: "q", Output: "a", Reference: "r"}, judge,
	)
	if err == nil {
		t.Fatal("corrective response missing reasoning was accepted")
	}
}

func (r Registry) mustGet(t *testing.T, name string) Scorer {
	t.Helper()
	scorer, found := r.Get(name)
	if !found {
		t.Fatalf("scorer %q not registered", name)
	}
	return scorer
}

type scriptedJudge struct {
	responses []JudgeResponse
	requests  []JudgeRequest
}

func (j *scriptedJudge) Complete(
	_ context.Context,
	request JudgeRequest,
) (JudgeResponse, error) {
	j.requests = append(j.requests, request)
	response := j.responses[0]
	j.responses = j.responses[1:]
	return response, nil
}
