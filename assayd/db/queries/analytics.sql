-- name: ApplicationMetrics :many
SELECT date_trunc('day', s.created_at, 'UTC')::timestamptz AS day,
       s.scorer, avg(s.value)::float8 AS mean,
       avg(CASE WHEN s.passed THEN 1.0 ELSE 0.0 END)::float8 AS pass_rate,
       count(*)::bigint AS n
FROM scores s
LEFT JOIN traces t ON t.id = s.trace_id
LEFT JOIN eval_runs r ON r.id = s.eval_run_id
WHERE coalesce(t.application_id, r.application_id) = sqlc.arg(application_id)::uuid
  AND s.created_at >= sqlc.arg(start_time) AND s.created_at < sqlc.arg(end_time)
  AND (sqlc.arg(scorer)::text = '' OR s.scorer = sqlc.arg(scorer))
GROUP BY day, s.scorer
ORDER BY day, s.scorer;

-- name: ListApplicationScores :many
SELECT s.* FROM scores s
LEFT JOIN traces t ON t.id = s.trace_id
LEFT JOIN eval_runs r ON r.id = s.eval_run_id
WHERE coalesce(t.application_id, r.application_id) = sqlc.arg(application_id)::uuid
  AND s.created_at >= sqlc.arg(start_time) AND s.created_at < sqlc.arg(end_time)
  AND (sqlc.arg(scorer)::text = '' OR s.scorer = sqlc.arg(scorer))
  AND (NOT sqlc.arg(filter_passed)::boolean OR s.passed = sqlc.arg(passed)::boolean)
  AND (NOT sqlc.arg(has_cursor)::boolean
       OR (s.created_at, s.id) > (sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::bigint))
ORDER BY s.created_at, s.id LIMIT sqlc.arg(page_size);
