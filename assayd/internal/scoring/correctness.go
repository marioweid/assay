package scoring

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/marioweid/assay/assayd/internal/domain"
)

type referenceFact struct {
	Fact   string `json:"fact"`
	Status string `json:"status"`
}

type correctnessResponse struct {
	ReferenceFacts []referenceFact `json:"reference_facts"`
	Contradictions []string        `json:"contradictions"`
	Reasoning      string          `json:"reasoning"`
	Score          float64         `json:"score"`
}

type correctnessScorer struct{}

func (correctnessScorer) Name() string     { return domain.ScorerCorrectness }
func (correctnessScorer) PromptID() string { return domain.CorrectnessPromptV1 }

func (correctnessScorer) Score(
	ctx context.Context,
	input ScoreInput,
	judge Judge,
) (ScoreResult, error) {
	if strings.TrimSpace(input.Reference) == "" {
		return ScoreResult{}, errors.New("correctness requires a reference answer")
	}
	response, tokens, err := completeStructured(
		ctx, judge,
		JudgeRequest{System: correctnessPrompt, User: map[string]string{
			"question": input.Input, "generated": input.Output, "reference": input.Reference,
		}},
		validateCorrectness,
	)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("judge correctness: %w", err)
	}
	correct, contradicted := correctnessFacts(response)
	value := response.Score
	if contradicted && value > 0.3 {
		value = 0.3
	}
	coverage := 1.0
	if len(response.ReferenceFacts) > 0 {
		coverage = float64(correct) / float64(len(response.ReferenceFacts))
	}
	return ScoreResult{
		Value: value, Rationale: response.Reasoning, JudgeTokens: tokens,
		Details: map[string]any{
			"reference_facts": response.ReferenceFacts,
			"contradictions":  response.Contradictions,
			"coverage":        coverage,
		},
	}, nil
}

func correctnessFacts(response correctnessResponse) (int, bool) {
	correct := 0
	contradicted := len(response.Contradictions) > 0
	for _, fact := range response.ReferenceFacts {
		switch fact.Status {
		case "correct":
			correct++
		case "contradicted":
			contradicted = true
		}
	}
	return correct, contradicted
}

func validateCorrectness(value correctnessResponse) error {
	if len(value.ReferenceFacts) == 0 {
		return errors.New("correctness requires at least one reference fact")
	}
	if value.Score < 0 || value.Score > 1 {
		return errors.New("correctness score must be between 0 and 1")
	}
	if strings.TrimSpace(value.Reasoning) == "" {
		return errors.New("correctness reasoning must not be blank")
	}
	for _, fact := range value.ReferenceFacts {
		if strings.TrimSpace(fact.Fact) == "" {
			return errors.New("reference fact must not be blank")
		}
		if !validFactStatus(fact.Status) {
			return fmt.Errorf("invalid reference fact status %q", fact.Status)
		}
	}
	return nil
}

func validFactStatus(status string) bool {
	return status == "correct" || status == "contradicted" || status == "missing"
}
