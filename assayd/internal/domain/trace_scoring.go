package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

const contextChunkPrefix = "assay.context.chunks."

// TraceScoreInput contains the captured content for one online score.
type TraceScoreInput struct {
	Span      Span
	Input     string
	Output    string
	Context   []Chunk
	Reference string
}

// BuildTraceScoreInput validates and extracts scorer content from one scorable span.
func BuildTraceScoreInput(trace Trace, scorer string) (TraceScoreInput, error) {
	input, err := extractTraceScoreInput(trace, scorer)
	if err != nil {
		return TraceScoreInput{}, err
	}
	if err := validateTraceScoreRequirements(input, trace, scorer); err != nil {
		return TraceScoreInput{}, err
	}
	return input, nil
}

func extractTraceScoreInput(trace Trace, scorer string) (TraceScoreInput, error) {
	span, err := oneScorableSpan(trace)
	if err != nil {
		return TraceScoreInput{}, scoreInputError(trace, scorer, "scorable span", err)
	}
	inputMessages, err := messageList(span.Attributes["gen_ai.input.messages"])
	if err != nil {
		return TraceScoreInput{}, scoreInputError(trace, scorer, "gen_ai.input.messages", err)
	}
	outputMessages, err := messageList(span.Attributes["gen_ai.output.messages"])
	if err != nil {
		return TraceScoreInput{}, scoreInputError(trace, scorer, "gen_ai.output.messages", err)
	}
	input, err := selectedMessage(inputMessages, "user", true)
	if err != nil {
		return TraceScoreInput{}, scoreInputError(trace, scorer, "gen_ai.input.messages", err)
	}
	output, err := selectedMessage(outputMessages, "assistant", false)
	if err != nil {
		return TraceScoreInput{}, scoreInputError(trace, scorer, "gen_ai.output.messages", err)
	}
	context, err := traceContext(trace)
	if err != nil {
		return TraceScoreInput{}, scoreInputError(trace, scorer, "context", err)
	}
	return TraceScoreInput{
		Span: span, Input: input, Output: output, Context: context,
		Reference: selectedReference(span.ReferenceAnswer, trace.ReferenceAnswer),
	}, nil
}

func validateTraceScoreRequirements(input TraceScoreInput, trace Trace, scorer string) error {
	switch scorer {
	case ScorerGroundedness:
		if len(input.Context) == 0 {
			return scoreInputError(trace, scorer, "context", errorsRequired())
		}
	case ScorerCorrectness:
		if input.Reference == "" {
			return scoreInputError(trace, scorer, "reference", errorsRequired())
		}
	default:
		return scoreInputError(trace, scorer, "scorer", errorsRequired())
	}
	return nil
}

func oneScorableSpan(trace Trace) (Span, error) {
	var selected Span
	count := 0
	for _, span := range trace.Spans {
		if span.IsScorable {
			selected = span
			count++
		}
	}
	if count != 1 {
		return Span{}, fmt.Errorf("expected exactly one, found %d", count)
	}
	return selected, nil
}

func messageList(value any) ([]any, error) {
	if messages, ok := value.([]any); ok {
		return messages, nil
	}
	encoded, ok := value.(string)
	if !ok || strings.TrimSpace(encoded) == "" {
		return nil, errorsRequired()
	}
	decoder := json.NewDecoder(bytes.NewBufferString(encoded))
	decoder.UseNumber()
	var messages []any
	if err := decoder.Decode(&messages); err != nil {
		return nil, errorsMalformed()
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errorsMalformed()
	}
	if messages == nil {
		return nil, errorsMalformed()
	}
	return messages, nil
}

func selectedMessage(messages []any, role string, last bool) (string, error) {
	selected := ""
	for _, value := range messages {
		message, messageRole, err := parsedMessage(value)
		if err != nil {
			return "", err
		}
		if messageRole != role {
			continue
		}
		content, err := messageContent(message)
		if err != nil {
			return "", err
		}
		if !last {
			return content, nil
		}
		selected = content
	}
	if selected == "" {
		return "", fmt.Errorf("required %s message missing", role)
	}
	return selected, nil
}

func parsedMessage(value any) (map[string]any, string, error) {
	message, ok := value.(map[string]any)
	if !ok {
		return nil, "", errorsMalformed()
	}
	role, ok := message["role"].(string)
	if !ok || strings.TrimSpace(role) == "" {
		return nil, "", errorsMalformed()
	}
	return message, role, nil
}

func messageContent(message map[string]any) (string, error) {
	if content := directMessageContent(message); content != "" {
		return content, nil
	}
	parts, ok := message["parts"].([]any)
	if !ok || len(parts) == 0 {
		return "", errorsMalformed()
	}
	return joinedTextParts(parts)
}

func joinedTextParts(parts []any) (string, error) {
	var content strings.Builder
	for _, value := range parts {
		part, ok := value.(map[string]any)
		if !ok || part["type"] != "text" {
			continue
		}
		text, ok := part["content"].(string)
		if !ok {
			return "", errorsMalformed()
		}
		content.WriteString(text)
	}
	result := strings.TrimSpace(content.String())
	if result == "" {
		return "", errorsMalformed()
	}
	return result, nil
}

func directMessageContent(message map[string]any) string {
	content, _ := message["content"].(string)
	return strings.TrimSpace(content)
}

func traceContext(trace Trace) ([]Chunk, error) {
	flattened, documents, err := collectTraceContext(trace)
	if err != nil {
		return nil, err
	}
	if hasFlattenedContext(flattened) {
		return flattenedContext(flattened)
	}
	return documentChunks(documents)
}

func collectTraceContext(trace Trace) (map[string]any, []any, error) {
	flattened := make(map[string]any)
	var documents []any
	var documentSources []any
	for _, span := range trace.Spans {
		if err := mergeFlattenedContext(flattened, span.Attributes); err != nil {
			return nil, nil, err
		}
		value, found := span.Attributes["gen_ai.retrieval.documents"]
		if !found || containsEqual(documentSources, value) {
			continue
		}
		values, err := messageList(value)
		if err != nil {
			return nil, nil, errorsMalformed()
		}
		documents = append(documents, values...)
		documentSources = append(documentSources, value)
	}
	return flattened, documents, nil
}

func mergeFlattenedContext(destination map[string]any, attributes map[string]any) error {
	for key, value := range attributes {
		if key != "assay.context.chunk.count" && !strings.HasPrefix(key, contextChunkPrefix) {
			continue
		}
		if existing, duplicate := destination[key]; duplicate {
			if !reflect.DeepEqual(existing, value) {
				return errorsMalformed()
			}
			continue
		}
		destination[key] = value
	}
	return nil
}

func containsEqual(values []any, target any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, target) {
			return true
		}
	}
	return false
}

func hasFlattenedContext(attributes map[string]any) bool {
	if _, found := attributes["assay.context.chunk.count"]; found {
		return true
	}
	for key := range attributes {
		if strings.HasPrefix(key, contextChunkPrefix) {
			return true
		}
	}
	return false
}

func flattenedContext(attributes map[string]any) ([]Chunk, error) {
	count, err := integerAttribute(attributes["assay.context.chunk.count"])
	if err != nil || count < 0 {
		return nil, errorsMalformed()
	}
	chunks := make([]Chunk, 0, count)
	seen := make(map[string]struct{}, count)
	for index := range count {
		id, text, err := flattenedChunk(attributes, index)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errorsMalformed()
		}
		seen[id] = struct{}{}
		chunks = append(chunks, Chunk{ID: id, Text: text})
	}
	return chunks, nil
}

func flattenedChunk(attributes map[string]any, index int) (string, string, error) {
	prefix := contextChunkPrefix + strconv.Itoa(index)
	id, idOK := attributes[prefix+".id"].(string)
	text, textOK := attributes[prefix+".text"].(string)
	id, text = strings.TrimSpace(id), strings.TrimSpace(text)
	if !idOK || !textOK || id == "" || text == "" {
		return "", "", errorsMalformed()
	}
	return id, text, nil
}

func integerAttribute(value any) (int, error) {
	switch number := value.(type) {
	case int:
		return number, nil
	case int64:
		return int(number), nil
	case float64:
		if number == float64(int(number)) {
			return int(number), nil
		}
	case json.Number:
		parsed, err := strconv.Atoi(string(number))
		if err == nil {
			return parsed, nil
		}
	}
	return 0, errorsMalformed()
}

func documentChunks(values []any) ([]Chunk, error) {
	chunks := make([]Chunk, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		id, text, err := documentChunk(value, index)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errorsMalformed()
		}
		seen[id] = struct{}{}
		chunks = append(chunks, Chunk{ID: id, Text: text})
	}
	return chunks, nil
}

func documentChunk(value any, index int) (string, string, error) {
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text != "" {
			return fmt.Sprintf("k%d", index), text, nil
		}
		return "", "", errorsMalformed()
	}
	document, ok := value.(map[string]any)
	if !ok {
		return "", "", errorsMalformed()
	}
	return documentObjectChunk(document, index)
}

func documentObjectChunk(document map[string]any, index int) (string, string, error) {
	text, ok := document["text"].(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return "", "", errorsMalformed()
	}
	id := fmt.Sprintf("k%d", index)
	if configured, found := document["id"]; found {
		id, ok = configured.(string)
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			return "", "", errorsMalformed()
		}
	}
	return id, text, nil
}

func selectedReference(spanReference *string, traceReference *string) string {
	if spanReference != nil && strings.TrimSpace(*spanReference) != "" {
		return strings.TrimSpace(*spanReference)
	}
	if traceReference != nil {
		return strings.TrimSpace(*traceReference)
	}
	return ""
}

func scoreInputError(trace Trace, scorer string, field string, err error) error {
	return fmt.Errorf(
		"build trace %s score input for %s field %s: %w",
		trace.ID, scorer, field, errors.Join(ErrInvalid, err),
	)
}

func errorsRequired() error  { return fmt.Errorf("required value missing") }
func errorsMalformed() error { return fmt.Errorf("malformed value") }
