package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type createDatasetInput struct {
	Body struct {
		ApplicationID string  `json:"application_id" format:"uuid"`
		Name          string  `json:"name" minLength:"1"`
		Description   *string `json:"description,omitempty"`
	}
}

type listDatasetsInput struct {
	ApplicationID string `query:"application_id" format:"uuid" required:"false"`
	Limit         int    `query:"limit" minimum:"0" maximum:"500" required:"false"`
	Cursor        string `query:"cursor" required:"false"`
}

type datasetIDInput struct {
	ID string `path:"id" format:"uuid"`
}

type createDatasetItemsInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Items []datasetItemInput `json:"items" minItems:"1" maxItems:"1000"`
	}
}

type listDatasetItemsInput struct {
	ID     string `path:"id" format:"uuid"`
	Limit  int    `query:"limit" minimum:"0" maximum:"500" required:"false"`
	Cursor string `query:"cursor" required:"false"`
}

type datasetItemInput struct {
	ExternalID     *string        `json:"external_id,omitempty"`
	Input          map[string]any `json:"input"`
	Output         string         `json:"output"`
	ExpectedOutput *string        `json:"expected_output,omitempty"`
	Context        []domain.Chunk `json:"context,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type datasetResult struct{ Body datasetResponse }
type datasetCollectionResult struct {
	Body struct {
		Items      []datasetResponse `json:"items"`
		NextCursor string            `json:"next_cursor,omitempty"`
	}
}
type datasetItemCollectionResult struct {
	Body struct {
		Items      []datasetItemResponse `json:"items"`
		NextCursor string                `json:"next_cursor,omitempty"`
	}
}

type datasetResponse struct {
	ID            string    `json:"id" format:"uuid"`
	ApplicationID string    `json:"application_id" format:"uuid"`
	Name          string    `json:"name"`
	Description   *string   `json:"description,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type datasetItemResponse struct {
	ID             string         `json:"id" format:"uuid"`
	DatasetID      string         `json:"dataset_id" format:"uuid"`
	ExternalID     *string        `json:"external_id,omitempty"`
	Input          map[string]any `json:"input"`
	Output         *string        `json:"output,omitempty"`
	ExpectedOutput *string        `json:"expected_output,omitempty"`
	Context        []domain.Chunk `json:"context"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type pageCursorJSON struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func (h *handler) registerDatasetRoutes() {
	create := h.operation(
		http.MethodPost, "/v1/datasets", "create-dataset", "Create a dataset",
		http.StatusConflict,
	)
	create.DefaultStatus = http.StatusCreated
	huma.Register(h.api, create, h.createDataset)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/datasets", "list-datasets", "List datasets",
	), h.listDatasets)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/datasets/{id}", "get-dataset", "Get a dataset",
		http.StatusNotFound,
	), h.getDataset)
	remove := h.operation(
		http.MethodDelete, "/v1/datasets/{id}", "delete-dataset", "Delete a dataset",
		http.StatusNotFound,
	)
	remove.DefaultStatus = http.StatusNoContent
	huma.Register(h.api, remove, h.deleteDataset)
	addItems := h.operation(
		http.MethodPost, "/v1/datasets/{id}/items", "create-dataset-items",
		"Create dataset items", http.StatusNotFound, http.StatusConflict,
	)
	addItems.DefaultStatus = http.StatusCreated
	huma.Register(h.api, addItems, h.createDatasetItems)
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/datasets/{id}/items", "list-dataset-items",
		"List dataset items", http.StatusNotFound,
	), h.listDatasetItems)
}

func (h *handler) createDataset(
	ctx context.Context,
	input *createDatasetInput,
) (*datasetResult, error) {
	applicationID, err := parseID(input.Body.ApplicationID, "application ID")
	if err != nil {
		return nil, h.responseError("create dataset", err)
	}
	dataset, err := h.evaluations.CreateDataset(ctx, domain.CreateDatasetInput{
		ApplicationID: applicationID, Name: input.Body.Name, Description: input.Body.Description,
	})
	if err != nil {
		return nil, h.responseError("create dataset", err)
	}
	return &datasetResult{Body: datasetOutput(dataset)}, nil
}

func (h *handler) listDatasets(
	ctx context.Context,
	input *listDatasetsInput,
) (*datasetCollectionResult, error) {
	query := domain.DatasetQuery{PageQuery: domain.PageQuery{Limit: input.Limit}}
	var err error
	query.Cursor, err = decodePageCursor(input.Cursor)
	if err == nil && input.ApplicationID != "" {
		applicationID, parseErr := parseID(input.ApplicationID, "application ID")
		query.ApplicationID, err = &applicationID, parseErr
	}
	if err != nil {
		return nil, h.responseError("list datasets", err)
	}
	page, err := h.evaluations.ListDatasets(ctx, query)
	if err != nil {
		return nil, h.responseError("list datasets", err)
	}
	result := &datasetCollectionResult{}
	result.Body.Items = make([]datasetResponse, 0, len(page.Items))
	for _, dataset := range page.Items {
		result.Body.Items = append(result.Body.Items, datasetOutput(dataset))
	}
	result.Body.NextCursor, err = encodePageCursor(page.NextCursor)
	return result, err
}

func (h *handler) getDataset(ctx context.Context, input *datasetIDInput) (*datasetResult, error) {
	id, err := parseID(input.ID, "dataset ID")
	if err != nil {
		return nil, h.responseError("get dataset", err)
	}
	dataset, err := h.evaluations.GetDataset(ctx, id)
	if err != nil {
		return nil, h.responseError("get dataset", err)
	}
	return &datasetResult{Body: datasetOutput(dataset)}, nil
}

func (h *handler) deleteDataset(ctx context.Context, input *datasetIDInput) (*emptyOutput, error) {
	id, err := parseID(input.ID, "dataset ID")
	if err != nil {
		return nil, h.responseError("delete dataset", err)
	}
	if err := h.evaluations.DeleteDataset(ctx, id); err != nil {
		return nil, h.responseError("delete dataset", err)
	}
	return &emptyOutput{}, nil
}

func (h *handler) createDatasetItems(
	ctx context.Context,
	input *createDatasetItemsInput,
) (*datasetItemCollectionResult, error) {
	id, err := parseID(input.ID, "dataset ID")
	if err != nil {
		return nil, h.responseError("create dataset items", err)
	}
	items := make([]domain.CreateDatasetItemInput, 0, len(input.Body.Items))
	for _, item := range input.Body.Items {
		items = append(items, domain.CreateDatasetItemInput{
			ExternalID: item.ExternalID, Input: item.Input, Output: item.Output,
			ExpectedOutput: item.ExpectedOutput, Context: item.Context, Metadata: item.Metadata,
		})
	}
	created, err := h.evaluations.CreateDatasetItems(ctx, id, items)
	if err != nil {
		return nil, h.responseError("create dataset items", err)
	}
	result := &datasetItemCollectionResult{}
	result.Body.Items = make([]datasetItemResponse, 0, len(created))
	for _, item := range created {
		result.Body.Items = append(result.Body.Items, datasetItemOutput(item))
	}
	return result, nil
}

func (h *handler) listDatasetItems(
	ctx context.Context,
	input *listDatasetItemsInput,
) (*datasetItemCollectionResult, error) {
	id, err := parseID(input.ID, "dataset ID")
	if err != nil {
		return nil, h.responseError("list dataset items", err)
	}
	cursor, err := decodePageCursor(input.Cursor)
	if err != nil {
		return nil, h.responseError("list dataset items", err)
	}
	page, err := h.evaluations.ListDatasetItems(
		ctx, id, domain.PageQuery{Limit: input.Limit, Cursor: cursor},
	)
	if err != nil {
		return nil, h.responseError("list dataset items", err)
	}
	result := &datasetItemCollectionResult{}
	result.Body.Items = make([]datasetItemResponse, 0, len(page.Items))
	for _, item := range page.Items {
		result.Body.Items = append(result.Body.Items, datasetItemOutput(item))
	}
	result.Body.NextCursor, err = encodePageCursor(page.NextCursor)
	return result, err
}

func datasetOutput(dataset domain.Dataset) datasetResponse {
	return datasetResponse{
		ID: dataset.ID.String(), ApplicationID: dataset.ApplicationID.String(),
		Name: dataset.Name, Description: dataset.Description,
		CreatedAt: dataset.CreatedAt, UpdatedAt: dataset.UpdatedAt,
	}
}

func datasetItemOutput(item domain.DatasetItem) datasetItemResponse {
	return datasetItemResponse{
		ID: item.ID.String(), DatasetID: item.DatasetID.String(),
		ExternalID: item.ExternalID, Input: item.Input, Output: item.Output,
		ExpectedOutput: item.ExpectedOutput, Context: item.Context, Metadata: item.Metadata,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func encodePageCursor(cursor *domain.PageCursor) (string, error) {
	if cursor == nil {
		return "", nil
	}
	payload, err := json.Marshal(pageCursorJSON{CreatedAt: cursor.CreatedAt, ID: cursor.ID.String()})
	if err != nil {
		return "", fmt.Errorf("encode page cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodePageCursor(encoded string) (*domain.PageCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("page cursor: %w: invalid encoding", domain.ErrInvalid)
	}
	var cursor pageCursorJSON
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return nil, fmt.Errorf("page cursor: %w: invalid JSON", domain.ErrInvalid)
	}
	id, err := uuid.Parse(cursor.ID)
	if err != nil || cursor.CreatedAt.IsZero() {
		return nil, fmt.Errorf("page cursor: %w: invalid values", domain.ErrInvalid)
	}
	return &domain.PageCursor{CreatedAt: cursor.CreatedAt, ID: id}, nil
}
