-- name: CreateDataset :one
INSERT INTO datasets (id, application_id, name, description)
VALUES ($1, $2, $3, sqlc.narg(description))
RETURNING id, application_id, name, description, created_at, updated_at;

-- name: ListDatasets :many
SELECT id, application_id, name, description, created_at, updated_at
FROM datasets
WHERE (NOT sqlc.arg(filter_application)::boolean
       OR application_id = sqlc.arg(application_id))
  AND (NOT sqlc.arg(has_cursor)::boolean
       OR (created_at, id) > (sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::uuid))
ORDER BY created_at, id
LIMIT sqlc.arg(page_size);

-- name: GetDataset :one
SELECT id, application_id, name, description, created_at, updated_at
FROM datasets
WHERE id = $1;

-- name: UpdateDataset :one
UPDATE datasets
SET name = CASE WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name) ELSE name END,
    description = CASE
        WHEN sqlc.arg(clear_description)::boolean THEN NULL
        WHEN sqlc.arg(set_description)::boolean THEN sqlc.narg(description)
        ELSE description
    END,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, application_id, name, description, created_at, updated_at;

-- name: TouchDataset :exec
UPDATE datasets SET updated_at = now() WHERE id = $1;

-- name: DeleteDataset :one
DELETE FROM datasets WHERE id = $1 RETURNING id;

-- name: CreateDatasetItem :one
INSERT INTO dataset_items (
    id, dataset_id, external_id, input, output, expected_output, context, metadata
)
VALUES (
    sqlc.arg(id), sqlc.arg(dataset_id), sqlc.narg(external_id), sqlc.arg(input),
    sqlc.narg(output), sqlc.narg(expected_output), sqlc.narg(context), sqlc.arg(metadata)
)
RETURNING id, dataset_id, external_id, input, output, expected_output, context, metadata,
          created_at, updated_at;

-- name: ListDatasetItems :many
SELECT id, dataset_id, external_id, input, output, expected_output, context, metadata,
       created_at, updated_at
FROM dataset_items
WHERE dataset_id = sqlc.arg(dataset_id)
  AND (NOT sqlc.arg(has_cursor)::boolean
       OR (created_at, id) > (sqlc.arg(cursor_time)::timestamptz, sqlc.arg(cursor_id)::uuid))
ORDER BY created_at, id
LIMIT sqlc.arg(page_size);

-- name: GetDatasetItem :one
SELECT id, dataset_id, external_id, input, output, expected_output, context, metadata,
       created_at, updated_at
FROM dataset_items
WHERE dataset_id = sqlc.arg(dataset_id) AND id = sqlc.arg(item_id);

-- name: ReplaceDatasetItem :one
UPDATE dataset_items
SET external_id = sqlc.narg(external_id), input = sqlc.arg(input),
    output = sqlc.narg(output), expected_output = sqlc.narg(expected_output),
    context = sqlc.arg(context), metadata = sqlc.arg(metadata), updated_at = now()
WHERE dataset_id = sqlc.arg(dataset_id) AND id = sqlc.arg(item_id)
RETURNING id, dataset_id, external_id, input, output, expected_output, context, metadata,
          created_at, updated_at;

-- name: DeleteDatasetItem :one
DELETE FROM dataset_items
WHERE dataset_id = sqlc.arg(dataset_id) AND id = sqlc.arg(item_id)
RETURNING id;

-- name: LatestTraceScoreEvidence :one
SELECT id, judged_input, judged_output, judged_context, judged_reference
FROM scores
WHERE trace_id = sqlc.arg(trace_id) AND scorer = sqlc.arg(scorer)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountDatasetItems :one
SELECT count(*)::integer FROM dataset_items WHERE dataset_id = $1;

-- name: CountDatasetItemsMissingOutput :one
SELECT count(*)::integer FROM dataset_items WHERE dataset_id = $1 AND output IS NULL;

-- name: CountDatasetItemsMissingReference :one
SELECT count(*)::integer FROM dataset_items WHERE dataset_id = $1 AND expected_output IS NULL;
