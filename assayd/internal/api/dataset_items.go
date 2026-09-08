package api

import (
	"context"
	"net/http"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type datasetItemIDInput struct {
	ID     string `path:"id" format:"uuid"`
	ItemID string `path:"itemId" format:"uuid"`
}

type replaceDatasetItemInput struct {
	ID     string `path:"id" format:"uuid"`
	ItemID string `path:"itemId" format:"uuid"`
	Body   struct {
		ExternalID     *string        `json:"external_id" required:"true"`
		Input          map[string]any `json:"input" required:"true"`
		Output         *string        `json:"output" required:"true"`
		ExpectedOutput *string        `json:"expected_output" required:"true"`
		Context        []domain.Chunk `json:"context" required:"true" nullable:"false"`
		Metadata       map[string]any `json:"metadata" required:"true"`
	}
}

type datasetItemResult struct{ Body datasetItemResponse }

func (h *handler) registerDatasetItemRoutes() {
	path := "/v1/datasets/{id}/items/{itemId}"
	huma.Register(h.api, h.operation(
		http.MethodGet, path, "get-dataset-item", "Get a dataset item", http.StatusNotFound,
	), h.getDatasetItem)
	huma.Register(h.api, h.operation(
		http.MethodPut, path, "replace-dataset-item", "Replace a dataset item",
		http.StatusNotFound, http.StatusConflict,
	), h.replaceDatasetItem)
	remove := h.operation(
		http.MethodDelete, path, "delete-dataset-item", "Delete a dataset item",
		http.StatusNotFound,
	)
	remove.DefaultStatus = http.StatusNoContent
	huma.Register(h.api, remove, h.deleteDatasetItem)
}

func (h *handler) getDatasetItem(
	ctx context.Context,
	input *datasetItemIDInput,
) (*datasetItemResult, error) {
	datasetID, itemID, err := parseDatasetItemIDs(input.ID, input.ItemID)
	if err != nil {
		return nil, h.responseError("get dataset item", err)
	}
	item, err := h.evaluations.GetDatasetItem(ctx, datasetID, itemID)
	if err != nil {
		return nil, h.responseError("get dataset item", err)
	}
	return &datasetItemResult{Body: datasetItemOutput(item)}, nil
}

func (h *handler) replaceDatasetItem(
	ctx context.Context,
	input *replaceDatasetItemInput,
) (*datasetItemResult, error) {
	datasetID, itemID, err := parseDatasetItemIDs(input.ID, input.ItemID)
	if err != nil {
		return nil, h.responseError("replace dataset item", err)
	}
	item, err := h.evaluations.ReplaceDatasetItem(ctx, datasetID, itemID,
		domain.ReplaceDatasetItemInput{
			ExternalID: input.Body.ExternalID, Input: input.Body.Input, Output: input.Body.Output,
			ExpectedOutput: input.Body.ExpectedOutput, Context: input.Body.Context,
			Metadata: input.Body.Metadata,
		})
	if err != nil {
		return nil, h.responseError("replace dataset item", err)
	}
	return &datasetItemResult{Body: datasetItemOutput(item)}, nil
}

func (h *handler) deleteDatasetItem(
	ctx context.Context,
	input *datasetItemIDInput,
) (*emptyOutput, error) {
	datasetID, itemID, err := parseDatasetItemIDs(input.ID, input.ItemID)
	if err != nil {
		return nil, h.responseError("delete dataset item", err)
	}
	if err := h.evaluations.DeleteDatasetItem(ctx, datasetID, itemID); err != nil {
		return nil, h.responseError("delete dataset item", err)
	}
	return &emptyOutput{}, nil
}

func parseDatasetItemIDs(datasetValue string, itemValue string) (uuid.UUID, uuid.UUID, error) {
	datasetID, err := parseID(datasetValue, "dataset ID")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	itemID, err := parseID(itemValue, "dataset item ID")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return datasetID, itemID, nil
}
