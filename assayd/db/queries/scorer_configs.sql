-- name: UpsertScorerConfig :one
INSERT INTO scorer_configs (
    id, application_id, scorer, enabled, threshold, judge_config, prompt_template_id
)
VALUES (
    sqlc.arg(id), sqlc.arg(application_id), sqlc.arg(scorer), sqlc.arg(enabled),
    sqlc.arg(threshold), sqlc.narg(judge_config), sqlc.narg(prompt_template_id)
)
ON CONFLICT (application_id, scorer) DO UPDATE
SET enabled = EXCLUDED.enabled,
    threshold = EXCLUDED.threshold,
    judge_config = EXCLUDED.judge_config,
    prompt_template_id = EXCLUDED.prompt_template_id,
    updated_at = now()
RETURNING id, application_id, scorer, enabled, threshold, judge_config, prompt_template_id,
          created_at, updated_at;

-- name: ListScorerConfigs :many
SELECT id, application_id, scorer, enabled, threshold, judge_config, prompt_template_id,
       created_at, updated_at
FROM scorer_configs
WHERE application_id = $1
ORDER BY scorer;

-- name: GetScorerConfig :one
SELECT id, application_id, scorer, enabled, threshold, judge_config, prompt_template_id,
       created_at, updated_at
FROM scorer_configs
WHERE application_id = $1 AND scorer = $2;
