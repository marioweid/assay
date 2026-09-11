package domain

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// UpdateDataset validates and applies dataset metadata changes.
func (s *EvaluationService) UpdateDataset(
	ctx context.Context,
	datasetID uuid.UUID,
	input UpdateDatasetInput,
) (Dataset, error) {
	input, err := normalizeDatasetUpdate(input)
	if err != nil {
		return Dataset{}, err
	}
	dataset, err := s.repository.UpdateDataset(ctx, datasetID, input)
	if err != nil {
		return Dataset{}, fmt.Errorf("update dataset %s: %w", datasetID, err)
	}
	return dataset, nil
}

func normalizeDatasetUpdate(input UpdateDatasetInput) (UpdateDatasetInput, error) {
	if input.Name == nil && input.Description == nil && !input.ClearDescription {
		return UpdateDatasetInput{}, fmt.Errorf("update dataset: %w: no changes supplied", ErrInvalid)
	}
	if input.Description != nil && input.ClearDescription {
		return UpdateDatasetInput{}, fmt.Errorf(
			"update dataset: %w: description and clear_description conflict", ErrInvalid,
		)
	}
	if input.Name != nil {
		name, err := requiredValue("dataset name", *input.Name)
		if err != nil {
			return UpdateDatasetInput{}, err
		}
		input.Name = &name
	}
	input.Description = trimmedOptional(input.Description)
	return input, nil
}

// GetDatasetItem returns one case scoped to its dataset.
func (s *EvaluationService) GetDatasetItem(
	ctx context.Context,
	datasetID uuid.UUID,
	itemID uuid.UUID,
) (DatasetItem, error) {
	item, err := s.repository.GetDatasetItem(ctx, datasetID, itemID)
	if err != nil {
		return DatasetItem{}, fmt.Errorf("get dataset item %s: %w", itemID, err)
	}
	return item, nil
}

// ReplaceDatasetItem validates and replaces all editable fields of one case.
func (s *EvaluationService) ReplaceDatasetItem(
	ctx context.Context,
	datasetID uuid.UUID,
	itemID uuid.UUID,
	input ReplaceDatasetItemInput,
) (DatasetItem, error) {
	item, err := normalizeDatasetItem(input)
	if err != nil {
		return DatasetItem{}, fmt.Errorf("replace dataset item: %w", err)
	}
	item.ID = itemID
	item.DatasetID = datasetID
	item, err = s.repository.ReplaceDatasetItem(ctx, datasetID, item)
	if err != nil {
		return DatasetItem{}, fmt.Errorf("replace dataset item %s: %w", itemID, err)
	}
	return item, nil
}

// DeleteDatasetItem removes one case scoped to its dataset.
func (s *EvaluationService) DeleteDatasetItem(
	ctx context.Context,
	datasetID uuid.UUID,
	itemID uuid.UUID,
) error {
	if err := s.repository.DeleteDatasetItem(ctx, datasetID, itemID); err != nil {
		return fmt.Errorf("delete dataset item %s: %w", itemID, err)
	}
	return nil
}

func normalizeDatasetItem(input ReplaceDatasetItemInput) (DatasetItem, error) {
	values := cloneJSONMap(input.Input)
	question, ok := values["question"].(string)
	question = strings.TrimSpace(question)
	if !ok || question == "" {
		return DatasetItem{}, fmt.Errorf("question: %w: must be a non-blank string", ErrInvalid)
	}
	values["question"] = question
	contextChunks, err := normalizedChunks(input.Context)
	if err != nil {
		return DatasetItem{}, err
	}
	expectedOutput, err := nonblankOptional("expected output", input.ExpectedOutput)
	if err != nil {
		return DatasetItem{}, err
	}
	metadata := cloneJSONMap(input.Metadata)
	return DatasetItem{
		ExternalID: trimmedOptional(input.ExternalID), Input: values,
		Output: optionalNonblank(input.Output), ExpectedOutput: expectedOutput,
		Context: contextChunks, Metadata: metadata,
	}, nil
}

func optionalNonblank(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func nonblankOptional(name string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed, err := requiredValue(name, *value)
	if err != nil {
		return nil, err
	}
	return &trimmed, nil
}
