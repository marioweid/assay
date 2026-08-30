package otlp

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
	"github.com/marioweid/assay/assayd/internal/id"
)

// ResourceGroup is one OTLP ResourceSpans envelope and its owning application identity.
type ResourceGroup struct {
	resource resourceJSON
	scopes   []scopeSpansJSON
}

// Rejections describes spans that could not be mapped without rejecting valid siblings.
type Rejections struct {
	Spans   int64
	Reasons []string
}

// Groups returns resource envelopes in request order.
func Groups(request *ExportTraceServiceRequest) []ResourceGroup {
	groups := make([]ResourceGroup, 0, len(request.ResourceSpans))
	for _, resourceSpans := range request.ResourceSpans {
		groups = append(groups, ResourceGroup{
			resource: resourceSpans.Resource,
			scopes:   resourceSpans.ScopeSpans,
		})
	}
	return groups
}

// Slug returns the preferred Assay slug or the service-name fallback.
func (g ResourceGroup) Slug() string {
	attributes := attributesToMap(g.resource.Attributes)
	if slug, ok := attributes["assay.application.slug"].(string); ok && strings.TrimSpace(slug) != "" {
		return strings.TrimSpace(slug)
	}
	if service, ok := attributes["service.name"].(string); ok {
		return strings.TrimSpace(service)
	}
	return ""
}

// SpanCount returns the number of spans carried by the resource envelope.
func (g ResourceGroup) SpanCount() int {
	count := 0
	for _, scope := range g.scopes {
		count += len(scope.Spans)
	}
	return count
}

// MapResourceSpans maps valid spans and reports invalid siblings as partial rejections.
func MapResourceSpans(
	group ResourceGroup,
	application domain.Application,
) ([]domain.Trace, Rejections, error) {
	resourceAttributes := attributesToMap(group.resource.Attributes)
	traces := make(map[[16]byte]*domain.Trace)
	var rejected Rejections
	for _, scope := range group.scopes {
		for _, otelSpan := range scope.Spans {
			span, traceID, err := mapSpan(otelSpan, application.ID, resourceAttributes)
			if err != nil {
				rejected.Spans++
				rejected.Reasons = append(rejected.Reasons, err.Error())
				continue
			}
			trace, found := traces[traceID]
			if !found {
				trace, err = newTrace(traceID, application.ID)
				if err != nil {
					return nil, rejected, err
				}
				traces[traceID] = trace
			}
			trace.Spans = append(trace.Spans, span)
		}
	}
	return finishTraces(traces), rejected, nil
}

func mapSpan(
	span spanJSON,
	applicationID [16]byte,
	resourceAttributes map[string]any,
) (domain.Span, [16]byte, error) {
	traceID, err := validateSpan(span)
	if err != nil {
		return domain.Span{}, traceID, err
	}
	attributes := mergedAttributes(resourceAttributes, attributesToMap(span.Attributes))
	events, err := mapEvents(span.Events)
	if err != nil {
		return domain.Span{}, traceID, fmt.Errorf("span %q events: %w", span.Name, err)
	}
	mapped := domain.Span{
		ApplicationID:   applicationID,
		Name:            span.Name,
		Kind:            spanKindName(span.Kind),
		OperationName:   stringAttribute(attributes, "gen_ai.operation.name"),
		StartTime:       time.Unix(0, int64(span.StartTimeUnixNano)).UTC(),
		EndTime:         time.Unix(0, int64(span.EndTimeUnixNano)).UTC(),
		DurationMS:      int64(span.EndTimeUnixNano-span.StartTimeUnixNano) / int64(time.Millisecond),
		StatusCode:      statusCodeName(span.Status.Code),
		StatusMessage:   span.Status.Message,
		IsScorable:      boolAttribute(attributes, "assay.scorable"),
		ScorableKind:    stringAttribute(attributes, "assay.scorable.kind"),
		Attributes:      attributes,
		Events:          events,
		InputTokens:     nonnegativeIntAttribute(attributes, "gen_ai.usage.input_tokens"),
		OutputTokens:    nonnegativeIntAttribute(attributes, "gen_ai.usage.output_tokens"),
		ReferenceAnswer: optionalStringAttribute(attributes, "assay.reference.answer"),
	}
	spanID, _ := decodeHexID(span.SpanID, 8)
	copy(mapped.OTelSpanID[:], spanID)
	if span.ParentSpanID != "" {
		mapped.ParentSpanID = &[8]byte{}
		parentID, _ := decodeHexID(span.ParentSpanID, 8)
		copy(mapped.ParentSpanID[:], parentID)
	}
	return mapped, traceID, nil
}

func validateSpan(span spanJSON) ([16]byte, error) {
	var traceID [16]byte
	traceBytes, err := decodeHexID(span.TraceID, len(traceID))
	if err != nil || allZero(traceBytes) {
		return traceID, fmt.Errorf("span %q has invalid trace ID", span.Name)
	}
	copy(traceID[:], traceBytes)
	spanBytes, err := decodeHexID(span.SpanID, 8)
	if err != nil || allZero(spanBytes) {
		return traceID, fmt.Errorf("span %q has invalid span ID", span.Name)
	}
	if !validTimestampRange(uint64(span.StartTimeUnixNano), uint64(span.EndTimeUnixNano)) {
		return traceID, fmt.Errorf("span %q has invalid timestamps", span.Name)
	}
	if err := validateParentSpanID(span); err != nil {
		return traceID, err
	}
	return traceID, nil
}

func validateParentSpanID(span spanJSON) error {
	if span.ParentSpanID == "" {
		return nil
	}
	parentBytes, err := decodeHexID(span.ParentSpanID, 8)
	if err != nil || allZero(parentBytes) {
		return fmt.Errorf("span %q has invalid parent span ID", span.Name)
	}
	return nil
}

func decodeHexID(value string, size int) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != size {
		return nil, errors.New("invalid hexadecimal ID")
	}
	return decoded, nil
}

func validTimestampRange(start uint64, end uint64) bool {
	return start <= math.MaxInt64 && end <= math.MaxInt64 && end >= start
}

func newTrace(traceID [16]byte, applicationID [16]byte) (*domain.Trace, error) {
	traceUUID, err := id.New()
	if err != nil {
		return nil, fmt.Errorf("generate trace ID: %w", err)
	}
	return &domain.Trace{
		ID:            traceUUID,
		ApplicationID: applicationID,
		OTelTraceID:   traceID,
	}, nil
}

func finishTraces(grouped map[[16]byte]*domain.Trace) []domain.Trace {
	traces := make([]domain.Trace, 0, len(grouped))
	for _, trace := range grouped {
		sort.Slice(trace.Spans, func(left int, right int) bool {
			if trace.Spans[left].StartTime.Equal(trace.Spans[right].StartTime) {
				return string(trace.Spans[left].OTelSpanID[:]) < string(trace.Spans[right].OTelSpanID[:])
			}
			return trace.Spans[left].StartTime.Before(trace.Spans[right].StartTime)
		})
		summarizeTrace(trace)
		traces = append(traces, *trace)
	}
	sort.Slice(traces, func(left int, right int) bool {
		return string(traces[left].OTelTraceID[:]) < string(traces[right].OTelTraceID[:])
	})
	return traces
}

func summarizeTrace(trace *domain.Trace) {
	root := 0
	for index, span := range trace.Spans {
		if betterRoot(span, trace.Spans[root]) {
			root = index
		}
		trace.TotalTokens += span.InputTokens + span.OutputTokens
		if trace.ReferenceAnswer == nil && span.ReferenceAnswer != nil {
			trace.ReferenceAnswer = span.ReferenceAnswer
		}
	}
	rootSpan := trace.Spans[root]
	trace.RootName = rootSpan.Name
	trace.StartTime = trace.Spans[0].StartTime
	trace.EndTime = trace.Spans[0].EndTime
	for _, span := range trace.Spans[1:] {
		if span.StartTime.Before(trace.StartTime) {
			trace.StartTime = span.StartTime
		}
		if span.EndTime.After(trace.EndTime) {
			trace.EndTime = span.EndTime
		}
	}
	trace.Status = rootSpan.StatusCode
	trace.SpanCount = len(trace.Spans)
	trace.Attributes = rootSpan.Attributes
}

func betterRoot(candidate domain.Span, current domain.Span) bool {
	if candidate.ParentSpanID == nil && current.ParentSpanID != nil {
		return true
	}
	if (candidate.ParentSpanID == nil) == (current.ParentSpanID == nil) {
		return candidate.StartTime.Before(current.StartTime)
	}
	return false
}

func attributesToMap(attributes []keyValueJSON) map[string]any {
	values := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		values[attribute.Key] = anyValue(attribute.Value)
	}
	return values
}

func anyValue(value anyValueJSON) any {
	switch {
	case value.StringValue != nil:
		return *value.StringValue
	case value.BoolValue != nil:
		return *value.BoolValue
	case value.IntValue != nil:
		return int64(*value.IntValue)
	case value.DoubleValue != nil:
		return *value.DoubleValue
	case value.BytesValue != nil:
		return base64.StdEncoding.EncodeToString(*value.BytesValue)
	default:
		return collectionAnyValue(value)
	}
}

func collectionAnyValue(value anyValueJSON) any {
	switch {
	case value.ArrayValue != nil:
		items := make([]any, 0, len(value.ArrayValue.Values))
		for _, item := range value.ArrayValue.Values {
			items = append(items, anyValue(item))
		}
		return items
	case value.KvlistValue != nil:
		return attributesToMap(value.KvlistValue.Values)
	default:
		return nil
	}
}

func mergedAttributes(resource map[string]any, span map[string]any) map[string]any {
	merged := make(map[string]any, len(resource)+len(span))
	for key, value := range resource {
		merged[key] = value
	}
	for key, value := range span {
		merged[key] = value
	}
	return merged
}

func mapEvents(events []spanEventJSON) ([]domain.SpanEvent, error) {
	mapped := make([]domain.SpanEvent, 0, len(events))
	for _, event := range events {
		if event.TimeUnixNano > math.MaxInt64 {
			return nil, errors.New("timestamp exceeds supported range")
		}
		mapped = append(mapped, domain.SpanEvent{
			Time:                   time.Unix(0, int64(event.TimeUnixNano)).UTC(),
			Name:                   event.Name,
			Attributes:             attributesToMap(event.Attributes),
			DroppedAttributesCount: event.DroppedAttributesCount,
		})
	}
	return mapped, nil
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func spanKindName(kind int32) string {
	switch kind {
	case 0:
		return "unspecified"
	case 1:
		return "internal"
	case 2:
		return "server"
	case 3:
		return "client"
	case 4:
		return "producer"
	case 5:
		return "consumer"
	default:
		return fmt.Sprintf("unknown_%d", kind)
	}
}

func statusCodeName(code int32) string {
	switch code {
	case 0:
		return "unset"
	case 1:
		return "ok"
	case 2:
		return "error"
	default:
		return fmt.Sprintf("unknown_%d", code)
	}
}

func stringAttribute(attributes map[string]any, key string) string {
	value, _ := attributes[key].(string)
	return value
}

func optionalStringAttribute(attributes map[string]any, key string) *string {
	value, ok := attributes[key].(string)
	if !ok {
		return nil
	}
	return &value
}

func boolAttribute(attributes map[string]any, key string) bool {
	value, _ := attributes[key].(bool)
	return value
}

func nonnegativeIntAttribute(attributes map[string]any, key string) int64 {
	value, ok := attributes[key].(int64)
	if !ok || value < 0 {
		return 0
	}
	return value
}
