package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	db "github.com/marioweid/assay/assayd/internal/store/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateDataset persists a dataset.
func (d *Database) CreateDataset(
	ctx context.Context,
	dataset domain.Dataset,
) (domain.Dataset, error) {
	row, err := d.queries.CreateDataset(ctx, db.CreateDatasetParams{
		ID: dataset.ID, ApplicationID: dataset.ApplicationID,
		Name: dataset.Name, Description: nullableText(dataset.Description),
	})
	if err != nil {
		return domain.Dataset{}, mapStoreError("insert dataset", err)
	}
	return datasetFromRow(row), nil
}

// ListDatasets returns a cursor-paginated dataset slice.
func (d *Database) ListDatasets(
	ctx context.Context,
	query domain.DatasetQuery,
) ([]domain.Dataset, error) {
	params := db.ListDatasetsParams{PageSize: int32(query.Limit)}
	if query.ApplicationID != nil {
		params.FilterApplication = true
		params.ApplicationID = *query.ApplicationID
	}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorTime = timestamp(query.Cursor.CreatedAt)
		params.CursorID = query.Cursor.ID
	}
	rows, err := d.queries.ListDatasets(ctx, params)
	if err != nil {
		return nil, mapStoreError("select datasets", err)
	}
	items := make([]domain.Dataset, 0, len(rows))
	for _, row := range rows {
		items = append(items, datasetFromRow(row))
	}
	return items, nil
}

// GetDataset returns a dataset by ID.
func (d *Database) GetDataset(ctx context.Context, datasetID uuid.UUID) (domain.Dataset, error) {
	row, err := d.queries.GetDataset(ctx, datasetID)
	if err != nil {
		return domain.Dataset{}, mapStoreError("select dataset", err)
	}
	return datasetFromRow(row), nil
}

// UpdateDataset applies selected metadata changes.
func (d *Database) UpdateDataset(
	ctx context.Context,
	datasetID uuid.UUID,
	input domain.UpdateDatasetInput,
) (domain.Dataset, error) {
	params := db.UpdateDatasetParams{
		ID: datasetID, SetName: input.Name != nil,
		SetDescription: input.Description != nil, ClearDescription: input.ClearDescription,
		Description: nullableText(input.Description),
	}
	if input.Name != nil {
		params.Name = *input.Name
	}
	row, err := d.queries.UpdateDataset(ctx, params)
	if err != nil {
		return domain.Dataset{}, mapStoreError("update dataset", err)
	}
	return datasetFromRow(row), nil
}

// DeleteDataset removes a dataset and its dependent records.
func (d *Database) DeleteDataset(ctx context.Context, datasetID uuid.UUID) error {
	return d.deleteWithJobLock(ctx, "delete dataset", func(queries *db.Queries) error {
		_, err := queries.DeleteDataset(ctx, datasetID)
		return err
	})
}

// CreateDatasetItems atomically persists dataset items.
func (d *Database) CreateDatasetItems(
	ctx context.Context,
	datasetID uuid.UUID,
	items []domain.DatasetItem,
) ([]domain.DatasetItem, error) {
	if len(items) == 0 {
		return []domain.DatasetItem{}, nil
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin dataset item transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	created := make([]domain.DatasetItem, 0, len(items))
	for _, item := range items {
		item.DatasetID = datasetID
		params, convertErr := datasetItemParameters(item)
		if convertErr != nil {
			return nil, convertErr
		}
		row, insertErr := queries.CreateDatasetItem(ctx, params)
		if insertErr != nil {
			return nil, mapStoreError("insert dataset item", insertErr)
		}
		createdItem, convertErr := datasetItemFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		created = append(created, createdItem)
	}
	if err := commitDatasetMutation(
		ctx, tx, queries, datasetID, "dataset item creation",
	); err != nil {
		return nil, err
	}
	return created, nil
}

// CreateDatasetItemFromTrace imports the latest retained online score evidence transactionally.
//
//nolint:cyclop // The transaction checks every persistence boundary before committing.
func (d *Database) CreateDatasetItemFromTrace(
	ctx context.Context,
	datasetID uuid.UUID,
	input domain.DatasetItemFromTraceInput,
) (domain.DatasetItem, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return domain.DatasetItem{}, fmt.Errorf("begin trace score import transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	dataset, err := queries.GetDataset(ctx, datasetID)
	if err != nil {
		return domain.DatasetItem{}, mapStoreError("select import dataset", err)
	}
	trace, err := queries.GetTraceByID(ctx, input.TraceID)
	if err != nil {
		return domain.DatasetItem{}, mapStoreError("select import trace", err)
	}
	if dataset.ApplicationID != trace.ApplicationID {
		return domain.DatasetItem{}, fmt.Errorf(
			"import trace score: %w: application mismatch", domain.ErrInvalid,
		)
	}
	evidence, err := queries.LatestTraceScoreEvidence(ctx, db.LatestTraceScoreEvidenceParams{
		TraceID: nullableUUID(&input.TraceID), Scorer: input.Scorer,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatasetItem{}, fmt.Errorf(
			"select trace score evidence: %w", domain.ErrInvalid,
		)
	}
	if err != nil {
		return domain.DatasetItem{}, mapStoreError("select trace score evidence", err)
	}
	item, err := traceScoreDatasetItem(datasetID, input, evidence)
	if err != nil {
		return domain.DatasetItem{}, err
	}
	params, err := datasetItemParameters(item)
	if err != nil {
		return domain.DatasetItem{}, err
	}
	row, err := queries.CreateDatasetItem(ctx, params)
	if err != nil {
		return domain.DatasetItem{}, mapStoreError("insert trace score dataset item", err)
	}
	if err := commitDatasetMutation(ctx, tx, queries, datasetID, "trace score import"); err != nil {
		return domain.DatasetItem{}, err
	}
	return datasetItemFromRow(row)
}

func traceScoreDatasetItem(
	datasetID uuid.UUID,
	input domain.DatasetItemFromTraceInput,
	evidence db.LatestTraceScoreEvidenceRow,
) (domain.DatasetItem, error) {
	if !evidence.JudgedInput.Valid || !evidence.JudgedOutput.Valid ||
		strings.TrimSpace(evidence.JudgedOutput.String) == "" {
		return domain.DatasetItem{}, fmt.Errorf(
			"import trace score: %w: score evidence is incomplete", domain.ErrInvalid,
		)
	}
	var context []domain.Chunk
	if err := decodeStoredJSON(evidence.JudgedContext, &context); err != nil {
		return domain.DatasetItem{}, fmt.Errorf(
			"decode trace score context: %w", errors.Join(domain.ErrInvalid, err),
		)
	}
	expected := optionalText(evidence.JudgedReference)
	if input.ExpectedOutput != nil {
		expected = input.ExpectedOutput
	}
	externalID := fmt.Sprintf("trace:%s:%s", input.TraceID, input.Scorer)
	item, err := domain.NewDatasetItem(datasetID, domain.CreateDatasetItemInput{
		ExternalID: &externalID, Input: map[string]any{"question": evidence.JudgedInput.String},
		Output: evidence.JudgedOutput.String, ExpectedOutput: expected, Context: context,
		Metadata: map[string]any{
			"trace_id": input.TraceID.String(), "score_id": evidence.ID, "scorer": input.Scorer,
		},
	})
	if err != nil {
		return domain.DatasetItem{}, fmt.Errorf("normalize trace score dataset item: %w", err)
	}
	return item, nil
}

// CountDatasetItems returns the number of cases currently in a dataset.
func (d *Database) CountDatasetItems(ctx context.Context, datasetID uuid.UUID) (int, error) {
	count, err := d.queries.CountDatasetItems(ctx, datasetID)
	if err != nil {
		return 0, mapStoreError("count dataset items", err)
	}
	return int(count), nil
}

// CountDatasetItemsMissingOutput returns the number of cases without stored output.
func (d *Database) CountDatasetItemsMissingOutput(
	ctx context.Context,
	datasetID uuid.UUID,
) (int, error) {
	count, err := d.queries.CountDatasetItemsMissingOutput(ctx, datasetID)
	if err != nil {
		return 0, mapStoreError("count dataset items missing output", err)
	}
	return int(count), nil
}

// CountDatasetItemsMissingReference returns the number of cases without expected output.
func (d *Database) CountDatasetItemsMissingReference(
	ctx context.Context,
	datasetID uuid.UUID,
) (int, error) {
	count, err := d.queries.CountDatasetItemsMissingReference(ctx, datasetID)
	if err != nil {
		return 0, mapStoreError("count dataset items missing reference", err)
	}
	return int(count), nil
}

// ListDatasetItems returns cursor-paginated items from one dataset.
func (d *Database) ListDatasetItems(
	ctx context.Context,
	datasetID uuid.UUID,
	query domain.PageQuery,
) ([]domain.DatasetItem, error) {
	params := db.ListDatasetItemsParams{DatasetID: datasetID, PageSize: int32(query.Limit)}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorTime = timestamp(query.Cursor.CreatedAt)
		params.CursorID = query.Cursor.ID
	}
	rows, err := d.queries.ListDatasetItems(ctx, params)
	if err != nil {
		return nil, mapStoreError("select dataset items", err)
	}
	items := make([]domain.DatasetItem, 0, len(rows))
	for _, row := range rows {
		item, convertErr := datasetItemFromRow(row)
		if convertErr != nil {
			return nil, convertErr
		}
		items = append(items, item)
	}
	return items, nil
}

// GetDatasetItem returns one item only when it belongs to the supplied dataset.
func (d *Database) GetDatasetItem(
	ctx context.Context,
	datasetID uuid.UUID,
	itemID uuid.UUID,
) (domain.DatasetItem, error) {
	row, err := d.queries.GetDatasetItem(ctx, db.GetDatasetItemParams{
		DatasetID: datasetID, ItemID: itemID,
	})
	if err != nil {
		return domain.DatasetItem{}, mapStoreError("select dataset item", err)
	}
	item, err := datasetItemFromRow(row)
	if err != nil {
		return domain.DatasetItem{}, err
	}
	return item, nil
}

// ReplaceDatasetItem atomically replaces editable fields and touches its dataset.
func (d *Database) ReplaceDatasetItem(
	ctx context.Context,
	datasetID uuid.UUID,
	item domain.DatasetItem,
) (domain.DatasetItem, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return domain.DatasetItem{}, fmt.Errorf("begin dataset item replacement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	params, err := replaceDatasetItemParameters(datasetID, item)
	if err != nil {
		return domain.DatasetItem{}, err
	}
	row, err := queries.ReplaceDatasetItem(ctx, params)
	if err != nil {
		return domain.DatasetItem{}, mapStoreError("replace dataset item", err)
	}
	if err := commitDatasetMutation(
		ctx, tx, queries, datasetID, "dataset item replacement",
	); err != nil {
		return domain.DatasetItem{}, err
	}
	return datasetItemFromRow(row)
}

// DeleteDatasetItem atomically removes one scoped item and touches its dataset.
func (d *Database) DeleteDatasetItem(
	ctx context.Context,
	datasetID uuid.UUID,
	itemID uuid.UUID,
) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin dataset item deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	_, err = queries.DeleteDatasetItem(ctx, db.DeleteDatasetItemParams{
		DatasetID: datasetID, ItemID: itemID,
	})
	if err != nil {
		return mapStoreError("delete dataset item", err)
	}
	return commitDatasetMutation(ctx, tx, queries, datasetID, "dataset item deletion")
}

func commitDatasetMutation(
	ctx context.Context,
	tx pgx.Tx,
	queries *db.Queries,
	datasetID uuid.UUID,
	operation string,
) error {
	if err := queries.TouchDataset(ctx, datasetID); err != nil {
		return mapStoreError("touch dataset after "+operation, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", operation, err)
	}
	return nil
}

func datasetFromRow(row db.Dataset) domain.Dataset {
	return domain.Dataset{
		ID: row.ID, ApplicationID: row.ApplicationID, Name: row.Name,
		Description: optionalText(row.Description), CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func datasetItemParameters(item domain.DatasetItem) (db.CreateDatasetItemParams, error) {
	input, err := encodeJSON("dataset item input", item.Input)
	if err != nil {
		return db.CreateDatasetItemParams{}, err
	}
	contextPayload, err := encodeJSON("dataset item context", item.Context)
	if err != nil {
		return db.CreateDatasetItemParams{}, err
	}
	metadata, err := encodeJSON("dataset item metadata", item.Metadata)
	if err != nil {
		return db.CreateDatasetItemParams{}, err
	}
	return db.CreateDatasetItemParams{
		ID: item.ID, DatasetID: item.DatasetID, ExternalID: nullableText(item.ExternalID),
		Input: input, Output: nullableText(item.Output),
		ExpectedOutput: nullableText(item.ExpectedOutput),
		Context:        contextPayload, Metadata: metadata,
	}, nil
}

func replaceDatasetItemParameters(
	datasetID uuid.UUID,
	item domain.DatasetItem,
) (db.ReplaceDatasetItemParams, error) {
	input, err := encodeJSON("dataset item input", item.Input)
	if err != nil {
		return db.ReplaceDatasetItemParams{}, err
	}
	contextPayload, err := encodeJSON("dataset item context", item.Context)
	if err != nil {
		return db.ReplaceDatasetItemParams{}, err
	}
	metadata, err := encodeJSON("dataset item metadata", item.Metadata)
	if err != nil {
		return db.ReplaceDatasetItemParams{}, err
	}
	return db.ReplaceDatasetItemParams{
		DatasetID: datasetID, ItemID: item.ID, ExternalID: nullableText(item.ExternalID),
		Input: input, Output: nullableText(item.Output),
		ExpectedOutput: nullableText(item.ExpectedOutput), Context: contextPayload,
		Metadata: metadata,
	}, nil
}

func datasetItemFromRow(row db.DatasetItem) (domain.DatasetItem, error) {
	item := domain.DatasetItem{
		ID: row.ID, DatasetID: row.DatasetID, ExternalID: optionalText(row.ExternalID),
		Output: optionalText(row.Output), ExpectedOutput: optionalText(row.ExpectedOutput),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if err := decodeStoredJSON(row.Input, &item.Input); err != nil {
		return domain.DatasetItem{}, fmt.Errorf("decode dataset item input: %w", err)
	}
	if err := normalizeAttributeNumbers(item.Input); err != nil {
		return domain.DatasetItem{}, fmt.Errorf("normalize dataset item input: %w", err)
	}
	if len(row.Context) > 0 {
		if err := decodeStoredJSON(row.Context, &item.Context); err != nil {
			return domain.DatasetItem{}, fmt.Errorf("decode dataset item context: %w", err)
		}
	}
	if err := decodeStoredJSON(row.Metadata, &item.Metadata); err != nil {
		return domain.DatasetItem{}, fmt.Errorf("decode dataset item metadata: %w", err)
	}
	if err := normalizeAttributeNumbers(item.Metadata); err != nil {
		return domain.DatasetItem{}, fmt.Errorf("normalize dataset item metadata: %w", err)
	}
	return item, nil
}

func optionalTimestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
