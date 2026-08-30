-- name: CreateApplication :one
INSERT INTO applications (
    id, project_id, name, slug, config, auto_score_scorers, target_endpoint
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, project_id, name, slug, config, auto_score_scorers, target_endpoint,
          created_at, updated_at;

-- name: ListApplications :many
SELECT id, project_id, name, slug, config, auto_score_scorers, target_endpoint,
       created_at, updated_at
FROM applications
ORDER BY created_at, id;

-- name: ListApplicationsByProject :many
SELECT id, project_id, name, slug, config, auto_score_scorers, target_endpoint,
       created_at, updated_at
FROM applications
WHERE project_id = $1
ORDER BY created_at, id;

-- name: GetApplication :one
SELECT id, project_id, name, slug, config, auto_score_scorers, target_endpoint,
       created_at, updated_at
FROM applications
WHERE id = $1;

-- name: GetApplicationByProjectSlug :one
SELECT id, project_id, name, slug, config, auto_score_scorers, target_endpoint,
       created_at, updated_at
FROM applications
WHERE project_id = $1 AND slug = $2;

-- name: UpdateApplication :one
UPDATE applications
SET name = $2,
    slug = $3,
    config = $4,
    auto_score_scorers = $5,
    target_endpoint = $6,
    updated_at = now()
WHERE id = $1
RETURNING id, project_id, name, slug, config, auto_score_scorers, target_endpoint,
          created_at, updated_at;

-- name: DeleteApplication :one
DELETE FROM applications
WHERE id = $1
RETURNING id;
