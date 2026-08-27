-- name: CreateAPIKey :one
INSERT INTO api_keys (id, project_id, name, key_hash, key_prefix)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, project_id, name, key_hash, key_prefix, last_used_at, revoked_at,
          created_at, updated_at;

-- name: ListAPIKeys :many
SELECT id, project_id, name, key_hash, key_prefix, last_used_at, revoked_at,
       created_at, updated_at
FROM api_keys
WHERE project_id = $1
ORDER BY created_at, id;

-- name: GetActiveAPIKeyByHash :one
SELECT id, project_id, name, key_hash, key_prefix, last_used_at, revoked_at,
       created_at, updated_at
FROM api_keys
WHERE key_hash = $1 AND revoked_at IS NULL;

-- name: RevokeAPIKey :one
UPDATE api_keys
SET revoked_at = now(), updated_at = now()
WHERE id = $1 AND project_id = $2 AND revoked_at IS NULL
RETURNING id;
