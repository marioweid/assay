-- +goose Up
ALTER TABLE eval_runs DROP CONSTRAINT eval_runs_mode_check;
ALTER TABLE eval_runs ADD CONSTRAINT eval_runs_mode_check
    CHECK (mode IN ('score_existing', 'generate_then_score'));

ALTER TABLE eval_run_items
    ADD COLUMN generated_output text,
    ADD COLUMN generated_context jsonb,
    ADD COLUMN generated_at timestamptz,
    ADD CONSTRAINT eval_run_items_generated_context_check
        CHECK (generated_context IS NULL OR jsonb_typeof(generated_context) = 'array');

ALTER TABLE jobs ALTER COLUMN eval_run_id DROP NOT NULL;
ALTER TABLE jobs DROP CONSTRAINT jobs_eval_run_id_key;
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs
    ADD COLUMN trace_id uuid REFERENCES traces(id) ON DELETE CASCADE,
    ADD COLUMN scorer text CHECK (scorer IN ('groundedness', 'correctness')),
    ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('eval_run', 'scoring_task')),
    ADD CONSTRAINT jobs_target_check CHECK (
        (kind = 'eval_run' AND eval_run_id IS NOT NULL AND trace_id IS NULL AND scorer IS NULL)
        OR
        (kind = 'scoring_task' AND eval_run_id IS NULL AND trace_id IS NOT NULL AND scorer IS NOT NULL)
    );

CREATE UNIQUE INDEX jobs_eval_run_idx ON jobs(eval_run_id) WHERE kind = 'eval_run';
CREATE UNIQUE INDEX jobs_trace_scorer_idx ON jobs(trace_id, scorer) WHERE kind = 'scoring_task';

ALTER TABLE scores DROP CONSTRAINT scores_eval_run_id_dataset_item_id_fkey;
ALTER TABLE scores
    ALTER COLUMN eval_run_id DROP NOT NULL,
    ALTER COLUMN dataset_item_id DROP NOT NULL,
    ADD COLUMN trace_id uuid REFERENCES traces(id) ON DELETE CASCADE,
    ADD COLUMN span_id bigint,
    ADD COLUMN span_start_time timestamptz,
    ADD COLUMN judged_input text,
    ADD COLUMN judged_output text,
    ADD COLUMN judged_context jsonb,
    ADD COLUMN judged_reference text,
    ADD CONSTRAINT scores_judged_context_check
        CHECK (judged_context IS NULL OR jsonb_typeof(judged_context) = 'array'),
    ADD CONSTRAINT scores_offline_target_fkey
        FOREIGN KEY (eval_run_id, dataset_item_id)
        REFERENCES eval_run_items(eval_run_id, dataset_item_id) ON DELETE CASCADE,
    ADD CONSTRAINT scores_target_check CHECK (
        (
            eval_run_id IS NOT NULL AND dataset_item_id IS NOT NULL
            AND trace_id IS NULL AND span_id IS NULL AND span_start_time IS NULL
            AND judged_input IS NULL AND judged_output IS NULL
            AND judged_context IS NULL AND judged_reference IS NULL
        )
        OR
        (
            eval_run_id IS NULL AND dataset_item_id IS NULL
            AND trace_id IS NOT NULL AND span_id IS NOT NULL AND span_start_time IS NOT NULL
            AND judged_input IS NOT NULL AND judged_output IS NOT NULL
            AND judged_context IS NOT NULL
        )
    );

CREATE INDEX scores_trace_cursor_idx ON scores(trace_id, created_at, id)
    WHERE trace_id IS NOT NULL;

-- +goose Down
DELETE FROM eval_runs WHERE mode = 'generate_then_score';
DELETE FROM jobs WHERE kind = 'scoring_task';
DELETE FROM scores WHERE trace_id IS NOT NULL;

DROP INDEX scores_trace_cursor_idx;
ALTER TABLE scores
    DROP CONSTRAINT scores_target_check,
    DROP CONSTRAINT scores_offline_target_fkey,
    DROP CONSTRAINT scores_judged_context_check,
    DROP COLUMN judged_reference,
    DROP COLUMN judged_context,
    DROP COLUMN judged_output,
    DROP COLUMN judged_input,
    DROP COLUMN span_start_time,
    DROP COLUMN span_id,
    DROP COLUMN trace_id,
    ALTER COLUMN dataset_item_id SET NOT NULL,
    ALTER COLUMN eval_run_id SET NOT NULL,
    ADD CONSTRAINT scores_eval_run_id_dataset_item_id_fkey
        FOREIGN KEY (eval_run_id, dataset_item_id)
        REFERENCES eval_run_items(eval_run_id, dataset_item_id) ON DELETE CASCADE;

DROP INDEX jobs_trace_scorer_idx;
DROP INDEX jobs_eval_run_idx;
ALTER TABLE jobs
    DROP CONSTRAINT jobs_target_check,
    DROP CONSTRAINT jobs_kind_check,
    DROP COLUMN scorer,
    DROP COLUMN trace_id,
    ALTER COLUMN eval_run_id SET NOT NULL,
    ADD CONSTRAINT jobs_kind_check CHECK (kind = 'eval_run'),
    ADD CONSTRAINT jobs_eval_run_id_key UNIQUE (eval_run_id);

ALTER TABLE eval_run_items
    DROP CONSTRAINT eval_run_items_generated_context_check,
    DROP COLUMN generated_at,
    DROP COLUMN generated_context,
    DROP COLUMN generated_output;

ALTER TABLE eval_runs DROP CONSTRAINT eval_runs_mode_check;
ALTER TABLE eval_runs ADD CONSTRAINT eval_runs_mode_check CHECK (mode = 'score_existing');
