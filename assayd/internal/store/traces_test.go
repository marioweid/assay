package store

import (
	"testing"
	"time"

	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestTraceFromRowPreservesIdentityAndAttributes(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	traceID := []byte("0123456789abcdef")
	row := db.Trace{
		ID:            uuid.Must(uuid.NewV7()),
		ApplicationID: uuid.Must(uuid.NewV7()),
		OtelTraceID:   traceID,
		RootName:      "answer",
		StartTime:     pgtype.Timestamptz{Time: now, Valid: true},
		EndTime:       pgtype.Timestamptz{Time: now.Add(time.Second), Valid: true},
		Status:        "ok",
		SpanCount:     2,
		TotalTokens:   12,
		Attributes: []byte(
			`{"service.name":"support","large":9007199254740993,` +
				`"nested":[9007199254740995]}`,
		),
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}

	trace, err := traceFromRow(row)
	if err != nil {
		t.Fatalf("convert trace: %v", err)
	}
	identityMatches := string(trace.OTelTraceID[:]) == string(traceID)
	if !identityMatches || trace.Attributes["service.name"] != "support" {
		t.Fatalf("converted trace = %#v", trace)
	}
	assertExactIntegerAttributes(t, trace.Attributes)

	row.OtelTraceID = traceID[:15]
	if _, err := traceFromRow(row); err == nil {
		t.Fatal("convert short trace ID succeeded")
	}
}

func assertExactIntegerAttributes(t *testing.T, attributes map[string]any) {
	t.Helper()
	if attributes["large"] != int64(9007199254740993) {
		t.Fatalf("large integer attribute = %#v", attributes["large"])
	}
	nested, ok := attributes["nested"].([]any)
	if !ok || len(nested) != 1 || nested[0] != int64(9007199254740995) {
		t.Fatalf("nested integer attributes = %#v", attributes["nested"])
	}
}

func TestSpanFromRowPreservesParentAndEvents(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	spanID := []byte("span-id1")
	parentID := []byte("parent01")
	row := db.Span{
		ID:            1,
		TraceID:       uuid.Must(uuid.NewV7()),
		ApplicationID: uuid.Must(uuid.NewV7()),
		OtelSpanID:    spanID,
		ParentSpanID:  parentID,
		Name:          "generation",
		Kind:          "client",
		StartTime:     pgtype.Timestamptz{Time: now, Valid: true},
		EndTime:       pgtype.Timestamptz{Time: now.Add(time.Second), Valid: true},
		StatusCode:    "ok",
		Attributes:    []byte(`{"assay.scorable":true}`),
		Events: []byte(
			`[{"time":"2026-08-28T12:00:00Z","name":"done",` +
				`"attributes":{},"dropped_attributes_count":0}]`,
		),
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}

	span, err := spanFromRow(row)
	if err != nil {
		t.Fatalf("convert span: %v", err)
	}
	if span.ParentSpanID == nil || string(span.ParentSpanID[:]) != string(parentID) {
		t.Fatalf("converted parent = %#v", span.ParentSpanID)
	}
	if len(span.Events) != 1 || span.Events[0].Name != "done" {
		t.Fatalf("converted events = %#v", span.Events)
	}
}
