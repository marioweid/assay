package domain

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestClassifyRunComparisonOutcomes(t *testing.T) {
	scorer := ScorerGroundedness
	cases := []struct {
		name      string
		evidence  RunComparisonEvidence
		wantKind  string
		wantDelta *float64
	}{
		{
			name: "positive delta", evidence: comparisonEvidence(0.4, 0.9),
			wantKind: ComparisonMatched, wantDelta: floatPointer(0.5),
		},
		{
			name: "negative delta", evidence: comparisonEvidence(0.8, 0.2),
			wantKind: ComparisonMatched, wantDelta: floatPointer(-0.6),
		},
		{
			name: "zero scores remain scored", evidence: comparisonEvidence(0, 0),
			wantKind: ComparisonMatched, wantDelta: floatPointer(0),
		},
		{
			name:     "candidate only",
			evidence: RunComparisonEvidence{Candidate: comparisonItem(0.9)},
			wantKind: ComparisonCandidateOnly,
		},
		{
			name:     "baseline only",
			evidence: RunComparisonEvidence{Baseline: comparisonItem(0.4)},
			wantKind: ComparisonBaselineOnly,
		},
		{name: "execution failure", evidence: failedComparisonEvidence(), wantKind: ComparisonUnscored},
		{name: "changed case", evidence: changedComparisonEvidence(), wantKind: ComparisonChangedCase},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			kind, delta := classifyComparison(test.evidence, scorer)
			if kind != test.wantKind {
				t.Fatalf("kind = %q, want %q", kind, test.wantKind)
			}
			assertOptionalFloat(t, delta, test.wantDelta)
		})
	}
}

func TestComparisonSummaryUsesAllMatchedSamples(t *testing.T) {
	summary := comparisonSummary(RunComparisonAggregate{
		SumDelta: -0.1, Matched: 3, ChangedCases: 1,
		BaselineOnly: 1, CandidateOnly: 1, Unscored: 1,
	})
	mean := -0.1 / 3
	want := RunComparisonSummary{
		N: 3, MeanDelta: &mean, Matched: 3, ChangedCases: 1,
		BaselineOnly: 1, CandidateOnly: 1, Unscored: 1,
	}
	if !reflect.DeepEqual(summary, want) {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
	if empty := comparisonSummary(RunComparisonAggregate{}); !reflect.DeepEqual(
		empty,
		RunComparisonSummary{},
	) {
		t.Fatalf("empty summary = %+v", empty)
	}
}

func TestCompareValidatesPairAndPaginates(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	baselineID := uuid.Must(uuid.NewV7())
	candidateID := uuid.Must(uuid.NewV7())
	firstID := uuid.Must(uuid.NewV7())
	secondID := uuid.Must(uuid.NewV7())
	repository := &comparisonRepositoryStub{
		runs: map[uuid.UUID]EvalRun{
			baselineID:  comparisonRun(baselineID, applicationID, datasetID),
			candidateID: comparisonRun(candidateID, applicationID, datasetID),
		},
		read: RunComparisonRead{
			Items: []RunComparisonEvidence{
				{DatasetItemID: firstID, Baseline: comparisonItem(0.4), Candidate: comparisonItem(0.9)},
				{DatasetItemID: secondID, Baseline: comparisonItem(0.8), Candidate: comparisonItem(0.2)},
			},
			Aggregate:       RunComparisonAggregate{SumDelta: -0.1, Matched: 2},
			ContextMismatch: true, JudgeConfigMismatch: true,
		},
	}
	service := NewRunComparisonService(repository)
	page, err := service.Compare(t.Context(), RunComparisonQuery{
		BaselineID: baselineID, CandidateID: candidateID, Scorer: ScorerGroundedness, Limit: 1,
	})
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if repository.query.Limit != 2 || len(page.Items) != 1 || page.NextCursor == nil ||
		*page.NextCursor != firstID {
		t.Fatalf("page = %+v, repository limit = %d", page, repository.query.Limit)
	}
	if len(page.Warnings) != 2 || page.Summary.N != 2 {
		t.Fatalf("warnings/summary = %v / %+v", page.Warnings, page.Summary)
	}
}

func TestCompareRejectsInvalidPairs(t *testing.T) {
	applicationID := uuid.Must(uuid.NewV7())
	datasetID := uuid.Must(uuid.NewV7())
	baselineID := uuid.Must(uuid.NewV7())
	candidateID := uuid.Must(uuid.NewV7())
	tests := []struct {
		name      string
		baseline  EvalRun
		candidate EvalRun
		query     RunComparisonQuery
		want      error
	}{
		{
			name: "same run",
			query: RunComparisonQuery{
				BaselineID: baselineID, CandidateID: baselineID, Scorer: ScorerGroundedness,
			},
			want: ErrInvalid,
		},
		{
			name: "active", baseline: comparisonRun(baselineID, applicationID, datasetID),
			candidate: activeComparisonRun(candidateID, applicationID, datasetID),
			want:      ErrConflict,
		},
		{
			name:      "different application",
			baseline:  comparisonRun(baselineID, applicationID, datasetID),
			candidate: comparisonRun(candidateID, uuid.Must(uuid.NewV7()), datasetID),
			want:      ErrInvalid,
		},
		{
			name:      "different dataset",
			baseline:  comparisonRun(baselineID, applicationID, datasetID),
			candidate: comparisonRun(candidateID, applicationID, uuid.Must(uuid.NewV7())),
			want:      ErrInvalid,
		},
		{
			name:      "missing scorer",
			baseline:  comparisonRun(baselineID, applicationID, datasetID),
			candidate: comparisonRun(candidateID, applicationID, datasetID),
			query:     RunComparisonQuery{Scorer: ScorerCorrectness}, want: ErrInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := test.query
			if query.BaselineID == uuid.Nil {
				query.BaselineID, query.CandidateID = baselineID, candidateID
			}
			if query.Scorer == "" {
				query.Scorer = ScorerGroundedness
			}
			repository := &comparisonRepositoryStub{runs: map[uuid.UUID]EvalRun{
				baselineID: test.baseline, candidateID: test.candidate,
			}}
			_, err := NewRunComparisonService(repository).Compare(t.Context(), query)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

type comparisonRepositoryStub struct {
	runs  map[uuid.UUID]EvalRun
	read  RunComparisonRead
	query RunComparisonQuery
}

func (r *comparisonRepositoryStub) GetEvalRun(_ context.Context, id uuid.UUID) (EvalRun, error) {
	run, found := r.runs[id]
	if !found {
		return EvalRun{}, ErrNotFound
	}
	return run, nil
}

func (r *comparisonRepositoryStub) CompareEvalRuns(
	_ context.Context,
	query RunComparisonQuery,
) (RunComparisonRead, error) {
	r.query = query
	return r.read, nil
}

func comparisonEvidence(baseline, candidate float64) RunComparisonEvidence {
	return RunComparisonEvidence{
		Baseline: comparisonItem(baseline), Candidate: comparisonItem(candidate),
	}
}

func comparisonItem(value float64) *EvalRunItem {
	reference := "reference"
	return &EvalRunItem{
		Status: EvalStatusSucceeded,
		Item:   DatasetItem{Input: map[string]any{"question": "q"}, ExpectedOutput: &reference},
		Scores: []Score{{Scorer: ScorerGroundedness, Value: value}},
	}
}

func failedComparisonEvidence() RunComparisonEvidence {
	baseline := comparisonItem(0.4)
	candidate := comparisonItem(0.9)
	candidate.Status = EvalStatusFailed
	candidate.Scores = nil
	return RunComparisonEvidence{Baseline: baseline, Candidate: candidate}
}

func changedComparisonEvidence() RunComparisonEvidence {
	evidence := comparisonEvidence(0.4, 0.9)
	evidence.Candidate.Item.Input = map[string]any{"question": "changed"}
	return evidence
}

func comparisonRun(id, applicationID, datasetID uuid.UUID) EvalRun {
	return EvalRun{
		ID: id, ApplicationID: applicationID, DatasetID: datasetID,
		Status: EvalStatusSucceeded, Scorers: []string{ScorerGroundedness},
	}
}

func activeComparisonRun(id, applicationID, datasetID uuid.UUID) EvalRun {
	run := comparisonRun(id, applicationID, datasetID)
	run.Status = EvalStatusRunning
	return run
}

func floatPointer(value float64) *float64 { return &value }

func assertOptionalFloat(t *testing.T, got, want *float64) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("delta = %v, want %v", got, want)
		}
		return
	}
	if math.Abs(*got-*want) > 1e-12 {
		t.Fatalf("delta = %v, want %v", *got, *want)
	}
}
