package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UpsertTraces persists an accepted OTLP request atomically.
func (d *Database) UpsertTraces(
	ctx context.Context,
	projectID uuid.UUID,
	traces []domain.Trace,
) error {
	transaction, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin trace ingest transaction: %w", err)
	}
	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	queries := d.queries.WithTx(transaction)
	affected := make(map[uuid.UUID]struct{}, len(traces))
	for _, trace := range traces {
		storedID, upsertErr := upsertTrace(ctx, queries, projectID, trace)
		if upsertErr != nil {
			return upsertErr
		}
		affected[storedID] = struct{}{}
	}
	for traceID := range affected {
		if _, err := queries.RefreshTraceSummary(ctx, traceID); err != nil {
			return mapStoreError("refresh trace summary", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit trace ingest transaction: %w", err)
	}
	return nil
}

func upsertTrace(
	ctx context.Context,
	queries *db.Queries,
	projectID uuid.UUID,
	trace domain.Trace,
) (uuid.UUID, error) {
	attributes, err := encodeJSON("trace attributes", trace.Attributes)
	if err != nil {
		return uuid.Nil, err
	}
	row, err := queries.UpsertTrace(ctx, db.UpsertTraceParams{
		ID:              trace.ID,
		OtelTraceID:     trace.OTelTraceID[:],
		RootName:        trace.RootName,
		StartTime:       timestamp(trace.StartTime),
		EndTime:         timestamp(trace.EndTime),
		Status:          trace.Status,
		SpanCount:       int32(trace.SpanCount),
		TotalTokens:     trace.TotalTokens,
		ReferenceAnswer: nullableText(trace.ReferenceAnswer),
		Attributes:      attributes,
		ApplicationID:   trace.ApplicationID,
		ProjectID:       projectID,
	})
	if err != nil {
		return uuid.Nil, mapStoreError("upsert trace", err)
	}
	for _, span := range trace.Spans {
		if err := upsertSpan(ctx, queries, row.ID, span); err != nil {
			return uuid.Nil, err
		}
	}
	return row.ID, nil
}

func upsertSpan(
	ctx context.Context,
	queries *db.Queries,
	traceID uuid.UUID,
	span domain.Span,
) error {
	attributes, err := encodeJSON("span attributes", span.Attributes)
	if err != nil {
		return err
	}
	events, err := encodeJSON("span events", span.Events)
	if err != nil {
		return err
	}
	parameters := db.UpsertSpanParams{
		TraceID:         traceID,
		ApplicationID:   span.ApplicationID,
		OtelSpanID:      span.OTelSpanID[:],
		Name:            span.Name,
		Kind:            span.Kind,
		OperationName:   span.OperationName,
		StartTime:       timestamp(span.StartTime),
		EndTime:         timestamp(span.EndTime),
		DurationMs:      span.DurationMS,
		StatusCode:      span.StatusCode,
		StatusMessage:   span.StatusMessage,
		IsScorable:      span.IsScorable,
		ScorableKind:    span.ScorableKind,
		Attributes:      attributes,
		Events:          events,
		InputTokens:     span.InputTokens,
		OutputTokens:    span.OutputTokens,
		ReferenceAnswer: nullableText(span.ReferenceAnswer),
	}
	if span.ParentSpanID != nil {
		parameters.ParentSpanID = span.ParentSpanID[:]
	}
	if err := queries.UpsertSpan(ctx, parameters); err != nil {
		return mapStoreError("upsert span", err)
	}
	return nil
}

// ListTraces returns traces matching project-scoped filters.
func (d *Database) ListTraces(
	ctx context.Context,
	projectID uuid.UUID,
	query domain.TraceQuery,
) ([]domain.Trace, error) {
	parameters := traceListParameters(projectID, query)
	rows, err := d.queries.ListProjectTraces(ctx, parameters)
	if err != nil {
		return nil, mapStoreError("list project traces", err)
	}
	traces := make([]domain.Trace, 0, len(rows))
	for _, row := range rows {
		trace, convertErr := traceFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		traces = append(traces, trace)
	}
	return traces, nil
}

func traceListParameters(projectID uuid.UUID, query domain.TraceQuery) db.ListProjectTracesParams {
	parameters := db.ListProjectTracesParams{
		ProjectID:    projectID,
		FilterStatus: query.Status != "",
		Status:       query.Status,
		PageSize:     int32(query.Limit),
	}
	if query.ApplicationID != nil {
		parameters.FilterApplication = true
		parameters.ApplicationID = *query.ApplicationID
	}
	if query.Start != nil {
		parameters.FilterStart = true
		parameters.StartTime = timestamp(*query.Start)
	}
	if query.End != nil {
		parameters.FilterEnd = true
		parameters.EndTime = timestamp(*query.End)
	}
	if query.Cursor != nil {
		parameters.HasCursor = true
		parameters.CursorTime = timestamp(query.Cursor.StartTime)
		parameters.CursorID = query.Cursor.ID
	}
	return parameters
}

// GetTrace returns one project-owned trace with all of its spans.
func (d *Database) GetTrace(
	ctx context.Context,
	projectID uuid.UUID,
	traceID uuid.UUID,
) (domain.Trace, error) {
	row, err := d.queries.GetProjectTrace(
		ctx,
		db.GetProjectTraceParams{ID: traceID, ProjectID: projectID},
	)
	if err != nil {
		return domain.Trace{}, mapStoreError("select project trace", err)
	}
	trace, err := traceFromRow(row)
	if err != nil {
		return domain.Trace{}, err
	}
	rows, err := d.queries.ListProjectTraceSpans(
		ctx,
		db.ListProjectTraceSpansParams{TraceID: traceID, ProjectID: projectID},
	)
	if err != nil {
		return domain.Trace{}, mapStoreError("select project trace spans", err)
	}
	trace.Spans = make([]domain.Span, 0, len(rows))
	for _, spanRow := range rows {
		span, convertErr := spanFromRow(spanRow)
		if convertErr != nil {
			return domain.Trace{}, convertErr
		}
		trace.Spans = append(trace.Spans, span)
	}
	return trace, nil
}

func traceFromRow(row db.Trace) (domain.Trace, error) {
	if len(row.OtelTraceID) != 16 {
		return domain.Trace{}, fmt.Errorf(
			"decode OTLP trace ID: got %d bytes, want 16",
			len(row.OtelTraceID),
		)
	}
	attributes := make(map[string]any)
	if err := decodeStoredJSON(row.Attributes, &attributes); err != nil {
		return domain.Trace{}, fmt.Errorf("decode trace attributes: %w", err)
	}
	if err := normalizeAttributeNumbers(attributes); err != nil {
		return domain.Trace{}, fmt.Errorf("decode trace attributes: %w", err)
	}
	totalCost, err := numericString(row.TotalCost)
	if err != nil {
		return domain.Trace{}, err
	}
	trace := domain.Trace{
		ID:              row.ID,
		ApplicationID:   row.ApplicationID,
		RootName:        row.RootName,
		StartTime:       row.StartTime.Time,
		EndTime:         row.EndTime.Time,
		Status:          row.Status,
		SpanCount:       int(row.SpanCount),
		TotalTokens:     row.TotalTokens,
		TotalCost:       totalCost,
		ReferenceAnswer: optionalText(row.ReferenceAnswer),
		Attributes:      attributes,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
	copy(trace.OTelTraceID[:], row.OtelTraceID)
	return trace, nil
}

func spanFromRow(row db.Span) (domain.Span, error) {
	if len(row.OtelSpanID) != 8 {
		return domain.Span{}, fmt.Errorf("decode OTLP span ID: got %d bytes, want 8", len(row.OtelSpanID))
	}
	attributes, events, err := decodeSpanMetadata(row)
	if err != nil {
		return domain.Span{}, err
	}
	span := domain.Span{
		ID:              row.ID,
		TraceID:         row.TraceID,
		ApplicationID:   row.ApplicationID,
		Name:            row.Name,
		Kind:            row.Kind,
		OperationName:   row.OperationName,
		StartTime:       row.StartTime.Time,
		EndTime:         row.EndTime.Time,
		DurationMS:      row.DurationMs,
		StatusCode:      row.StatusCode,
		StatusMessage:   row.StatusMessage,
		IsScorable:      row.IsScorable,
		ScorableKind:    row.ScorableKind,
		Attributes:      attributes,
		Events:          events,
		InputTokens:     row.InputTokens,
		OutputTokens:    row.OutputTokens,
		ReferenceAnswer: optionalText(row.ReferenceAnswer),
		CreatedAt:       row.CreatedAt.Time,
	}
	copy(span.OTelSpanID[:], row.OtelSpanID)
	if len(row.ParentSpanID) > 0 {
		if len(row.ParentSpanID) != 8 {
			return domain.Span{}, fmt.Errorf(
				"decode OTLP parent span ID: got %d bytes, want 8",
				len(row.ParentSpanID),
			)
		}
		span.ParentSpanID = &[8]byte{}
		copy(span.ParentSpanID[:], row.ParentSpanID)
	}
	return span, nil
}

func decodeSpanMetadata(row db.Span) (map[string]any, []domain.SpanEvent, error) {
	attributes := make(map[string]any)
	if err := decodeStoredJSON(row.Attributes, &attributes); err != nil {
		return nil, nil, fmt.Errorf("decode span attributes: %w", err)
	}
	if err := normalizeAttributeNumbers(attributes); err != nil {
		return nil, nil, fmt.Errorf("decode span attributes: %w", err)
	}
	var events []domain.SpanEvent
	if err := decodeStoredJSON(row.Events, &events); err != nil {
		return nil, nil, fmt.Errorf("decode span events: %w", err)
	}
	for index := range events {
		if err := normalizeAttributeNumbers(events[index].Attributes); err != nil {
			return nil, nil, fmt.Errorf("decode span event attributes: %w", err)
		}
	}
	return attributes, events, nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func nullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func numericString(value pgtype.Numeric) (*string, error) {
	if !value.Valid {
		return nil, nil
	}
	databaseValue, err := value.Value()
	if err != nil {
		return nil, fmt.Errorf("decode trace total cost: %w", err)
	}
	text, ok := databaseValue.(string)
	if !ok {
		return nil, errors.New("decode trace total cost: database value is not text")
	}
	return &text, nil
}

func decodeStoredJSON(payload []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	return decoder.Decode(destination)
}

func normalizeAttributeNumbers(attributes map[string]any) error {
	for key, value := range attributes {
		normalized, err := normalizeJSONNumber(value)
		if err != nil {
			return fmt.Errorf("attribute %q: %w", key, err)
		}
		attributes[key] = normalized
	}
	return nil
}

func normalizeJSONNumber(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		return parsedJSONNumber(typed)
	case []any:
		return normalizeJSONArray(typed)
	case map[string]any:
		if err := normalizeAttributeNumbers(typed); err != nil {
			return nil, err
		}
	}
	return value, nil
}

func parsedJSONNumber(value json.Number) (any, error) {
	if integer, err := value.Int64(); err == nil {
		return integer, nil
	}
	floating, err := value.Float64()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON number %q: %w", value, err)
	}
	return floating, nil
}

func normalizeJSONArray(values []any) ([]any, error) {
	for index, value := range values {
		normalized, err := normalizeJSONNumber(value)
		if err != nil {
			return nil, err
		}
		values[index] = normalized
	}
	return values, nil
}
