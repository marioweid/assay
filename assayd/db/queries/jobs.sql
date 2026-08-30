-- name: CreateEvalRunJob :one
INSERT INTO jobs (id, kind, eval_run_id, status, max_attempts)
VALUES ($1, 'eval_run', $2, 'pending', $3)
RETURNING *;

-- name: LockJobTableForWrite :exec
LOCK TABLE jobs IN ROW EXCLUSIVE MODE;

-- name: LockJobTableForDelete :exec
LOCK TABLE jobs IN EXCLUSIVE MODE;

-- name: ClaimJob :one
WITH candidate AS (
    SELECT id
    FROM jobs
    WHERE status = 'pending' AND run_after <= now() AND attempts < max_attempts
    ORDER BY run_after ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE jobs j
SET status = 'running', attempts = attempts + 1, locked_by = sqlc.arg(worker_id),
    locked_at = now(),
    lease_expires_at = now() + (sqlc.arg(lease_seconds)::double precision * interval '1 second'),
    updated_at = now()
FROM candidate
WHERE j.id = candidate.id
RETURNING j.*;

-- name: HeartbeatJob :one
UPDATE jobs
SET lease_expires_at = now() + (sqlc.arg(lease_seconds)::double precision * interval '1 second'),
    updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'running' AND locked_by = sqlc.arg(worker_id)
  AND lease_expires_at > now()
RETURNING id;

-- name: LockOwnedJob :one
SELECT id
FROM jobs
WHERE id = sqlc.arg(id) AND status = 'running' AND locked_by = sqlc.arg(worker_id)
  AND lease_expires_at > now()
FOR UPDATE;

-- name: CompleteJob :one
WITH owned_job AS MATERIALIZED (
    SELECT j.id, j.eval_run_id
    FROM jobs j
    WHERE j.id = sqlc.arg(id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
    FOR UPDATE
),
aggregates AS (
    SELECT
        COALESCE(jsonb_object_agg(scorer, aggregate), '{}'::jsonb) AS value
    FROM (
        SELECT
            scorer,
            jsonb_build_object(
                'n', count(*),
                'mean', avg(value),
                'pass_rate', avg(CASE WHEN passed THEN 1.0 ELSE 0.0 END)
            ) AS aggregate
        FROM scores
        WHERE eval_run_id = (SELECT eval_run_id FROM owned_job)
        GROUP BY scorer
    ) scorer_aggregates
),
finalized_run AS (
    UPDATE eval_runs er
    SET status = CASE
            WHEN EXISTS (
                SELECT 1 FROM eval_run_items eri
                WHERE eri.eval_run_id = er.id AND eri.status = 'succeeded'
            ) THEN 'succeeded'
            ELSE 'failed'
        END,
        aggregates = aggregates.value,
        succeeded_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'succeeded'
        ),
        failed_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'failed'
        ),
        canceled_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'canceled'
        ),
        finished_at = now(), updated_at = now()
    FROM owned_job, aggregates
    WHERE er.id = owned_job.eval_run_id AND er.status = 'running'
      AND NOT EXISTS (
          SELECT 1 FROM eval_run_items eri
          WHERE eri.eval_run_id = er.id AND eri.status IN ('pending', 'running')
      )
    RETURNING er.id
)
UPDATE jobs j
SET status = 'succeeded', locked_by = NULL, locked_at = NULL, lease_expires_at = NULL,
    updated_at = now()
FROM owned_job, finalized_run
WHERE j.id = owned_job.id
RETURNING j.id;

-- name: RetryJob :one
WITH retried_job AS (
    UPDATE jobs j
    SET status = 'pending', run_after = now() + (
            sqlc.arg(delay_seconds)::double precision * interval '1 second'
        ),
        last_error = sqlc.arg(last_error), locked_by = NULL, locked_at = NULL,
        lease_expires_at = NULL, updated_at = now()
    WHERE j.id = sqlc.arg(id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
    RETURNING j.id, j.eval_run_id
),
reset_items AS (
    UPDATE eval_run_items eri
    SET status = 'pending', started_at = NULL, error = NULL, updated_at = now()
    FROM retried_job
    WHERE eri.eval_run_id = retried_job.eval_run_id AND eri.status = 'running'
)
SELECT id FROM retried_job;

-- name: ExhaustJob :one
WITH exhausted_job AS (
    UPDATE jobs j
    SET status = 'failed', last_error = sqlc.arg(last_error), locked_by = NULL,
        locked_at = NULL, lease_expires_at = NULL, updated_at = now()
    WHERE j.id = sqlc.arg(job_id) AND j.status = 'running'
      AND j.locked_by = sqlc.arg(worker_id) AND j.lease_expires_at > now()
    RETURNING j.id, j.eval_run_id
),
failed_run AS (
    UPDATE eval_runs er
    SET status = 'failed', error = sqlc.arg(last_error),
        succeeded_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'succeeded'
        ),
        failed_items = er.total_items - (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status IN ('succeeded', 'canceled')
        ),
        canceled_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'canceled'
        ),
        finished_at = now(), updated_at = now()
    FROM exhausted_job
    WHERE er.id = exhausted_job.eval_run_id AND er.status IN ('pending', 'running')
    RETURNING er.id
),
failed_items AS (
    UPDATE eval_run_items eri
    SET status = 'failed', error = sqlc.arg(last_error), finished_at = now(), updated_at = now()
    FROM failed_run
    WHERE eri.eval_run_id = failed_run.id AND eri.status IN ('pending', 'running')
)
SELECT id FROM exhausted_job;

-- name: ReapExpiredJobs :one
WITH candidates AS MATERIALIZED (
    SELECT id
    FROM jobs
    WHERE status = 'running' AND lease_expires_at < now()
    ORDER BY id
    FOR UPDATE SKIP LOCKED
),
expired AS (
    UPDATE jobs
    SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
        run_after = now(),
        last_error = CASE
            WHEN attempts >= max_attempts THEN 'job lease expired after maximum attempts'
            ELSE last_error
        END,
        locked_by = NULL, locked_at = NULL, lease_expires_at = NULL, updated_at = now()
    WHERE id IN (SELECT id FROM candidates)
    RETURNING id, eval_run_id, attempts, max_attempts
),
failed_runs AS (
    UPDATE eval_runs er
    SET status = 'failed', error = 'job lease expired after maximum attempts',
        succeeded_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'succeeded'
        ),
        failed_items = er.total_items - (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status IN ('succeeded', 'canceled')
        ),
        canceled_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'canceled'
        ),
        finished_at = now(), updated_at = now()
    FROM expired
    WHERE er.id = expired.eval_run_id AND expired.attempts >= expired.max_attempts
      AND er.status IN ('pending', 'running')
    RETURNING er.id
),
failed_items AS (
    UPDATE eval_run_items eri
    SET status = 'failed', error = 'job lease expired after maximum attempts',
        finished_at = now(), updated_at = now()
    FROM failed_runs
    WHERE eri.eval_run_id = failed_runs.id AND eri.status IN ('pending', 'running')
),
reset_items AS (
    UPDATE eval_run_items eri
    SET status = 'pending', started_at = NULL, error = NULL, updated_at = now()
    FROM expired
    WHERE eri.eval_run_id = expired.eval_run_id
      AND expired.attempts < expired.max_attempts AND eri.status = 'running'
)
SELECT
    (SELECT count(*) FROM expired)::integer AS reaped_jobs,
    (SELECT count(*) FROM failed_items)::integer AS failed_items,
    (SELECT count(*) FROM reset_items)::integer AS reset_items;

-- name: ReleaseWorkerJobs :one
WITH candidates AS MATERIALIZED (
    SELECT j.id
    FROM jobs j
    WHERE j.status = 'running' AND j.locked_by = sqlc.arg(worker_id)
    ORDER BY j.id
    FOR UPDATE OF j
),
released AS (
    UPDATE jobs
    SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
        run_after = now(),
        last_error = CASE
            WHEN attempts >= max_attempts THEN 'worker stopped after maximum attempts'
            ELSE last_error
        END,
        locked_by = NULL, locked_at = NULL,
        lease_expires_at = NULL, updated_at = now()
    WHERE id IN (SELECT id FROM candidates)
    RETURNING eval_run_id, attempts, max_attempts
),
failed_runs AS (
    UPDATE eval_runs er
    SET status = 'failed', error = 'worker stopped after maximum attempts',
        succeeded_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'succeeded'
        ),
        failed_items = er.total_items - (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status IN ('succeeded', 'canceled')
        ),
        canceled_items = (
            SELECT count(*) FROM eval_run_items eri
            WHERE eri.eval_run_id = er.id AND eri.status = 'canceled'
        ),
        finished_at = now(), updated_at = now()
    FROM released
    WHERE er.id = released.eval_run_id AND released.attempts >= released.max_attempts
      AND er.status IN ('pending', 'running')
    RETURNING er.id
),
failed_items AS (
    UPDATE eval_run_items eri
    SET status = 'failed', error = 'worker stopped after maximum attempts',
        finished_at = now(), updated_at = now()
    FROM failed_runs
    WHERE eri.eval_run_id = failed_runs.id AND eri.status IN ('pending', 'running')
),
reset_items AS (
    UPDATE eval_run_items eri
    SET status = 'pending', started_at = NULL, error = NULL, updated_at = now()
    FROM released
    WHERE eri.eval_run_id = released.eval_run_id
      AND released.attempts < released.max_attempts AND eri.status = 'running'
)
SELECT
    (SELECT count(*) FROM released)::integer AS released_jobs,
    (SELECT count(*) FROM failed_items)::integer AS failed_items,
    (SELECT count(*) FROM reset_items)::integer AS reset_items;
