package api

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestTraceCursorRoundTrip(t *testing.T) {
	want := domain.TraceCursor{
		StartTime: time.Date(2026, 8, 28, 12, 0, 0, 123, time.UTC),
		ID:        uuid.Must(uuid.NewV7()),
	}
	encoded, err := encodeTraceCursor(&want)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	got, err := decodeTraceCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if got == nil || got.ID != want.ID || !got.StartTime.Equal(want.StartTime) {
		t.Fatalf("decoded cursor = %#v, want %#v", got, want)
	}
	if _, err := decodeTraceCursor("not-base64"); err == nil {
		t.Fatal("malformed cursor decoded")
	}
}

func TestTraceOutputBuildsTreeAndKeepsOrphansAsRoots(t *testing.T) {
	rootID := [8]byte{1}
	childID := [8]byte{2}
	missingParent := [8]byte{9}
	traceID := [16]byte{1, 2, 3}
	reference := "expected answer"
	trace := domain.Trace{
		ID:          uuid.Must(uuid.NewV7()),
		OTelTraceID: traceID,
		Spans: []domain.Span{
			{Name: "root", OTelSpanID: rootID},
			{
				Name:            "child",
				OTelSpanID:      childID,
				ParentSpanID:    &rootID,
				InputTokens:     12,
				OutputTokens:    4,
				ReferenceAnswer: &reference,
			},
			{Name: "orphan", OTelSpanID: [8]byte{3}, ParentSpanID: &missingParent},
		},
	}

	output := traceOutput(trace, true)
	assertTraceTree(t, output, traceID)
	assertExtractedChildFields(t, output.Spans[0].Children[0])
}

func TestTraceScoreSummariesAreListOnly(t *testing.T) {
	trace := domain.Trace{ScoreSummaries: []domain.TraceScoreSummary{{Scorer: "correctness"}}}
	listJSON, err := json.Marshal(traceListOutput(trace))
	if err != nil {
		t.Fatalf("marshal trace list output: %v", err)
	}
	detailJSON, err := json.Marshal(traceOutput(trace, true))
	if err != nil {
		t.Fatalf("marshal trace detail output: %v", err)
	}
	var list, detail map[string]any
	if err := json.Unmarshal(listJSON, &list); err != nil {
		t.Fatalf("decode trace list output: %v", err)
	}
	if err := json.Unmarshal(detailJSON, &detail); err != nil {
		t.Fatalf("decode trace detail output: %v", err)
	}
	if _, found := list["score_summaries"]; !found {
		t.Fatalf("trace list output = %#v, want score summaries", list)
	}
	if _, found := detail["score_summaries"]; found {
		t.Fatalf("trace detail output = %#v, do not want score summaries", detail)
	}
}

func TestScoreOutputIncludesOnlineAuditFields(t *testing.T) {
	traceID := uuid.Must(uuid.NewV7())
	spanID := int64(42)
	spanTime := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	reference := "reference"
	response := scoreOutput(domain.Score{
		TraceID: &traceID, SpanID: &spanID, SpanStartTime: &spanTime,
		JudgedInput: "question", JudgedOutput: "answer",
		JudgedContext:   []domain.Chunk{{ID: "k0", Text: "context"}},
		JudgedReference: &reference,
	})
	if response.TraceID == nil || *response.TraceID != traceID.String() ||
		response.JudgedInput == nil || *response.JudgedInput != "question" ||
		response.SpanStartTime == nil || len(response.JudgedContext) != 1 {
		t.Fatalf("online score response = %#v", response)
	}
}

func assertTraceTree(t *testing.T, output traceResponse, traceID [16]byte) {
	t.Helper()
	if output.OTelTraceID != hex.EncodeToString(traceID[:]) || len(output.Spans) != 2 {
		t.Fatalf("trace output = %#v", output)
	}
	if output.Spans[0].Name != "root" || len(output.Spans[0].Children) != 1 ||
		output.Spans[0].Children[0].Name != "child" {
		t.Fatalf("span tree = %#v", output.Spans)
	}
	if output.Spans[1].Name != "orphan" {
		t.Fatalf("orphan root = %#v", output.Spans[1])
	}
}

func assertExtractedChildFields(t *testing.T, child *spanResponse) {
	t.Helper()
	if child.InputTokens != 12 || child.OutputTokens != 4 ||
		child.ReferenceAnswer == nil || *child.ReferenceAnswer != "expected answer" {
		t.Fatalf("extracted child fields = %#v", child)
	}
}
