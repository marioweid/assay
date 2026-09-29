-- +goose Up
ALTER TABLE traces ADD COLUMN session_id text
    CHECK (session_id IS NULL OR (length(session_id) BETWEEN 1 AND 128
        AND session_id = btrim(session_id) AND session_id !~ '[[:cntrl:]]'));

-- Only a real root span can establish session membership, even for old partial exports.
UPDATE traces AS t
SET session_id = (
    SELECT CASE WHEN jsonb_typeof(s.attributes->'session.id') = 'string'
        AND length(s.attributes->>'session.id') BETWEEN 1 AND 128
        AND s.attributes->>'session.id' = btrim(s.attributes->>'session.id')
        AND s.attributes->>'session.id' !~ '[[:cntrl:]]'
        THEN s.attributes->>'session.id' END
    FROM spans s
    WHERE s.trace_id = t.id AND s.parent_span_id IS NULL
    ORDER BY s.start_time, s.id
    LIMIT 1
)
WHERE EXISTS (
    SELECT 1 FROM spans s
    WHERE s.trace_id = t.id AND s.parent_span_id IS NULL
      AND s.attributes ? 'session.id'
);

CREATE INDEX traces_session_activity_idx
    ON traces (application_id, session_id, end_time DESC, id DESC)
    WHERE session_id IS NOT NULL;

-- Bounds both recent-first and cursor-forward turn reads, even for very long sessions.
CREATE INDEX traces_session_turn_idx
    ON traces (application_id, session_id, start_time, id)
    WHERE session_id IS NOT NULL;

-- +goose Down
DROP INDEX traces_session_turn_idx;
DROP INDEX traces_session_activity_idx;
ALTER TABLE traces DROP COLUMN session_id;
