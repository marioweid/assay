-- +goose Up
ALTER TABLE eval_run_items
    ADD COLUMN snapshot_dataset_id uuid,
    ADD COLUMN snapshot_external_id text,
    ADD COLUMN snapshot_input jsonb,
    ADD COLUMN snapshot_output text,
    ADD COLUMN snapshot_expected_output text,
    ADD COLUMN snapshot_context jsonb,
    ADD COLUMN snapshot_metadata jsonb,
    ADD COLUMN snapshot_created_at timestamptz,
    ADD COLUMN snapshot_updated_at timestamptz,
    ADD COLUMN snapshot_origin text NOT NULL DEFAULT 'legacy_backfill';

UPDATE eval_run_items AS run_item
SET snapshot_dataset_id = item.dataset_id,
    snapshot_external_id = item.external_id,
    snapshot_input = item.input,
    snapshot_output = item.output,
    snapshot_expected_output = item.expected_output,
    snapshot_context = coalesce(item.context, '[]'::jsonb),
    snapshot_metadata = item.metadata,
    snapshot_created_at = item.created_at,
    snapshot_updated_at = item.updated_at
FROM dataset_items AS item
WHERE item.id = run_item.dataset_item_id;

ALTER TABLE eval_run_items
    DROP CONSTRAINT eval_run_items_dataset_item_id_fkey,
    ALTER COLUMN snapshot_dataset_id SET NOT NULL,
    ALTER COLUMN snapshot_input SET NOT NULL,
    ALTER COLUMN snapshot_context SET NOT NULL,
    ALTER COLUMN snapshot_metadata SET NOT NULL,
    ALTER COLUMN snapshot_created_at SET NOT NULL,
    ALTER COLUMN snapshot_updated_at SET NOT NULL,
    ALTER COLUMN snapshot_origin SET DEFAULT 'creation',
    ADD CONSTRAINT eval_run_items_snapshot_origin_check
        CHECK (snapshot_origin IN ('creation', 'legacy_backfill')),
    ADD CONSTRAINT eval_run_items_snapshot_input_check
        CHECK (jsonb_typeof(snapshot_input) = 'object'),
    ADD CONSTRAINT eval_run_items_snapshot_context_check
        CHECK (jsonb_typeof(snapshot_context) = 'array'),
    ADD CONSTRAINT eval_run_items_snapshot_metadata_check
        CHECK (jsonb_typeof(snapshot_metadata) = 'object');

-- +goose Down
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM eval_run_items AS run_item
        LEFT JOIN dataset_items AS item ON item.id = run_item.dataset_item_id
        WHERE item.id IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot roll back snapshots: source dataset items no longer exist';
    END IF;
END $$;

ALTER TABLE eval_run_items
    DROP CONSTRAINT eval_run_items_snapshot_metadata_check,
    DROP CONSTRAINT eval_run_items_snapshot_context_check,
    DROP CONSTRAINT eval_run_items_snapshot_input_check,
    DROP CONSTRAINT eval_run_items_snapshot_origin_check,
    DROP COLUMN snapshot_origin,
    DROP COLUMN snapshot_updated_at,
    DROP COLUMN snapshot_created_at,
    DROP COLUMN snapshot_metadata,
    DROP COLUMN snapshot_context,
    DROP COLUMN snapshot_expected_output,
    DROP COLUMN snapshot_output,
    DROP COLUMN snapshot_input,
    DROP COLUMN snapshot_external_id,
    DROP COLUMN snapshot_dataset_id,
    ADD CONSTRAINT eval_run_items_dataset_item_id_fkey
        FOREIGN KEY (dataset_item_id) REFERENCES dataset_items(id) ON DELETE CASCADE;
