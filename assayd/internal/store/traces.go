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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UpsertTraces persists an accepted OTLP request atomically.
//
//nolint:cyclop // One transaction checks each trace, summary, and optional task transition.
func (d *Database) UpsertTraces(
	ctx context.Context,
	projectID uuid.UUID,
	traces []domain.Trace,
	intents []domain.AutoScoreIntent,
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
	storedIDs := make(map[traceIdentity]uuid.UUID, len(traces))
	for _, trace := range traces {
		storedID, upsertErr := upsertTrace(ctx, queries, projectID, trace)
		if upsertErr != nil {
			return upsertErr
		}
		affected[storedID] = struct{}{}
		identity := traceIdentity{
			applicationID: trace.ApplicationID, otelTraceID: trace.OTelTraceID,
		}
		storedIDs[identity] = storedID
	}
	for _, intent := range intents {
		traceID, found := storedIDs[traceIdentity{
			applicationID: intent.ApplicationID, otelTraceID: intent.OTelTraceID,
		}]
		if !found {
			return fmt.Errorf("queue automatic score: %w: trace was not ingested", domain.ErrInvalid)
		}
		eligible, err := queries.TraceEligibleForAutoScore(ctx, db.TraceEligibleForAutoScoreParams{
			SelectedTraceID: traceID, Scorer: intent.Scorer,
		})
		if err != nil {
			return mapStoreError("validate automatic score", err)
		}
		if !eligible.Valid || !eligible.Bool {
			continue
		}
		_, err = queries.CreateScoringTask(ctx, db.CreateScoringTaskParams{
			ID: intent.JobID, TraceID: nullableUUID(&traceID),
			Scorer: nullableText(&intent.Scorer), MaxAttempts: int32(intent.MaxAttempts),
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return mapStoreError("queue automatic score", err)
		}
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

type traceIdentity struct {
	applicationID uuid.UUID
	otelTraceID   [16]byte
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
//
//nolint:cyclop // Detail assembly validates three independently decoded child collections.
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
	scoreRows, err := d.queries.ListOnlineScores(ctx, nullableUUID(&traceID))
	if err != nil {
		return domain.Trace{}, mapStoreError("select trace scores", err)
	}
	for _, scoreRow := range scoreRows {
		score, convertErr := scoreFromRow(scoreRow)
		if convertErr != nil {
			return domain.Trace{}, convertErr
		}
		trace.Scores = append(trace.Scores, score)
	}
	jobRows, err := d.queries.ListTraceScoringTasks(ctx, nullableUUID(&traceID))
	if err != nil {
		return domain.Trace{}, mapStoreError("select trace scoring tasks", err)
	}
	for _, jobRow := range jobRows {
		job, convertErr := jobFromRow(jobRow)
		if convertErr != nil {
			return domain.Trace{}, convertErr
		}
		trace.ScoringTasks = append(trace.ScoringTasks, job)
	}
	return trace, nil
}

// QueueTraceScores atomically creates or refreshes validated project trace tasks.
func (d *Database) QueueTraceScores(
	ctx context.Context,
	projectID uuid.UUID,
	requests []domain.TraceScoreRequest,
	refresh bool,
) ([]domain.Job, error) {
	transaction, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin trace score queue transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := d.queries.WithTx(transaction)
	jobs := make([]domain.Job, 0, len(requests))
	for _, request := range requests {
		if _, err := queries.GetProjectTrace(ctx, db.GetProjectTraceParams{
			ID: request.TraceID, ProjectID: projectID,
		}); err != nil {
			return nil, mapStoreError("validate queued trace", err)
		}
		job, err := queueTraceScore(ctx, queries, request, refresh)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit trace score queue transaction: %w", err)
	}
	return jobs, nil
}

func queueTraceScore(
	ctx context.Context,
	queries *db.Queries,
	request domain.TraceScoreRequest,
	refresh bool,
) (domain.Job, error) {
	params := db.CreateScoringTaskParams{
		ID: request.JobID, TraceID: nullableUUID(&request.TraceID),
		Scorer: nullableText(&request.Scorer), MaxAttempts: int32(request.MaxAttempts),
	}
	var row db.Job
	var err error
	if refresh {
		row, err = queries.RefreshScoringTask(ctx, db.RefreshScoringTaskParams(params))
	} else {
		row, err = queries.CreateScoringTask(ctx, params)
	}
	if errors.Is(err, pgx.ErrNoRows) && !refresh {
		rows, selectErr := queries.ListTraceScoringTasks(ctx, nullableUUID(&request.TraceID))
		if selectErr != nil {
			return domain.Job{}, mapStoreError("select existing trace score task", selectErr)
		}
		for _, existing := range rows {
			if existing.Scorer.String == request.Scorer {
				return jobFromRow(existing)
			}
		}
	}
	if err != nil {
		return domain.Job{}, mapStoreError("queue trace score", err)
	}
	return jobFromRow(row)
}

// AttachTraceReference atomically updates trace/span reference and an optional correctness task.
//
//nolint:cyclop // The transaction keeps reference, span, and optional task writes indivisible.
func (d *Database) AttachTraceReference(
	ctx context.Context,
	projectID uuid.UUID,
	traceID uuid.UUID,
	reference string,
	job *domain.Job,
) (domain.Trace, error) {
	transaction, err := d.pool.Begin(ctx)
	if err != nil {
		return domain.Trace{}, fmt.Errorf("begin trace reference transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := d.queries.WithTx(transaction)
	row, err := queries.AttachTraceReference(ctx, db.AttachTraceReferenceParams{
		TraceID: traceID, ProjectID: projectID, ReferenceAnswer: nullableText(&reference),
	})
	if err != nil {
		return domain.Trace{}, mapStoreError("attach trace reference", err)
	}
	if job != nil {
		request := domain.TraceScoreRequest{
			TraceID: traceID, Scorer: job.Scorer, JobID: job.ID, MaxAttempts: job.MaxAttempts,
		}
		if _, err := queueTraceScore(ctx, queries, request, true); err != nil {
			return domain.Trace{}, err
		}
	}
	trace, err := traceFromRow(row)
	if err != nil {
		return domain.Trace{}, err
	}
	spanRows, err := queries.ListProjectTraceSpans(ctx, db.ListProjectTraceSpansParams{
		TraceID: traceID, ProjectID: projectID,
	})
	if err != nil {
		return domain.Trace{}, mapStoreError("select referenced trace spans", err)
	}
	for _, spanRow := range spanRows {
		span, convertErr := spanFromRow(spanRow)
		if convertErr != nil {
			return domain.Trace{}, convertErr
		}
		trace.Spans = append(trace.Spans, span)
	}
	if err := transaction.Commit(ctx); err != nil {
		return domain.Trace{}, fmt.Errorf("commit trace reference transaction: %w", err)
	}
	return trace, nil
}

// GetTraceForScoring returns one trace and all spans without project presentation scoping.
func (d *Database) GetTraceForScoring(
	ctx context.Context,
	traceID uuid.UUID,
) (domain.Trace, error) {
	row, err := d.queries.GetTraceForScoring(ctx, traceID)
	if err != nil {
		return domain.Trace{}, mapStoreError("select trace for scoring", err)
	}
	trace, err := traceFromRow(row)
	if err != nil {
		return domain.Trace{}, err
	}
	rows, err := d.queries.ListTraceSpansForScoring(ctx, traceID)
	if err != nil {
		return domain.Trace{}, mapStoreError("select trace spans for scoring", err)
	}
	for _, spanRow := range rows {
		span, convertErr := spanFromRow(spanRow)
		if convertErr != nil {
			return domain.Trace{}, convertErr
		}
		trace.Spans = append(trace.Spans, span)
	}
	return trace, nil
}

// CompleteTraceScore replaces one online score and its Assay-generated span event under lease.
//
//nolint:cyclop // The lease-fenced transaction checks every persistence boundary explicitly.
func (d *Database) CompleteTraceScore(
	ctx context.Context,
	score domain.Score,
	lease domain.JobLease,
) error {
	if score.TraceID == nil || score.SpanID == nil || score.SpanStartTime == nil {
		return fmt.Errorf("complete trace score: %w: incomplete target", domain.ErrInvalid)
	}
	transaction, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin trace score transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := d.queries.WithTx(transaction)
	if _, err := queries.LockOwnedScoringTask(ctx, db.LockOwnedScoringTaskParams{
		JobID: lease.JobID, WorkerID: nullableText(&lease.WorkerID),
		TraceID: nullableUUID(score.TraceID), Scorer: nullableText(&score.Scorer),
	}); err != nil {
		return mapStoreError("lock scoring task", err)
	}
	events, err := queries.GetScoredSpanEvents(ctx, db.GetScoredSpanEventsParams{
		SpanID: *score.SpanID, TraceID: *score.TraceID,
		SpanStartTime: timestamp(*score.SpanStartTime),
	})
	if err != nil {
		return mapStoreError("lock scored span", err)
	}
	if err := queries.DeleteOnlineScore(ctx, db.DeleteOnlineScoreParams{
		TraceID: nullableUUID(score.TraceID), Scorer: score.Scorer,
	}); err != nil {
		return mapStoreError("delete previous online score", err)
	}
	row, err := insertOnlineScore(ctx, queries, score)
	if err != nil {
		return err
	}
	updatedEvents, err := onlineScoreEvents(events, score, row.CreatedAt.Time)
	if err != nil {
		return err
	}
	if _, err := queries.UpdateScoredSpanEvents(ctx, db.UpdateScoredSpanEventsParams{
		Events: updatedEvents, SpanID: *score.SpanID, TraceID: *score.TraceID,
		SpanStartTime: timestamp(*score.SpanStartTime),
	}); err != nil {
		return mapStoreError("update scored span event", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit trace score transaction: %w", err)
	}
	return nil
}

func insertOnlineScore(
	ctx context.Context,
	queries *db.Queries,
	score domain.Score,
) (db.Score, error) {
	details, err := encodeJSON("online score details", score.Details)
	if err != nil {
		return db.Score{}, err
	}
	contextJSON, err := encodeJSON("judged context", score.JudgedContext)
	if err != nil {
		return db.Score{}, err
	}
	value, err := numeric(score.Value)
	if err != nil {
		return db.Score{}, fmt.Errorf("encode online score value: %w", err)
	}
	threshold, err := numeric(score.Threshold)
	if err != nil {
		return db.Score{}, fmt.Errorf("encode online score threshold: %w", err)
	}
	row, err := queries.InsertOnlineScore(ctx, db.InsertOnlineScoreParams{
		Scorer: score.Scorer, ScorerConfigID: nullableUUID(score.ScorerConfigID),
		Value: value, Threshold: threshold, Passed: score.Passed, Rationale: score.Rationale,
		Details: details, PromptTemplateID: score.PromptTemplateID,
		JudgeModel: score.JudgeModel, JudgeProvider: score.JudgeProvider,
		JudgeTokens: int32(score.JudgeTokens), TraceID: nullableUUID(score.TraceID),
		SpanID:        pgtype.Int8{Int64: *score.SpanID, Valid: true},
		SpanStartTime: timestamp(*score.SpanStartTime), JudgedInput: nullableText(&score.JudgedInput),
		JudgedOutput: nullableText(&score.JudgedOutput), JudgedContext: contextJSON,
		JudgedReference: nullableText(score.JudgedReference),
	})
	if err != nil {
		return db.Score{}, mapStoreError("insert online score", err)
	}
	return row, nil
}

func onlineScoreEvents(
	payload []byte,
	score domain.Score,
	createdAt time.Time,
) ([]byte, error) {
	var events []domain.SpanEvent
	if err := decodeStoredJSON(payload, &events); err != nil {
		return nil, fmt.Errorf("decode scored span events: %w", err)
	}
	kept := events[:0]
	for _, event := range events {
		generated := event.Name == "gen_ai.evaluation.result" &&
			event.Attributes["gen_ai.evaluation.name"] == score.Scorer &&
			event.Attributes["assay.evaluation.judge.model"] != nil
		if !generated {
			kept = append(kept, event)
		}
	}
	label := "fail"
	if score.Passed {
		label = "pass"
	}
	kept = append(kept, domain.SpanEvent{Time: createdAt, Name: "gen_ai.evaluation.result",
		Attributes: map[string]any{
			"gen_ai.evaluation.name": score.Scorer, "gen_ai.evaluation.score.value": score.Value,
			"gen_ai.evaluation.score.label":   label,
			"gen_ai.evaluation.explanation":   score.Rationale,
			"assay.evaluation.judge.model":    score.JudgeModel,
			"assay.evaluation.judge.provider": score.JudgeProvider,
		},
	})
	return encodeJSON("scored span events", kept)
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
