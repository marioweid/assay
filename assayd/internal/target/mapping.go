package target

import (
	"fmt"
	"strings"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/ohler55/ojg/jp"
)

// Mapping contains compiled target response paths.
type Mapping struct {
	output     jp.Expr
	context    jp.Expr
	hasContext bool
}

// Compile validates and compiles target response paths.
func Compile(config domain.ResponseMapping) (Mapping, error) {
	output, err := jp.ParseString(strings.TrimSpace(config.Output))
	if err != nil {
		return Mapping{}, fmt.Errorf("compile target output mapping: %w", err)
	}
	mapping := Mapping{output: output}
	contextPath := strings.TrimSpace(config.Context)
	if contextPath == "" {
		return mapping, nil
	}
	mapping.context, err = jp.ParseString(contextPath)
	if err != nil {
		return Mapping{}, fmt.Errorf("compile target context mapping: %w", err)
	}
	mapping.hasContext = true
	return mapping, nil
}

// Extract maps a decoded target response into generated scorer input.
func (m Mapping) Extract(response any, fallback []domain.Chunk) (domain.Generation, error) {
	outputs := m.output.Get(response)
	if len(outputs) != 1 {
		return domain.Generation{}, fmt.Errorf(
			"map target output: %w: expected one string", domain.ErrInvalid,
		)
	}
	output, ok := outputs[0].(string)
	output = strings.TrimSpace(output)
	if !ok || output == "" {
		return domain.Generation{}, fmt.Errorf(
			"map target output: %w: expected one non-blank string", domain.ErrInvalid,
		)
	}
	context := append([]domain.Chunk(nil), fallback...)
	if m.hasContext {
		var err error
		context, err = mappedContext(m.context.Get(response))
		if err != nil {
			return domain.Generation{}, err
		}
	}
	return domain.Generation{Output: output, Context: context}, nil
}

func mappedContext(values []any) ([]domain.Chunk, error) {
	chunks := make([]domain.Chunk, 0, len(values))
	for index, value := range values {
		text, ok := value.(string)
		text = strings.TrimSpace(text)
		if !ok || text == "" {
			return nil, fmt.Errorf(
				"map target context item %d: %w: expected non-blank string",
				index, domain.ErrInvalid,
			)
		}
		chunks = append(chunks, domain.Chunk{ID: fmt.Sprintf("k%d", index), Text: text})
	}
	return chunks, nil
}
