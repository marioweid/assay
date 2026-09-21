package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type compareEvalRunsInput struct {
	ID         string `path:"id" format:"uuid"`
	OtherRunID string `query:"other_run_id" format:"uuid" required:"true"`
	Scorer     string `query:"scorer" required:"true"`
	Limit      int    `query:"limit" minimum:"0" maximum:"500" required:"false"`
	Cursor     string `query:"cursor" required:"false"`
}

type runComparisonResult struct {
	Body runComparisonResponse
}

type runComparisonResponse struct {
	BaselineRunID  string               `json:"baseline_run_id" format:"uuid"`
	CandidateRunID string               `json:"candidate_run_id" format:"uuid"`
	Scorer         string               `json:"scorer"`
	Items          []runComparisonRow   `json:"items" nullable:"false"`
	NextCursor     string               `json:"next_cursor,omitempty"`
	Summary        runComparisonSummary `json:"summary"`
	Warnings       []string             `json:"warnings" nullable:"false"`
}

type runComparisonRow struct {
	DatasetItemID string `json:"dataset_item_id" format:"uuid"`
	//nolint:lll // Huma needs the complete comparison enum in one schema tag.
	Kind      string                      `json:"kind" enum:"matched,changed_case,baseline_only,candidate_only,unscored"`
	Baseline  nullableEvalRunItemResponse `json:"baseline"`
	Candidate nullableEvalRunItemResponse `json:"candidate"`
	Delta     *float64                    `json:"delta"`
}

type nullableEvalRunItemResponse struct {
	Value *evalRunItemResponse
}

func (value nullableEvalRunItemResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(value.Value)
}

func (nullableEvalRunItemResponse) Schema(registry huma.Registry) *huma.Schema {
	return &huma.Schema{AnyOf: []*huma.Schema{
		registry.Schema(reflect.TypeFor[evalRunItemResponse](), true, ""),
		{Type: "null"},
	}}
}

type runComparisonSummary struct {
	N             int      `json:"n"`
	MeanDelta     *float64 `json:"mean_delta"`
	Matched       int      `json:"matched"`
	ChangedCases  int      `json:"changed_cases"`
	BaselineOnly  int      `json:"baseline_only"`
	CandidateOnly int      `json:"candidate_only"`
	Unscored      int      `json:"unscored"`
}

type runComparisonCursor struct {
	BaselineID  string `json:"baseline_id"`
	CandidateID string `json:"candidate_id"`
	Scorer      string `json:"scorer"`
	ItemID      string `json:"item_id"`
}

func (h *handler) registerRunComparisonRoute() {
	huma.Register(h.api, h.operation(
		http.MethodGet, "/v1/runs/{id}/comparison", "compare-eval-runs",
		"Compare evaluation runs", http.StatusNotFound, http.StatusConflict,
	), h.compareEvalRuns)
}

func (h *handler) compareEvalRuns(
	ctx context.Context,
	input *compareEvalRunsInput,
) (*runComparisonResult, error) {
	baselineID, err := parseID(input.ID, "baseline run ID")
	if err != nil {
		return nil, h.responseError("compare eval runs", err)
	}
	candidateID, err := parseID(input.OtherRunID, "candidate run ID")
	if err != nil {
		return nil, h.responseError("compare eval runs", err)
	}
	cursor, err := decodeRunComparisonCursor(
		input.Cursor, baselineID, candidateID, input.Scorer,
	)
	if err != nil {
		return nil, h.responseError("compare eval runs", err)
	}
	page, err := h.comparisons.Compare(ctx, domain.RunComparisonQuery{
		BaselineID: baselineID, CandidateID: candidateID, Scorer: input.Scorer,
		Limit: input.Limit, Cursor: cursor,
	})
	if err != nil {
		return nil, h.responseError("compare eval runs", err)
	}
	response, err := runComparisonOutput(page)
	if err != nil {
		return nil, h.responseError("compare eval runs", err)
	}
	return &runComparisonResult{Body: response}, nil
}

func runComparisonOutput(page domain.RunComparisonPage) (runComparisonResponse, error) {
	response := runComparisonResponse{
		BaselineRunID: page.BaselineRunID.String(), CandidateRunID: page.CandidateRunID.String(),
		Scorer: page.Scorer, Items: make([]runComparisonRow, 0, len(page.Items)),
		Warnings: page.Warnings,
		Summary: runComparisonSummary{
			N: page.Summary.N, MeanDelta: page.Summary.MeanDelta,
			Matched: page.Summary.Matched, ChangedCases: page.Summary.ChangedCases,
			BaselineOnly:  page.Summary.BaselineOnly,
			CandidateOnly: page.Summary.CandidateOnly, Unscored: page.Summary.Unscored,
		},
	}
	for _, item := range page.Items {
		row := runComparisonRow{
			DatasetItemID: item.DatasetItemID.String(), Kind: item.Kind, Delta: item.Delta,
		}
		if item.Baseline != nil {
			value := evalRunItemOutput(*item.Baseline)
			row.Baseline.Value = &value
		}
		if item.Candidate != nil {
			value := evalRunItemOutput(*item.Candidate)
			row.Candidate.Value = &value
		}
		response.Items = append(response.Items, row)
	}
	var err error
	response.NextCursor, err = encodeRunComparisonCursor(page)
	return response, err
}

func encodeRunComparisonCursor(page domain.RunComparisonPage) (string, error) {
	if page.NextCursor == nil {
		return "", nil
	}
	payload, err := json.Marshal(runComparisonCursor{
		BaselineID: page.BaselineRunID.String(), CandidateID: page.CandidateRunID.String(),
		Scorer: page.Scorer, ItemID: page.NextCursor.String(),
	})
	if err != nil {
		return "", fmt.Errorf("encode run comparison cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeRunComparisonCursor(
	value string,
	baselineID uuid.UUID,
	candidateID uuid.UUID,
	scorer string,
) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("comparison cursor: %w: invalid encoding", domain.ErrInvalid)
	}
	var cursor runComparisonCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return nil, fmt.Errorf("comparison cursor: %w: invalid payload", domain.ErrInvalid)
	}
	if !comparisonCursorMatches(cursor, baselineID, candidateID, scorer) {
		return nil, fmt.Errorf(
			"comparison cursor: %w: cursor belongs to another comparison",
			domain.ErrInvalid,
		)
	}
	return parseComparisonCursorItem(cursor.ItemID)
}

func comparisonCursorMatches(
	cursor runComparisonCursor,
	baselineID uuid.UUID,
	candidateID uuid.UUID,
	scorer string,
) bool {
	return cursor.BaselineID == baselineID.String() &&
		cursor.CandidateID == candidateID.String() && cursor.Scorer == scorer
}

func parseComparisonCursorItem(value string) (*uuid.UUID, error) {
	itemID, err := uuid.Parse(value)
	if err != nil || itemID == uuid.Nil {
		return nil, fmt.Errorf(
			"comparison cursor: %w: invalid item ID",
			domain.ErrInvalid,
		)
	}
	return &itemID, nil
}
