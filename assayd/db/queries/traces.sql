-- name: UpsertTrace :one
INSERT INTO traces (
    id, application_id, otel_trace_id, root_name, start_time, end_time, status,
    span_count, total_tokens, reference_answer, attributes
)
SELECT
    sqlc.arg(id), applications.id, sqlc.arg(otel_trace_id), sqlc.arg(root_name),
    sqlc.arg(start_time), sqlc.arg(end_time), sqlc.arg(status), sqlc.arg(span_count),
    sqlc.arg(total_tokens), sqlc.narg(reference_answer), sqlc.arg(attributes)
FROM applications
WHERE applications.id = sqlc.arg(application_id)
  AND applications.project_id = sqlc.arg(project_id)
ON CONFLICT (application_id, otel_trace_id) DO UPDATE
SET updated_at = now()
RETURNING id, application_id, otel_trace_id, root_name, start_time, end_time, status,
          span_count, total_tokens, total_cost, reference_answer, attributes,
          created_at, updated_at;

-- name: UpsertSpan :exec
INSERT INTO spans (
    trace_id, application_id, otel_span_id, parent_span_id, name, kind,
    operation_name, start_time, end_time, duration_ms, status_code, status_message,
    is_scorable, scorable_kind, attributes, events, input_tokens, output_tokens,
    reference_answer
)
VALUES (
    sqlc.arg(trace_id), sqlc.arg(application_id), sqlc.arg(otel_span_id),
    sqlc.narg(parent_span_id), sqlc.arg(name), sqlc.arg(kind),
    sqlc.arg(operation_name), sqlc.arg(start_time), sqlc.arg(end_time),
    sqlc.arg(duration_ms), sqlc.arg(status_code), sqlc.arg(status_message),
    sqlc.arg(is_scorable), sqlc.arg(scorable_kind), sqlc.arg(attributes),
    sqlc.arg(events), sqlc.arg(input_tokens), sqlc.arg(output_tokens),
    sqlc.narg(reference_answer)
)
ON CONFLICT (trace_id, otel_span_id, start_time) DO UPDATE
SET application_id = EXCLUDED.application_id,
    parent_span_id = EXCLUDED.parent_span_id,
    name = EXCLUDED.name,
    kind = EXCLUDED.kind,
    operation_name = EXCLUDED.operation_name,
    end_time = EXCLUDED.end_time,
    duration_ms = EXCLUDED.duration_ms,
    status_code = EXCLUDED.status_code,
    status_message = EXCLUDED.status_message,
    is_scorable = EXCLUDED.is_scorable,
    scorable_kind = EXCLUDED.scorable_kind,
    attributes = EXCLUDED.attributes,
    events = coalesce((
        SELECT jsonb_agg(preserved.event ORDER BY preserved.source, preserved.position)
        FROM (
            SELECT event AS event, 0 AS source, position
            FROM jsonb_array_elements(EXCLUDED.events) WITH ORDINALITY incoming(event, position)
            WHERE event->>'name' NOT LIKE 'assay.%'
            UNION ALL
            SELECT event AS event, 1 AS source, position
            FROM jsonb_array_elements(spans.events) WITH ORDINALITY stored(event, position)
            WHERE event->>'name' = 'gen_ai.evaluation.result'
              AND event->'attributes' ? 'assay.evaluation.judge.model'
        ) preserved
    ), '[]'::jsonb),
    input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens,
    reference_answer = EXCLUDED.reference_answer;

-- name: RefreshTraceSummary :one
WITH summary AS (
    SELECT
        spans.trace_id,
        min(spans.start_time) AS start_time,
        max(spans.end_time) AS end_time,
        count(*)::integer AS span_count,
        coalesce(sum(input_tokens + output_tokens), 0)::bigint AS total_tokens,
        max(reference_answer) FILTER (WHERE reference_answer IS NOT NULL) AS reference_answer
    FROM spans
    WHERE spans.trace_id = sqlc.arg(selected_trace_id)
    GROUP BY spans.trace_id
), root AS (
    SELECT name, status_code, attributes
    FROM spans
    WHERE spans.trace_id = sqlc.arg(selected_trace_id)
    ORDER BY (parent_span_id IS NULL) DESC, start_time, id
    LIMIT 1
)
UPDATE traces
SET root_name = root.name,
    start_time = summary.start_time,
    end_time = summary.end_time,
    status = root.status_code,
    span_count = summary.span_count,
    total_tokens = summary.total_tokens,
    reference_answer = summary.reference_answer,
    attributes = root.attributes,
    updated_at = now()
FROM summary, root
WHERE traces.id = summary.trace_id
RETURNING traces.id, traces.application_id, traces.otel_trace_id, traces.root_name,
          traces.start_time, traces.end_time, traces.status, traces.span_count,
          traces.total_tokens, traces.total_cost, traces.reference_answer, traces.attributes,
          traces.created_at, traces.updated_at;

-- name: ListProjectTraces :many
SELECT traces.id, traces.application_id, traces.otel_trace_id, traces.root_name,
       traces.start_time, traces.end_time, traces.status, traces.span_count,
       traces.total_tokens, traces.total_cost, traces.reference_answer, traces.attributes,
       traces.created_at, traces.updated_at
FROM traces
JOIN applications ON applications.id = traces.application_id
WHERE applications.project_id = sqlc.arg(project_id)
  AND (NOT sqlc.arg(filter_application)::boolean
       OR traces.application_id = sqlc.arg(application_id))
  AND (NOT sqlc.arg(filter_start)::boolean OR traces.start_time >= sqlc.arg(start_time))
  AND (NOT sqlc.arg(filter_end)::boolean OR traces.start_time < sqlc.arg(end_time))
  AND (NOT sqlc.arg(filter_status)::boolean OR traces.status = sqlc.arg(status))
  AND (NOT sqlc.arg(has_cursor)::boolean
       OR (traces.start_time, traces.id) < (
           sqlc.arg(cursor_time)::timestamptz,
           sqlc.arg(cursor_id)::uuid
       ))
ORDER BY traces.start_time DESC, traces.id DESC
LIMIT sqlc.arg(page_size);

-- name: GetProjectTrace :one
SELECT traces.id, traces.application_id, traces.otel_trace_id, traces.root_name,
       traces.start_time, traces.end_time, traces.status, traces.span_count,
       traces.total_tokens, traces.total_cost, traces.reference_answer, traces.attributes,
       traces.created_at, traces.updated_at
FROM traces
JOIN applications ON applications.id = traces.application_id
WHERE traces.id = $1 AND applications.project_id = $2;

-- name: ListProjectTraceSpans :many
SELECT spans.id, spans.trace_id, spans.application_id, spans.otel_span_id,
       spans.parent_span_id, spans.name, spans.kind, spans.operation_name,
       spans.start_time, spans.end_time, spans.duration_ms, spans.status_code,
       spans.status_message, spans.is_scorable, spans.scorable_kind, spans.attributes,
       spans.events, spans.input_tokens, spans.output_tokens, spans.reference_answer,
       spans.created_at
FROM spans
JOIN applications ON applications.id = spans.application_id
WHERE spans.trace_id = $1 AND applications.project_id = $2
ORDER BY spans.start_time, spans.id;

-- name: GetTraceForScoring :one
SELECT * FROM traces WHERE id = $1;

-- name: ListTraceSpansForScoring :many
SELECT * FROM spans WHERE trace_id = $1 ORDER BY start_time, id;

-- name: ListOnlineScores :many
SELECT * FROM scores WHERE trace_id = $1 ORDER BY created_at, id;

-- name: TraceEligibleForAutoScore :one
SELECT
    (SELECT count(*) FROM spans s WHERE s.trace_id = sqlc.arg(selected_trace_id) AND s.is_scorable) = 1
    AND EXISTS (
        SELECT 1 FROM spans s
        WHERE s.trace_id = sqlc.arg(selected_trace_id) AND s.is_scorable
          AND s.attributes ? 'gen_ai.input.messages'
          AND s.attributes ? 'gen_ai.output.messages'
    )
    AND CASE sqlc.arg(scorer)::text
        WHEN 'correctness' THEN traces.reference_answer IS NOT NULL
        WHEN 'groundedness' THEN EXISTS (
            SELECT 1 FROM spans s
            WHERE s.trace_id = sqlc.arg(selected_trace_id)
              AND (
                  s.attributes ? 'assay.context.chunk.count'
                  OR s.attributes ? 'gen_ai.retrieval.documents'
                  OR EXISTS (
                      SELECT 1 FROM jsonb_object_keys(s.attributes) key
                      WHERE key LIKE 'assay.context.chunks.%'
                  )
              )
        )
        ELSE false
    END
FROM traces WHERE id = sqlc.arg(selected_trace_id);

-- name: AttachTraceReference :one
WITH target AS MATERIALIZED (
    SELECT traces.id
    FROM traces
    JOIN applications ON applications.id = traces.application_id
    WHERE traces.id = sqlc.arg(trace_id) AND applications.project_id = sqlc.arg(project_id)
      AND (SELECT count(*) FROM spans WHERE spans.trace_id = traces.id AND is_scorable) = 1
    FOR UPDATE
), updated_span AS (
    UPDATE spans
    SET reference_answer = sqlc.arg(reference_answer)
    WHERE trace_id = (SELECT id FROM target) AND is_scorable
    RETURNING trace_id
)
UPDATE traces
SET reference_answer = sqlc.arg(reference_answer), updated_at = now()
WHERE id = (SELECT trace_id FROM updated_span)
RETURNING *;

-- name: LockOwnedScoringTask :one
SELECT id
FROM jobs
WHERE id = sqlc.arg(job_id) AND kind = 'scoring_task' AND status = 'running'
  AND locked_by = sqlc.arg(worker_id) AND lease_expires_at > now()
  AND trace_id = sqlc.arg(trace_id) AND scorer = sqlc.arg(scorer)
FOR UPDATE;

-- name: DeleteOnlineScore :exec
DELETE FROM scores WHERE trace_id = $1 AND scorer = $2;

-- name: InsertOnlineScore :one
INSERT INTO scores (
    scorer, scorer_config_id, value, threshold, passed, rationale, details,
    prompt_template_id, judge_model, judge_provider, judge_tokens,
    trace_id, span_id, span_start_time, judged_input, judged_output,
    judged_context, judged_reference
)
VALUES (
    sqlc.arg(scorer), sqlc.narg(scorer_config_id), sqlc.arg(value), sqlc.arg(threshold),
    sqlc.arg(passed), sqlc.arg(rationale), sqlc.arg(details), sqlc.arg(prompt_template_id),
    sqlc.arg(judge_model), sqlc.arg(judge_provider), sqlc.arg(judge_tokens),
    sqlc.arg(trace_id), sqlc.arg(span_id), sqlc.arg(span_start_time), sqlc.arg(judged_input),
    sqlc.arg(judged_output), sqlc.arg(judged_context), sqlc.narg(judged_reference)
)
RETURNING *;

-- name: GetScoredSpanEvents :one
SELECT events FROM spans
WHERE id = sqlc.arg(span_id) AND trace_id = sqlc.arg(trace_id)
  AND start_time = sqlc.arg(span_start_time)
FOR UPDATE;

-- name: UpdateScoredSpanEvents :one
UPDATE spans SET events = sqlc.arg(events)
WHERE id = sqlc.arg(span_id) AND trace_id = sqlc.arg(trace_id)
  AND start_time = sqlc.arg(span_start_time)
RETURNING id;
