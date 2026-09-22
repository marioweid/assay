-- name: CompareEvalRuns :many
WITH baseline_scores AS (
    SELECT DISTINCT ON (score.dataset_item_id)
        score.id,
        score.created_at,
        score.dataset_item_id,
        score.value::double precision AS value,
        score.threshold::double precision AS threshold,
        score.prompt_template_id,
        score.judge_model,
        score.judge_provider
    FROM scores AS score
    WHERE score.eval_run_id = sqlc.arg(baseline_id) AND score.scorer = sqlc.arg(scorer)
    ORDER BY score.dataset_item_id, score.created_at DESC, score.id DESC
), candidate_scores AS (
    SELECT DISTINCT ON (score.dataset_item_id)
        score.id,
        score.created_at,
        score.dataset_item_id,
        score.value::double precision AS value,
        score.threshold::double precision AS threshold,
        score.prompt_template_id,
        score.judge_model,
        score.judge_provider
    FROM scores AS score
    WHERE score.eval_run_id = sqlc.arg(candidate_id) AND score.scorer = sqlc.arg(scorer)
    ORDER BY score.dataset_item_id, score.created_at DESC, score.id DESC
), baseline_items AS (
    SELECT
        item.eval_run_id,
        item.dataset_item_id,
        item.status,
        item.snapshot_input,
        item.snapshot_expected_output,
        item.snapshot_context,
        score.id AS selected_score_id,
        score.created_at AS selected_score_created_at,
        score.value AS selected_score_value,
        score.threshold AS selected_score_threshold,
        score.prompt_template_id AS selected_prompt_template_id,
        score.judge_model AS selected_judge_model,
        score.judge_provider AS selected_judge_provider
    FROM eval_run_items AS item
    LEFT JOIN baseline_scores AS score USING (dataset_item_id)
    WHERE item.eval_run_id = sqlc.arg(baseline_id)
), candidate_items AS (
    SELECT
        item.eval_run_id,
        item.dataset_item_id,
        item.status,
        item.snapshot_input,
        item.snapshot_expected_output,
        item.snapshot_context,
        score.id AS selected_score_id,
        score.created_at AS selected_score_created_at,
        score.value AS selected_score_value,
        score.threshold AS selected_score_threshold,
        score.prompt_template_id AS selected_prompt_template_id,
        score.judge_model AS selected_judge_model,
        score.judge_provider AS selected_judge_provider
    FROM eval_run_items AS item
    LEFT JOIN candidate_scores AS score USING (dataset_item_id)
    WHERE item.eval_run_id = sqlc.arg(candidate_id)
), paired AS (
    SELECT
        coalesce(baseline.dataset_item_id, candidate.dataset_item_id) AS dataset_item_id,
        baseline.selected_score_id AS baseline_score_id,
        baseline.selected_score_created_at AS baseline_score_created_at,
        candidate.selected_score_id AS candidate_score_id,
        candidate.selected_score_created_at AS candidate_score_created_at,
        CASE
            WHEN baseline.eval_run_id IS NULL THEN 'candidate_only'
            WHEN candidate.eval_run_id IS NULL THEN 'baseline_only'
            WHEN baseline.snapshot_input IS DISTINCT FROM candidate.snapshot_input
              OR baseline.snapshot_expected_output IS DISTINCT FROM candidate.snapshot_expected_output
                THEN 'changed_case'
            WHEN baseline.status <> 'succeeded' OR candidate.status <> 'succeeded'
              OR baseline.selected_score_id IS NULL OR candidate.selected_score_id IS NULL
                THEN 'unscored'
            ELSE 'matched'
        END AS kind,
        CASE
            WHEN baseline.eval_run_id IS NOT NULL AND candidate.eval_run_id IS NOT NULL
              AND baseline.snapshot_input IS NOT DISTINCT FROM candidate.snapshot_input
              AND baseline.snapshot_expected_output IS NOT DISTINCT FROM candidate.snapshot_expected_output
              AND baseline.status = 'succeeded' AND candidate.status = 'succeeded'
              AND baseline.selected_score_id IS NOT NULL AND candidate.selected_score_id IS NOT NULL
                THEN candidate.selected_score_value - baseline.selected_score_value
            ELSE NULL
        END AS delta,
        baseline.eval_run_id IS NOT NULL AND candidate.eval_run_id IS NOT NULL
          AND baseline.snapshot_context IS DISTINCT FROM candidate.snapshot_context AS context_mismatch,
        baseline.selected_score_id IS NOT NULL AND candidate.selected_score_id IS NOT NULL
          AND (
              baseline.selected_judge_model,
              baseline.selected_judge_provider,
              baseline.selected_prompt_template_id,
              baseline.selected_score_threshold
          ) IS DISTINCT FROM (
              candidate.selected_judge_model,
              candidate.selected_judge_provider,
              candidate.selected_prompt_template_id,
              candidate.selected_score_threshold
          ) AS judge_config_mismatch
    FROM baseline_items AS baseline
    FULL OUTER JOIN candidate_items AS candidate USING (dataset_item_id)
), run_pair AS (
    SELECT true AS valid
    FROM eval_runs
    WHERE id IN (sqlc.arg(baseline_id), sqlc.arg(candidate_id))
    HAVING count(*) = 2
), summary AS (
    SELECT
        coalesce(sum(delta) FILTER (WHERE kind = 'matched'), 0)::double precision AS sum_delta,
        count(*) FILTER (WHERE kind = 'matched')::integer AS matched,
        count(*) FILTER (WHERE kind = 'changed_case')::integer AS changed_cases,
        count(*) FILTER (WHERE kind = 'baseline_only')::integer AS baseline_only,
        count(*) FILTER (WHERE kind = 'candidate_only')::integer AS candidate_only,
        count(*) FILTER (WHERE kind = 'unscored')::integer AS unscored,
        coalesce(bool_or(context_mismatch), false) AS context_mismatch,
        coalesce(bool_or(judge_config_mismatch), false) AS judge_config_mismatch
    FROM run_pair
    LEFT JOIN paired ON run_pair.valid
    GROUP BY run_pair.valid
), page AS (
    SELECT *
    FROM paired
    WHERE NOT sqlc.arg(has_cursor)::boolean
       OR dataset_item_id > sqlc.arg(cursor_id)::uuid
    ORDER BY dataset_item_id
    LIMIT sqlc.arg(page_size)
)
SELECT
    page.dataset_item_id,
    (page.dataset_item_id IS NOT NULL)::boolean AS has_item,
    (CASE WHEN baseline.eval_run_id IS NULL THEN 'null'::jsonb ELSE
        to_jsonb(baseline) || jsonb_build_object(
            'selected_score',
            CASE WHEN baseline_score.id IS NULL THEN NULL ELSE to_jsonb(baseline_score) END
        )
    END)::jsonb AS baseline_item,
    (CASE WHEN candidate.eval_run_id IS NULL THEN 'null'::jsonb ELSE
        to_jsonb(candidate) || jsonb_build_object(
            'selected_score',
            CASE WHEN candidate_score.id IS NULL THEN NULL ELSE to_jsonb(candidate_score) END
        )
    END)::jsonb AS candidate_item,
    summary.sum_delta,
    summary.matched,
    summary.changed_cases,
    summary.baseline_only,
    summary.candidate_only,
    summary.unscored,
    summary.context_mismatch::boolean,
    summary.judge_config_mismatch::boolean
FROM summary
LEFT JOIN page ON true
LEFT JOIN eval_run_items AS baseline
    ON baseline.eval_run_id = sqlc.arg(baseline_id)
   AND baseline.dataset_item_id = page.dataset_item_id
LEFT JOIN eval_run_items AS candidate
    ON candidate.eval_run_id = sqlc.arg(candidate_id)
   AND candidate.dataset_item_id = page.dataset_item_id
LEFT JOIN scores AS baseline_score
    ON baseline_score.id = page.baseline_score_id
   AND baseline_score.created_at = page.baseline_score_created_at
LEFT JOIN scores AS candidate_score
    ON candidate_score.id = page.candidate_score_id
   AND candidate_score.created_at = page.candidate_score_created_at
ORDER BY page.dataset_item_id NULLS LAST;
