-- name: ListProjectSessions :many
WITH sessions AS (
    SELECT t.session_id, min(t.start_time) AS start_time,
           max(t.end_time) AS end_time, count(*)::bigint AS turn_count,
           (array_agg(t.root_name ORDER BY t.start_time, t.id))[1] AS first_operation,
           (array_agg(t.id ORDER BY t.end_time DESC, t.id DESC))[1] AS last_trace_id
    FROM traces t
    JOIN applications a ON a.id = t.application_id
    WHERE a.project_id = sqlc.arg(project_id)
      AND t.application_id = sqlc.arg(application_id)
      AND t.session_id IS NOT NULL
      AND t.end_time <= sqlc.arg(anchor_time)::timestamptz
    GROUP BY t.session_id
)
SELECT session_id::text AS session_id, start_time::timestamptz AS start_time,
       end_time::timestamptz AS end_time, turn_count::bigint AS turn_count,
       first_operation::text AS first_operation, last_trace_id::uuid AS last_trace_id
FROM sessions
WHERE NOT sqlc.arg(has_cursor)::boolean OR (end_time, session_id) < (
    sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::text
)
ORDER BY end_time DESC, session_id DESC
LIMIT sqlc.arg(page_size);

-- name: ListRecentSessionTurns :many
SELECT t.id, t.root_name, t.start_time, t.end_time, t.status, t.span_count,
       t.total_tokens, t.attributes
FROM traces t
JOIN applications a ON a.id = t.application_id
WHERE a.project_id = sqlc.arg(project_id)
  AND t.application_id = sqlc.arg(application_id)
  AND t.session_id = sqlc.arg(session_id)
ORDER BY t.start_time DESC, t.id DESC
LIMIT sqlc.arg(page_size);

-- name: ListProjectSessionTurns :many
SELECT t.id, t.root_name, t.start_time, t.end_time, t.status, t.span_count,
       t.total_tokens, t.attributes
FROM traces t
JOIN applications a ON a.id = t.application_id
WHERE a.project_id = sqlc.arg(project_id)
  AND t.application_id = sqlc.arg(application_id)
  AND t.session_id = sqlc.arg(session_id)
ORDER BY t.start_time, t.id
LIMIT sqlc.arg(page_size);

-- name: ListProjectSessionTurnsAfterCursor :many
SELECT t.id, t.root_name, t.start_time, t.end_time, t.status, t.span_count,
       t.total_tokens, t.attributes
FROM traces t
JOIN applications a ON a.id = t.application_id
WHERE a.project_id = sqlc.arg(project_id)
  AND t.application_id = sqlc.arg(application_id)
  AND t.session_id = sqlc.arg(session_id)
  AND (t.start_time, t.id) > (
      sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::uuid
  )
ORDER BY t.start_time, t.id
LIMIT sqlc.arg(page_size);
