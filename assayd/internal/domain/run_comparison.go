package domain

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/google/uuid"
)

const (
	// ComparisonMatched identifies an unchanged case with both requested scores.
	ComparisonMatched = "matched"
	// ComparisonChangedCase identifies a case whose input or reference changed.
	ComparisonChangedCase = "changed_case"
	// ComparisonBaselineOnly identifies a case absent from the candidate run.
	ComparisonBaselineOnly = "baseline_only"
	// ComparisonCandidateOnly identifies a case absent from the baseline run.
	ComparisonCandidateOnly = "candidate_only"
	// ComparisonUnscored identifies a paired case without two successful scores.
	ComparisonUnscored = "unscored"
)

// RunComparisonQuery identifies a fixed pair and scorer comparison page.
type RunComparisonQuery struct {
	BaselineID  uuid.UUID
	CandidateID uuid.UUID
	Scorer      string
	Limit       int
	Cursor      *uuid.UUID
}

// RunComparisonEvidence contains both historical outcomes for one dataset item.
type RunComparisonEvidence struct {
	DatasetItemID uuid.UUID
	Baseline      *EvalRunItem
	Candidate     *EvalRunItem
}

// RunComparisonAggregate contains all-page counters calculated by the repository.
type RunComparisonAggregate struct {
	SumDelta      float64
	Matched       int
	ChangedCases  int
	BaselineOnly  int
	CandidateOnly int
	Unscored      int
}

// RunComparisonRead is one repository page plus all-page aggregate metadata.
type RunComparisonRead struct {
	Items               []RunComparisonEvidence
	Aggregate           RunComparisonAggregate
	ContextMismatch     bool
	JudgeConfigMismatch bool
}

// RunComparisonRow is one classified historical case pair.
type RunComparisonRow struct {
	DatasetItemID uuid.UUID
	Kind          string
	Baseline      *EvalRunItem
	Candidate     *EvalRunItem
	Delta         *float64
}

// RunComparisonSummary explains the full comparison denominator.
type RunComparisonSummary struct {
	N             int
	MeanDelta     *float64
	Matched       int
	ChangedCases  int
	BaselineOnly  int
	CandidateOnly int
	Unscored      int
}

// RunComparisonPage is one classified page with all-page summary metadata.
type RunComparisonPage struct {
	BaselineRunID  uuid.UUID
	CandidateRunID uuid.UUID
	Scorer         string
	Items          []RunComparisonRow
	NextCursor     *uuid.UUID
	Summary        RunComparisonSummary
	Warnings       []string
}

// RunComparisonRepository loads run metadata and one set-based comparison page.
type RunComparisonRepository interface {
	GetEvalRun(context.Context, uuid.UUID) (EvalRun, error)
	CompareEvalRuns(context.Context, RunComparisonQuery) (RunComparisonRead, error)
}

// RunComparisonService validates and classifies paired historical run evidence.
type RunComparisonService struct {
	repository RunComparisonRepository
}

// NewRunComparisonService constructs a paired-run comparison service.
func NewRunComparisonService(repository RunComparisonRepository) *RunComparisonService {
	return &RunComparisonService{repository: repository}
}

// Compare validates a fixed run pair and returns one classified evidence page.
func (s *RunComparisonService) Compare(
	ctx context.Context,
	query RunComparisonQuery,
) (RunComparisonPage, error) {
	baseline, candidate, err := s.validateRuns(ctx, query)
	if err != nil {
		return RunComparisonPage{}, err
	}
	if err := normalizeComparisonQuery(&query); err != nil {
		return RunComparisonPage{}, err
	}
	pageSize := query.Limit
	query.Limit++
	read, err := s.repository.CompareEvalRuns(ctx, query)
	if err != nil {
		return RunComparisonPage{}, fmt.Errorf("compare eval runs: %w", err)
	}
	page := RunComparisonPage{
		BaselineRunID: baseline.ID, CandidateRunID: candidate.ID, Scorer: query.Scorer,
		Items:   classifyComparisonItems(read.Items, query.Scorer),
		Summary: comparisonSummary(read.Aggregate), Warnings: comparisonWarnings(read),
	}
	if len(page.Items) > pageSize {
		page.Items = page.Items[:pageSize]
		cursor := page.Items[len(page.Items)-1].DatasetItemID
		page.NextCursor = &cursor
	}
	return page, nil
}

func (s *RunComparisonService) validateRuns(
	ctx context.Context,
	query RunComparisonQuery,
) (EvalRun, EvalRun, error) {
	if err := validateComparisonIdentity(query); err != nil {
		return EvalRun{}, EvalRun{}, err
	}
	baseline, err := s.repository.GetEvalRun(ctx, query.BaselineID)
	if err != nil {
		return EvalRun{}, EvalRun{}, fmt.Errorf("get baseline eval run: %w", err)
	}
	candidate, err := s.repository.GetEvalRun(ctx, query.CandidateID)
	if err != nil {
		return EvalRun{}, EvalRun{}, fmt.Errorf("get candidate eval run: %w", err)
	}
	if err := validateComparisonCompatibility(baseline, candidate, query.Scorer); err != nil {
		return EvalRun{}, EvalRun{}, err
	}
	return baseline, candidate, nil
}

func validateComparisonIdentity(query RunComparisonQuery) error {
	if query.BaselineID == uuid.Nil || query.CandidateID == uuid.Nil || query.Scorer == "" {
		return fmt.Errorf(
			"compare eval runs: %w: IDs and scorer are required",
			ErrInvalid,
		)
	}
	if query.BaselineID == query.CandidateID {
		return fmt.Errorf("compare eval runs: %w: runs must be distinct", ErrInvalid)
	}
	return nil
}

func validateComparisonCompatibility(baseline, candidate EvalRun, scorer string) error {
	if !terminalEvalRun(baseline) || !terminalEvalRun(candidate) {
		return fmt.Errorf(
			"compare eval runs: %w: both runs must be terminal",
			ErrConflict,
		)
	}
	if baseline.ApplicationID != candidate.ApplicationID ||
		baseline.DatasetID != candidate.DatasetID {
		return fmt.Errorf(
			"compare eval runs: %w: runs must share application and dataset",
			ErrInvalid,
		)
	}
	if !slices.Contains(baseline.Scorers, scorer) ||
		!slices.Contains(candidate.Scorers, scorer) {
		return fmt.Errorf(
			"compare eval runs: %w: scorer is not present in both runs",
			ErrInvalid,
		)
	}
	return nil
}

func normalizeComparisonQuery(query *RunComparisonQuery) error {
	if query.Limit == 0 {
		query.Limit = defaultPageLimit
	}
	if query.Limit < 1 || query.Limit > maxPageLimit {
		return fmt.Errorf("compare eval runs: %w: limit must be between 1 and 500", ErrInvalid)
	}
	return nil
}

func terminalEvalRun(run EvalRun) bool {
	return run.Status == EvalStatusSucceeded || run.Status == EvalStatusFailed ||
		run.Status == EvalStatusCanceled
}

func classifyComparisonItems(items []RunComparisonEvidence, scorer string) []RunComparisonRow {
	rows := make([]RunComparisonRow, 0, len(items))
	for _, item := range items {
		kind, delta := classifyComparison(item, scorer)
		rows = append(rows, RunComparisonRow{
			DatasetItemID: item.DatasetItemID, Kind: kind,
			Baseline: item.Baseline, Candidate: item.Candidate, Delta: delta,
		})
	}
	return rows
}

func classifyComparison(item RunComparisonEvidence, scorer string) (string, *float64) {
	if item.Baseline == nil {
		return ComparisonCandidateOnly, nil
	}
	if item.Candidate == nil {
		return ComparisonBaselineOnly, nil
	}
	if comparisonCaseChanged(item) {
		return ComparisonChangedCase, nil
	}
	baselineScore := comparisonScore(item.Baseline, scorer)
	candidateScore := comparisonScore(item.Candidate, scorer)
	if !comparisonScored(item, baselineScore, candidateScore) {
		return ComparisonUnscored, nil
	}
	delta := candidateScore.Value - baselineScore.Value
	return ComparisonMatched, &delta
}

func comparisonCaseChanged(item RunComparisonEvidence) bool {
	return !reflect.DeepEqual(item.Baseline.Item.Input, item.Candidate.Item.Input) ||
		!reflect.DeepEqual(
			item.Baseline.Item.ExpectedOutput,
			item.Candidate.Item.ExpectedOutput,
		)
}

func comparisonScored(item RunComparisonEvidence, baseline, candidate *Score) bool {
	return item.Baseline.Status == EvalStatusSucceeded &&
		item.Candidate.Status == EvalStatusSucceeded && baseline != nil && candidate != nil
}

func comparisonScore(item *EvalRunItem, scorer string) *Score {
	for index := range item.Scores {
		if item.Scores[index].Scorer == scorer {
			return &item.Scores[index]
		}
	}
	return nil
}

func comparisonSummary(aggregate RunComparisonAggregate) RunComparisonSummary {
	summary := RunComparisonSummary{
		N: aggregate.Matched, Matched: aggregate.Matched,
		ChangedCases: aggregate.ChangedCases, BaselineOnly: aggregate.BaselineOnly,
		CandidateOnly: aggregate.CandidateOnly, Unscored: aggregate.Unscored,
	}
	if aggregate.Matched > 0 {
		mean := aggregate.SumDelta / float64(aggregate.Matched)
		summary.MeanDelta = &mean
	}
	return summary
}

func comparisonWarnings(read RunComparisonRead) []string {
	warnings := []string{}
	if read.ContextMismatch {
		warnings = append(warnings, "Context differs between matched historical cases.")
	}
	if read.JudgeConfigMismatch {
		warnings = append(warnings, "Judge model, provider, prompt, or threshold differs between runs.")
	}
	return warnings
}
