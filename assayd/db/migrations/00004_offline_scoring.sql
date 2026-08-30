-- +goose Up
CREATE TABLE datasets (
    id uuid PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, name),
    UNIQUE (id, application_id)
);

CREATE TABLE dataset_items (
    id uuid PRIMARY KEY,
    dataset_id uuid NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    external_id text,
    input jsonb NOT NULL CHECK (jsonb_typeof(input) = 'object'),
    output text,
    expected_output text,
    context jsonb CHECK (context IS NULL OR jsonb_typeof(context) = 'array'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX dataset_items_external_id_idx
    ON dataset_items(dataset_id, external_id) WHERE external_id IS NOT NULL;
CREATE INDEX dataset_items_dataset_cursor_idx ON dataset_items(dataset_id, created_at, id);

CREATE TABLE scorer_configs (
    id uuid PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    scorer text NOT NULL CHECK (scorer IN ('groundedness', 'correctness')),
    enabled boolean NOT NULL DEFAULT true,
    threshold numeric NOT NULL DEFAULT 0.5 CHECK (threshold >= 0 AND threshold <= 1),
    judge_config jsonb CHECK (judge_config IS NULL OR jsonb_typeof(judge_config) = 'object'),
    prompt_template_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, scorer)
);

CREATE TABLE eval_runs (
    id uuid PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    dataset_id uuid NOT NULL,
    name text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    mode text NOT NULL CHECK (mode = 'score_existing'),
    params jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(params) = 'object'),
    scorers text[] NOT NULL CHECK (cardinality(scorers) > 0),
    aggregates jsonb CHECK (aggregates IS NULL OR jsonb_typeof(aggregates) = 'object'),
    total_items integer NOT NULL DEFAULT 0 CHECK (total_items >= 0),
    succeeded_items integer NOT NULL DEFAULT 0 CHECK (succeeded_items >= 0),
    failed_items integer NOT NULL DEFAULT 0 CHECK (failed_items >= 0),
    canceled_items integer NOT NULL DEFAULT 0 CHECK (canceled_items >= 0),
    started_at timestamptz,
    finished_at timestamptz,
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, application_id),
    FOREIGN KEY (dataset_id, application_id)
        REFERENCES datasets(id, application_id) ON DELETE CASCADE
);

CREATE INDEX eval_runs_application_cursor_idx
    ON eval_runs(application_id, created_at, id);

CREATE TABLE eval_run_items (
    eval_run_id uuid NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    dataset_item_id uuid NOT NULL REFERENCES dataset_items(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    error text,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (eval_run_id, dataset_item_id)
);

CREATE INDEX eval_run_items_cursor_idx
    ON eval_run_items(eval_run_id, created_at, dataset_item_id);

CREATE TABLE scores (
    id bigint GENERATED ALWAYS AS IDENTITY,
    scorer text NOT NULL CHECK (scorer IN ('groundedness', 'correctness')),
    scorer_config_id uuid REFERENCES scorer_configs(id) ON DELETE SET NULL,
    value numeric NOT NULL CHECK (value >= 0 AND value <= 1),
    threshold numeric NOT NULL CHECK (threshold >= 0 AND threshold <= 1),
    passed boolean NOT NULL,
    rationale text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    prompt_template_id text NOT NULL,
    judge_model text NOT NULL,
    judge_provider text NOT NULL,
    judge_tokens integer NOT NULL DEFAULT 0 CHECK (judge_tokens >= 0),
    eval_run_id uuid NOT NULL,
    dataset_item_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at),
    FOREIGN KEY (eval_run_id, dataset_item_id)
        REFERENCES eval_run_items(eval_run_id, dataset_item_id) ON DELETE CASCADE
) PARTITION BY RANGE (created_at);

CREATE TABLE scores_default PARTITION OF scores DEFAULT;
CREATE INDEX scores_eval_run_cursor_idx ON scores(eval_run_id, created_at, id);

CREATE TABLE jobs (
    id uuid PRIMARY KEY,
    kind text NOT NULL CHECK (kind = 'eval_run'),
    eval_run_id uuid NOT NULL UNIQUE REFERENCES eval_runs(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    run_after timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL CHECK (max_attempts > 0),
    locked_by text,
    locked_at timestamptz,
    lease_expires_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jobs_pending_run_after_idx ON jobs(run_after, id) WHERE status = 'pending';

-- +goose Down
DROP TABLE jobs;
DROP TABLE scores;
DROP TABLE eval_run_items;
DROP TABLE eval_runs;
DROP TABLE scorer_configs;
DROP TABLE dataset_items;
DROP TABLE datasets;
