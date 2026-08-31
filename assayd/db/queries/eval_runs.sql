-- name: CreateEvalRun :one
INSERT INTO eval_runs (
    id, application_id, dataset_id, name, status, mode, params, scorers, total_items
)
SELECT
	    sqlc.arg(id), datasets.application_id, datasets.id, sqlc.arg(name), 'pending',
	    sqlc.arg(mode), sqlc.arg(params), sqlc.arg(scorers),
    (SELECT count(*)::integer FROM dataset_items WHERE dataset_id = datasets.id)
FROM datasets
WHERE datasets.id = sqlc.arg(dataset_id)
  AND datasets.application_id = sqlc.arg(application_id)
RETURNING id, application_id, dataset_id, name, status, mode, params, scorers, aggregates,
          total_items, succeeded_items, failed_items, canceled_items, started_at, finished_at,
          error, created_at, updated_at;

-- name: CreateEvalRunItems :exec
INSERT INTO eval_run_items (eval_run_id, dataset_item_id, status)
SELECT sqlc.arg(eval_run_id), id, 'pending'
FROM dataset_items
WHERE dataset_id = sqlc.arg(dataset_id);

-- name: ListEvalRuns :many
SELECT id, application_id, dataset_id, name, status, mode, params, scorers, aggregates,
       total_items, succeeded_items, failed_items, canceled_items, started_at, finished_at,
       error, created_at, updated_at
FROM eval_runs
WHERE (NOT sqlc.arg(filter_application)::boolean
       OR application_id = sqlc.arg(application_id))
  AND (NOT sqlc.arg(filter_status)::boolean OR status = sqlc.arg(status))
  AND (NOT sqlc.arg(has_cursor)::boolean
       OR (created_at, id) > (sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::uuid))
ORDER BY created_at, id
LIMIT sqlc.arg(page_size);

-- name: GetEvalRun :one
SELECT id, application_id, dataset_id, name, status, mode, params, scorers, aggregates,
       total_items, succeeded_items, failed_items, canceled_items, started_at, finished_at,
       error, created_at, updated_at
FROM eval_runs
WHERE id = $1;

-- name: ListEvalRunItems :many
SELECT ri.eval_run_id, ri.dataset_item_id, ri.status, ri.error, ri.started_at, ri.finished_at,
	   ri.created_at, ri.updated_at, ri.generated_output, ri.generated_context, ri.generated_at,
       di.dataset_id, di.external_id, di.input, di.output, di.expected_output, di.context, di.metadata,
       di.created_at AS item_created_at, di.updated_at AS item_updated_at
FROM eval_run_items ri
JOIN dataset_items di ON di.id = ri.dataset_item_id
WHERE ri.eval_run_id = sqlc.arg(eval_run_id)
  AND (NOT sqlc.arg(has_cursor)::boolean
       OR (ri.created_at, ri.dataset_item_id) > (
           sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::uuid
       ))
ORDER BY ri.created_at, ri.dataset_item_id
LIMIT sqlc.arg(page_size);

-- name: ListPendingEvalRunItems :many
SELECT ri.eval_run_id, ri.dataset_item_id, ri.status, ri.error, ri.started_at, ri.finished_at,
	   ri.created_at, ri.updated_at, ri.generated_output, ri.generated_context, ri.generated_at,
       di.dataset_id, di.external_id, di.input, di.output, di.expected_output, di.context, di.metadata,
       di.created_at AS item_created_at, di.updated_at AS item_updated_at
FROM eval_run_items ri
JOIN dataset_items di ON di.id = ri.dataset_item_id
WHERE ri.eval_run_id = $1 AND ri.status = 'pending'
ORDER BY ri.created_at, ri.dataset_item_id;

-- name: StartEvalRun :one
WITH owned_job AS MATERIALIZED (
    SELECT j.id, j.eval_run_id
    FROM jobs j
    WHERE j.id = sqlc.arg(job_id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
      AND j.eval_run_id = sqlc.arg(selected_run_id)
    FOR UPDATE
)
UPDATE eval_runs er
SET status = 'running', started_at = coalesce(started_at, now()), updated_at = now(), error = NULL
FROM owned_job
WHERE er.id = owned_job.eval_run_id AND er.status IN ('pending', 'running')
RETURNING er.id, application_id, dataset_id, name, status, mode, params, scorers, aggregates,
           total_items, succeeded_items, failed_items, canceled_items, started_at, finished_at,
           error, created_at, updated_at;

-- name: MarkEvalRunItemRunning :one
WITH owned_job AS MATERIALIZED (
    SELECT j.id, j.eval_run_id
    FROM jobs j
    WHERE j.id = sqlc.arg(job_id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
      AND j.eval_run_id = sqlc.arg(selected_run_id)
    FOR UPDATE
)
UPDATE eval_run_items eri
SET status = 'running', started_at = coalesce(started_at, now()), updated_at = now(), error = NULL
FROM owned_job
WHERE eri.eval_run_id = owned_job.eval_run_id
  AND eri.dataset_item_id = sqlc.arg(selected_item_id)
  AND eri.status = 'pending'
  AND EXISTS (SELECT 1 FROM eval_runs er WHERE er.id = eri.eval_run_id AND er.status = 'running')
RETURNING eri.eval_run_id;

-- name: ResetEvalRunItemPending :one
WITH owned_job AS MATERIALIZED (
    SELECT j.id, j.eval_run_id
    FROM jobs j
    WHERE j.id = sqlc.arg(job_id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
      AND j.eval_run_id = sqlc.arg(selected_run_id)
    FOR UPDATE
)
UPDATE eval_run_items eri
SET status = 'pending', updated_at = now(), error = sqlc.narg(error)
FROM owned_job
WHERE eri.eval_run_id = owned_job.eval_run_id
  AND eri.dataset_item_id = sqlc.arg(selected_item_id)
  AND eri.status = 'running'
  AND EXISTS (SELECT 1 FROM eval_runs er WHERE er.id = eri.eval_run_id AND er.status = 'running')
RETURNING eri.eval_run_id;

-- name: FailEvalRunItem :one
WITH owned_job AS MATERIALIZED (
    SELECT j.id, j.eval_run_id
    FROM jobs j
    WHERE j.id = sqlc.arg(job_id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
      AND j.eval_run_id = sqlc.arg(selected_run_id)
    FOR UPDATE
)
UPDATE eval_run_items eri
SET status = 'failed', error = sqlc.arg(error), finished_at = now(), updated_at = now()
FROM owned_job
WHERE eri.eval_run_id = owned_job.eval_run_id
  AND eri.dataset_item_id = sqlc.arg(selected_item_id)
  AND eri.status = 'running'
  AND EXISTS (SELECT 1 FROM eval_runs er WHERE er.id = eri.eval_run_id AND er.status = 'running')
RETURNING eri.eval_run_id;

-- name: DeleteEvalRunItemScores :exec
DELETE FROM scores WHERE eval_run_id = $1 AND dataset_item_id = $2;

-- name: SaveEvalRunItemGeneration :one
WITH owned_job AS MATERIALIZED (
    SELECT j.id, j.eval_run_id
    FROM jobs j
    WHERE j.id = sqlc.arg(job_id) AND j.status = 'running'
      AND j.kind = 'eval_run' AND j.locked_by = sqlc.arg(worker_id)
      AND j.lease_expires_at > now() AND j.eval_run_id = sqlc.arg(selected_run_id)
    FOR UPDATE
)
UPDATE eval_run_items eri
SET generated_output = sqlc.arg(generated_output),
    generated_context = sqlc.arg(generated_context), generated_at = now(), updated_at = now()
FROM owned_job
WHERE eri.eval_run_id = owned_job.eval_run_id
  AND eri.dataset_item_id = sqlc.arg(selected_item_id) AND eri.status = 'running'
  AND EXISTS (SELECT 1 FROM eval_runs er WHERE er.id = eri.eval_run_id AND er.status = 'running')
RETURNING eri.eval_run_id;

-- name: InsertOfflineScore :one
INSERT INTO scores (
    scorer, scorer_config_id, value, threshold, passed, rationale, details,
    prompt_template_id, judge_model, judge_provider, judge_tokens, eval_run_id, dataset_item_id
)
VALUES (
    sqlc.arg(scorer), sqlc.narg(scorer_config_id), sqlc.arg(value), sqlc.arg(threshold),
    sqlc.arg(passed), sqlc.arg(rationale), sqlc.arg(details), sqlc.arg(prompt_template_id),
    sqlc.arg(judge_model), sqlc.arg(judge_provider), sqlc.arg(judge_tokens),
    sqlc.arg(eval_run_id), sqlc.arg(dataset_item_id)
)
RETURNING id, scorer, scorer_config_id, value, threshold, passed, rationale, details,
          prompt_template_id, judge_model, judge_provider, judge_tokens, eval_run_id,
          dataset_item_id, created_at;

-- name: CompleteEvalRunItem :one
WITH owned_job AS MATERIALIZED (
    SELECT j.id, j.eval_run_id
    FROM jobs j
    WHERE j.id = sqlc.arg(job_id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
      AND j.eval_run_id = sqlc.arg(selected_run_id)
    FOR UPDATE
)
UPDATE eval_run_items eri
SET status = 'succeeded', error = NULL, finished_at = now(), updated_at = now()
FROM owned_job
WHERE eri.eval_run_id = owned_job.eval_run_id
  AND eri.dataset_item_id = sqlc.arg(selected_item_id)
  AND eri.status = 'running'
  AND EXISTS (SELECT 1 FROM eval_runs er WHERE er.id = eri.eval_run_id AND er.status = 'running')
RETURNING eri.eval_run_id;

-- name: CancelEvalRun :one
WITH locked_job AS MATERIALIZED (
    SELECT j.id
    FROM jobs j
    WHERE j.eval_run_id = sqlc.arg(selected_id)
    FOR UPDATE
),
canceled_run AS (
    UPDATE eval_runs er
    SET status = 'canceled',
        succeeded_items = (
            SELECT count(*)::integer FROM eval_run_items
            WHERE eval_run_id = er.id AND status = 'succeeded'
        ),
        failed_items = (
            SELECT count(*)::integer FROM eval_run_items
            WHERE eval_run_id = er.id AND status = 'failed'
        ),
        canceled_items = total_items - (
            SELECT count(*)::integer FROM eval_run_items
            WHERE eval_run_id = er.id AND status IN ('succeeded', 'failed')
        ),
        finished_at = now(), updated_at = now()
    FROM locked_job
    WHERE er.id = sqlc.arg(selected_id) AND er.status IN ('pending', 'running')
    RETURNING er.id, application_id, dataset_id, name, status, mode, params, scorers, aggregates,
              total_items, succeeded_items, failed_items, canceled_items, started_at, finished_at,
              error, created_at, updated_at
), canceled_items AS (
    UPDATE eval_run_items eri
    SET status = 'canceled', finished_at = now(), updated_at = now()
    FROM canceled_run
    WHERE eri.eval_run_id = canceled_run.id AND eri.status IN ('pending', 'running')
), canceled_job AS (
    UPDATE jobs j
    SET status = 'canceled', locked_by = NULL, locked_at = NULL, lease_expires_at = NULL,
        updated_at = now()
    FROM locked_job, canceled_run
    WHERE j.id = locked_job.id AND j.status IN ('pending', 'running')
)
SELECT * FROM canceled_run;

-- name: ListEvalRunScores :many
SELECT id, scorer, scorer_config_id, value, threshold, passed, rationale, details,
	   prompt_template_id, judge_model, judge_provider, judge_tokens, eval_run_id,
	   dataset_item_id, created_at, trace_id, span_id, span_start_time, judged_input,
	   judged_output, judged_context, judged_reference
FROM scores
WHERE eval_run_id = sqlc.arg(eval_run_id)
  AND (NOT sqlc.arg(has_cursor)::boolean
       OR (created_at, id) > (sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::bigint))
ORDER BY created_at, id
LIMIT sqlc.arg(page_size);
