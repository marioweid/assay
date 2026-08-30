// Package otlp accepts and maps JSON-encoded OpenTelemetry trace exports.
package otlp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// ExportTraceServiceRequest is the JSON OTLP trace export envelope.
type ExportTraceServiceRequest struct {
	ResourceSpans []resourceSpansJSON `json:"resourceSpans"`
}

type resourceSpansJSON struct {
	Resource   resourceJSON     `json:"resource"`
	ScopeSpans []scopeSpansJSON `json:"scopeSpans"`
}

type resourceJSON struct {
	Attributes []keyValueJSON `json:"attributes"`
}

type scopeSpansJSON struct {
	Spans []spanJSON `json:"spans"`
}

type spanJSON struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId"`
	Name              string          `json:"name"`
	Kind              int32           `json:"kind"`
	StartTimeUnixNano uint64JSON      `json:"startTimeUnixNano"`
	EndTimeUnixNano   uint64JSON      `json:"endTimeUnixNano"`
	Attributes        []keyValueJSON  `json:"attributes"`
	Events            []spanEventJSON `json:"events"`
	Status            statusJSON      `json:"status"`
}

type statusJSON struct {
	Message string `json:"message"`
	Code    int32  `json:"code"`
}

type spanEventJSON struct {
	TimeUnixNano           uint64JSON     `json:"timeUnixNano"`
	Name                   string         `json:"name"`
	Attributes             []keyValueJSON `json:"attributes"`
	DroppedAttributesCount uint32         `json:"droppedAttributesCount"`
}

type keyValueJSON struct {
	Key   string       `json:"key"`
	Value anyValueJSON `json:"value"`
}

type anyValueJSON struct {
	StringValue *string         `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    *int64JSON      `json:"intValue"`
	DoubleValue *float64        `json:"doubleValue"`
	ArrayValue  *arrayValueJSON `json:"arrayValue"`
	KvlistValue *kvListJSON     `json:"kvlistValue"`
	BytesValue  *bytesJSON      `json:"bytesValue"`
}

type arrayValueJSON struct {
	Values []anyValueJSON `json:"values"`
}

type kvListJSON struct {
	Values []keyValueJSON `json:"values"`
}

type uint64JSON uint64
type int64JSON int64
type bytesJSON []byte

func (value *anyValueJSON) UnmarshalJSON(payload []byte) error {
	type wireValue anyValueJSON
	var decoded wireValue
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return err
	}
	variants := []bool{
		decoded.StringValue != nil,
		decoded.BoolValue != nil,
		decoded.IntValue != nil,
		decoded.DoubleValue != nil,
		decoded.ArrayValue != nil,
		decoded.KvlistValue != nil,
		decoded.BytesValue != nil,
	}
	count := 0
	for _, present := range variants {
		if present {
			count++
		}
	}
	if count > 1 {
		return errors.New("decode OTLP AnyValue: multiple value variants")
	}
	*value = anyValueJSON(decoded)
	return nil
}

// DecodeJSON decodes the protobuf-defined OTLP JSON representation without protobuf transport.
func DecodeJSON(payload []byte) (*ExportTraceServiceRequest, error) {
	if trimmed := bytes.TrimSpace(payload); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("decode OTLP JSON: root must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var request ExportTraceServiceRequest
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("decode OTLP JSON: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return nil, err
	}
	return &request, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode OTLP JSON trailer: %w", err)
	}
	return errors.New("decode OTLP JSON: multiple JSON values")
}

func (value *uint64JSON) UnmarshalJSON(payload []byte) error {
	parsed, err := parseJSONInteger(payload, 64)
	if err != nil {
		return err
	}
	*value = uint64JSON(parsed)
	return nil
}

func (value *int64JSON) UnmarshalJSON(payload []byte) error {
	text := unquotedNumber(payload)
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("decode signed OTLP integer: %w", err)
	}
	*value = int64JSON(parsed)
	return nil
}

func (value *bytesJSON) UnmarshalJSON(payload []byte) error {
	var encoded string
	if err := json.Unmarshal(payload, &encoded); err != nil {
		return fmt.Errorf("decode OTLP bytes value: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode OTLP bytes value: %w", err)
	}
	*value = decoded
	return nil
}

func parseJSONInteger(payload []byte, bits int) (uint64, error) {
	parsed, err := strconv.ParseUint(unquotedNumber(payload), 10, bits)
	if err != nil {
		return 0, fmt.Errorf("decode unsigned OTLP integer: %w", err)
	}
	return parsed, nil
}

func unquotedNumber(payload []byte) string {
	return string(bytes.Trim(bytes.TrimSpace(payload), `"`))
}
