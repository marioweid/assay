package otlp

import (
	"testing"
)

func TestDecodeJSONAcceptsOTLPHexIDsAndUnknownFields(t *testing.T) {
	payload := []byte(`{
  "unknownFutureField": true,
  "resourceSpans": [{
    "resource": {"attributes": []},
    "scopeSpans": [{"spans": [{
      "traceId": "00112233445566778899AABBCCDDEEFF",
      "spanId": "0102030405060708",
      "name": "answer",
      "kind": 2,
      "startTimeUnixNano": "1787911200000000000",
      "endTimeUnixNano": 1787911201000000000,
      "status": {"code": 1},
      "unknownSpanField": "ignored"
    }]}]
  }]
}`)

	request, err := DecodeJSON(payload)
	if err != nil {
		t.Fatalf("decode OTLP JSON: %v", err)
	}
	span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	if span.TraceID != "00112233445566778899AABBCCDDEEFF" ||
		span.SpanID != "0102030405060708" || span.Kind != 2 {
		t.Fatalf("decoded span = %#v", span)
	}
}

func TestDecodeJSONRejectsEnumNames(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "named kind",
			payload: `{"resourceSpans":[{"scopeSpans":[{"spans":[{` +
				`"traceId":"00112233445566778899aabbccddeeff",` +
				`"spanId":"0102030405060708","kind":"SPAN_KIND_SERVER"}]}]}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeJSON([]byte(test.payload)); err == nil {
				t.Fatal("DecodeJSON succeeded")
			}
		})
	}
}

func TestDecodeJSONRejectsMultipleAnyValueVariants(t *testing.T) {
	payload := []byte(`{"resourceSpans":[{"resource":{"attributes":[{` +
		`"key":"bad","value":{"stringValue":"one","boolValue":true}}]}}]}`)
	if _, err := DecodeJSON(payload); err == nil {
		t.Fatal("DecodeJSON accepted multiple AnyValue variants")
	}
}
