package scoring

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/marioweid/assay/assayd/internal/domain"
)

type claim struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type excludedContent struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

type extractionResponse struct {
	Claims   []claim           `json:"claims"`
	Excluded []excludedContent `json:"excluded"`
}

type verdict struct {
	ID                 string   `json:"id"`
	Verdict            string   `json:"verdict"`
	SupportingChunkIDs []string `json:"supporting_chunk_ids"`
	Reason             string   `json:"reason"`
}

type verificationResponse struct {
	Verdicts []verdict `json:"verdicts"`
}

type groundednessScorer struct{}

func (groundednessScorer) Name() string     { return domain.ScorerGroundedness }
func (groundednessScorer) PromptID() string { return domain.GroundednessPromptV1 }

func (groundednessScorer) Score(
	ctx context.Context,
	input ScoreInput,
	judge Judge,
) (ScoreResult, error) {
	extraction, tokens, err := completeStructured(
		ctx, judge,
		JudgeRequest{System: groundednessExtractionPrompt, User: map[string]string{
			"question": input.Input, "answer": input.Output,
		}},
		validateExtraction,
	)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("extract groundedness claims: %w", err)
	}
	if len(extraction.Claims) == 0 {
		return ScoreResult{
			Value: 1, Rationale: "No factual claims required verification.",
			Details:     map[string]any{"claims": extraction.Claims, "excluded": extraction.Excluded},
			JudgeTokens: tokens,
		}, nil
	}
	verification, verifyTokens, err := completeStructured(
		ctx, judge,
		JudgeRequest{System: groundednessVerificationPrompt, User: map[string]any{
			"context_chunks": input.Context, "claims": extraction.Claims,
		}},
		func(value verificationResponse) error {
			return validateVerification(value, extraction.Claims, input.Context)
		},
	)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("verify groundedness claims: %w", err)
	}
	return groundednessResult(extraction, verification, tokens+verifyTokens), nil
}

func validateExtraction(value extractionResponse) error {
	if value.Claims == nil || value.Excluded == nil {
		return errors.New("claims and excluded must be explicit JSON arrays")
	}
	if err := validateClaims(value.Claims); err != nil {
		return err
	}
	return validateExcluded(value.Excluded)
}

func validateClaims(claims []claim) error {
	seen := make(map[string]struct{}, len(claims))
	for _, item := range claims {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Text) == "" {
			return errors.New("claim id and text must not be blank")
		}
		if _, found := seen[item.ID]; found {
			return fmt.Errorf("duplicate claim id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func validateExcluded(excluded []excludedContent) error {
	for _, item := range excluded {
		if strings.TrimSpace(item.Text) == "" || strings.TrimSpace(item.Reason) == "" {
			return errors.New("excluded text and reason must not be blank")
		}
	}
	return nil
}

func validateVerification(
	value verificationResponse,
	claims []claim,
	contextChunks []domain.Chunk,
) error {
	want := make(map[string]struct{}, len(claims))
	for _, item := range claims {
		want[item.ID] = struct{}{}
	}
	chunkIDs := make(map[string]struct{}, len(contextChunks))
	for _, chunk := range contextChunks {
		chunkIDs[chunk.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(value.Verdicts))
	for _, item := range value.Verdicts {
		if _, found := want[item.ID]; !found {
			return fmt.Errorf("unknown verdict claim id %q", item.ID)
		}
		if _, found := seen[item.ID]; found {
			return fmt.Errorf("duplicate verdict claim id %q", item.ID)
		}
		if err := validateVerdictEvidence(item, chunkIDs); err != nil {
			return err
		}
		seen[item.ID] = struct{}{}
	}
	if len(seen) != len(want) {
		return errors.New("verdicts must cover every claim")
	}
	return nil
}

func validateVerdictEvidence(item verdict, chunkIDs map[string]struct{}) error {
	if !validVerdict(item.Verdict) {
		return fmt.Errorf("invalid verdict %q", item.Verdict)
	}
	if strings.TrimSpace(item.Reason) == "" {
		return fmt.Errorf("verdict reason for claim %q must not be blank", item.ID)
	}
	if item.Verdict != "unsupported" && len(item.SupportingChunkIDs) == 0 {
		return fmt.Errorf("verdict for claim %q requires context evidence", item.ID)
	}
	for _, chunkID := range item.SupportingChunkIDs {
		if _, found := chunkIDs[chunkID]; !found {
			return fmt.Errorf("unknown supporting chunk id %q", chunkID)
		}
	}
	return nil
}

func validVerdict(value string) bool {
	return value == "supported" || value == "contradicted" || value == "unsupported"
}

func groundednessResult(
	extraction extractionResponse,
	verification verificationResponse,
	tokens int,
) ScoreResult {
	supported := 0
	contradicted := make([]string, 0)
	unsupported := make([]string, 0)
	for _, item := range verification.Verdicts {
		switch item.Verdict {
		case "supported":
			supported++
		case "contradicted":
			contradicted = append(contradicted, item.ID)
		case "unsupported":
			unsupported = append(unsupported, item.ID)
		}
	}
	total := len(extraction.Claims)
	return ScoreResult{
		Value:     float64(supported) / float64(total),
		Rationale: fmt.Sprintf("%d of %d claims were supported.", supported, total),
		Details: map[string]any{
			"claims": extraction.Claims, "excluded": extraction.Excluded,
			"verdicts": verification.Verdicts, "contradicted_claims": contradicted,
			"unsupported_claims": unsupported,
		},
		JudgeTokens: tokens,
	}
}
