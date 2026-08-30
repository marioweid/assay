-- +goose Up
CREATE TABLE traces (
    id uuid PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    otel_trace_id bytea NOT NULL CHECK (octet_length(otel_trace_id) = 16),
    root_name text NOT NULL,
    start_time timestamptz NOT NULL,
    end_time timestamptz NOT NULL,
    status text NOT NULL,
    span_count integer NOT NULL DEFAULT 0 CHECK (span_count >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    total_cost numeric,
    reference_answer text,
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, application_id),
    UNIQUE (application_id, otel_trace_id),
    CHECK (end_time >= start_time)
);

CREATE TABLE spans (
    id bigint GENERATED ALWAYS AS IDENTITY,
    trace_id uuid NOT NULL,
    application_id uuid NOT NULL,
    otel_span_id bytea NOT NULL CHECK (octet_length(otel_span_id) = 8),
    parent_span_id bytea CHECK (parent_span_id IS NULL OR octet_length(parent_span_id) = 8),
    name text NOT NULL,
    kind text NOT NULL,
    operation_name text NOT NULL DEFAULT '',
    start_time timestamptz NOT NULL,
    end_time timestamptz NOT NULL,
    duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
    status_code text NOT NULL,
    status_message text NOT NULL DEFAULT '',
    is_scorable boolean NOT NULL DEFAULT false,
    scorable_kind text NOT NULL DEFAULT '',
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    events jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(events) = 'array'),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    reference_answer text,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, start_time),
    UNIQUE (trace_id, otel_span_id, start_time),
    FOREIGN KEY (trace_id, application_id)
        REFERENCES traces(id, application_id) ON DELETE CASCADE,
    CHECK (end_time >= start_time)
) PARTITION BY RANGE (start_time);

CREATE TABLE spans_default PARTITION OF spans DEFAULT;

CREATE INDEX traces_application_start_idx ON traces(application_id, start_time DESC, id DESC);
CREATE INDEX spans_trace_id_idx ON spans(trace_id);
CREATE INDEX spans_application_start_idx ON spans(application_id, start_time DESC, id DESC);

-- +goose Down
DROP TABLE spans;
DROP TABLE traces;
