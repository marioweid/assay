-- name: CreateProject :one
INSERT INTO projects (id, name, judge_config)
VALUES ($1, $2, $3)
RETURNING id, name, judge_config, created_at, updated_at;

-- name: ListProjects :many
SELECT id, name, judge_config, created_at, updated_at
FROM projects
ORDER BY created_at, id;

-- name: GetProject :one
SELECT id, name, judge_config, created_at, updated_at
FROM projects
WHERE id = $1;

-- name: UpdateProject :one
UPDATE projects
SET name = $2, judge_config = $3, updated_at = now()
WHERE id = $1
RETURNING id, name, judge_config, created_at, updated_at;

-- name: DeleteProject :one
DELETE FROM projects
WHERE id = $1
RETURNING id;
