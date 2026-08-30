package api

import (
	"encoding/hex"
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
