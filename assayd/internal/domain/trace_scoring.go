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

// TraceScoringReason explains why a scorer cannot be queued for a trace.
type TraceScoringReason struct {
	Code    string
	Message string
}

// TraceScoringEligibility reports deterministic scorer readiness for a trace.
type TraceScoringEligibility struct {
	Scorer   string
	Eligible bool
	Reasons  []TraceScoringReason
}

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
	input, reasons := evaluateTraceScoreInput(trace, scorer)
	if len(reasons) > 0 {
		return TraceScoreInput{}, scoreInputError(trace, scorer, reasons[0].Code, errorsRequired())
	}
	return input, nil
}

// EvaluateTraceScoreInput returns all stable content eligibility failures in fixed order.
func EvaluateTraceScoreInput(trace Trace, scorer string) (TraceScoreInput, []TraceScoringReason) {
	return evaluateTraceScoreInput(trace, scorer)
}

func evaluateTraceScoreInput(trace Trace, scorer string) (TraceScoreInput, []TraceScoringReason) {
	span, spanReason := singleScorableSpan(trace)
	if spanReason != "" {
		return TraceScoreInput{}, []TraceScoringReason{eligibilityReason(spanReason)}
	}
	input, inputReason := scorerMessage(
		span.Attributes, "gen_ai.input.messages", "user", true, "missing_input",
	)
	output, outputReason := scorerMessage(
		span.Attributes, "gen_ai.output.messages", "assistant", false, "missing_output",
	)
	context, contextReason := scorerContext(trace)
	reference := selectedReference(span.ReferenceAnswer, trace.ReferenceAnswer)
	result := TraceScoreInput{
		Span: span, Input: input, Output: output, Context: context, Reference: reference,
	}
	return result, traceScoreContentReasons(
		scorer, inputReason, outputReason, contextReason, reference,
	)
}

func singleScorableSpan(trace Trace) (Span, string) {
	var selected Span
	count := 0
	for _, candidate := range trace.Spans {
		if candidate.IsScorable {
			count++
			selected = candidate
		}
	}
	if count == 0 {
		return Span{}, "missing_scorable_span"
	}
	if count > 1 {
		return Span{}, "multiple_scorable_spans"
	}
	return selected, ""
}

func traceScoreContentReasons(
	scorer string,
	inputReason string,
	outputReason string,
	contextReason string,
	reference string,
) []TraceScoringReason {
	codes := []string{inputReason, outputReason}
	if scorer == ScorerGroundedness {
		codes = append(codes, contextReason)
	}
	if scorer == ScorerCorrectness && reference == "" {
		codes = append(codes, "missing_reference")
	}
	if scorer != ScorerGroundedness && scorer != ScorerCorrectness {
		codes = append(codes, "malformed_content")
	}
	reasons := make([]TraceScoringReason, 0, len(codes))
	for _, code := range codes {
		if code != "" {
			reasons = append(reasons, eligibilityReason(code))
		}
	}
	return reasons
}

func scorerMessage(
	attributes map[string]any,
	key string,
	role string,
	last bool,
	missing string,
) (string, string) {
	value, found := attributes[key]
	if !found || (isBlankString(value)) {
		return "", missing
	}
	messages, err := messageList(value)
	if err != nil {
		return "", "malformed_content"
	}
	result, err := selectedMessage(messages, role, last)
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return "", missing
		}
		return "", "malformed_content"
	}
	return result, ""
}

func scorerContext(trace Trace) ([]Chunk, string) {
	context, err := traceContext(trace)
	if err != nil {
		return nil, "malformed_content"
	}
	if len(context) == 0 {
		return []Chunk{}, "missing_context"
	}
	return context, ""
}

func isBlankString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func eligibilityReason(code string) TraceScoringReason {
	messages := map[string]string{
		"missing_scorable_span":   "Trace has no scorable span.",
		"multiple_scorable_spans": "Trace has multiple scorable spans.",
		"missing_input":           "Trace is missing scorer input.",
		"missing_output":          "Trace is missing scorer output.",
		"missing_context":         "Trace is missing retrieval context.",
		"missing_reference":       "Trace is missing a reference answer.",
		"malformed_content":       "Trace scorer content is malformed.",
		"scorer_disabled":         "Scorer is disabled.",
		"missing_judge":           "Scorer judge is not configured.",
	}
	return TraceScoringReason{Code: code, Message: messages[code]}
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
