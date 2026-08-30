package scoring

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/marioweid/assay/assayd/internal/domain"
)

// ScoreInput contains normalized values supplied to a scorer.
type ScoreInput struct {
	Input     string
	Output    string
	Context   []domain.Chunk
	Reference string
}

// ScoreResult is a scorer's normalized result before persistence.
type ScoreResult struct {
	Value       float64
	Rationale   string
	Details     map[string]any
	JudgeTokens int
}

// Scorer evaluates one dataset item with a judge.
type Scorer interface {
	Name() string
	PromptID() string
	Score(context.Context, ScoreInput, Judge) (ScoreResult, error)
}

// Registry contains the built-in scorers keyed by name.
type Registry struct {
	scorers map[string]Scorer
}

// NewRegistry constructs the built-in scorer registry.
func NewRegistry() Registry {
	return Registry{scorers: map[string]Scorer{
		domain.ScorerGroundedness: groundednessScorer{},
		domain.ScorerCorrectness:  correctnessScorer{},
	}}
}

// Get returns a scorer by its stable name.
func (r Registry) Get(name string) (Scorer, bool) {
	scorer, found := r.scorers[name]
	return scorer, found
}

func completeStructured[T any](
	ctx context.Context,
	judge Judge,
	request JudgeRequest,
	validate func(T) error,
) (T, int, error) {
	tokens := 0
	for attempt := range 2 {
		var value T
		response, err := judge.Complete(ctx, request)
		if err != nil {
			return value, tokens, err
		}
		tokens += response.Tokens
		err = json.Unmarshal([]byte(response.Content), &value)
		if err == nil {
			err = validate(value)
		}
		if err == nil {
			return value, tokens, nil
		}
		if attempt == 0 {
			request.Correction = err.Error()
		}
	}
	var zero T
	return zero, tokens, fmt.Errorf("judge returned invalid structured output after retry")
}
