package otlp

import (
	"testing"

	"github.com/marioweid/assay/assayd/internal/domain"

	"github.com/google/uuid"
)

func TestMapResourceSpansExtractsTraceAndSemanticAttributes(t *testing.T) {
	request, err := DecodeJSON([]byte(traceFixture))
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	groups := Groups(request)
	assertResourceGroup(t, groups)
	application := domain.Application{ID: uuid.Must(uuid.NewV7())}

	traces, rejected, err := MapResourceSpans(groups[0], application)
	if err != nil {
		t.Fatalf("map resource spans: %v", err)
	}
	assertMappedTrace(t, traces, rejected)
}

func assertResourceGroup(t *testing.T, groups []ResourceGroup) {
	t.Helper()
	if len(groups) != 1 {
		t.Fatalf("resource groups = %#v, want one", groups)
	}
	if groups[0].Slug() != "support-bot" {
		t.Fatalf("resource slug = %q, want support-bot", groups[0].Slug())
	}
	if groups[0].SpanCount() != 2 {
		t.Fatalf("resource span count = %d, want 2", groups[0].SpanCount())
	}
}

func assertMappedTrace(t *testing.T, traces []domain.Trace, rejected Rejections) {
	t.Helper()
	if rejected.Spans != 0 {
		t.Fatalf("rejections = %#v, want none", rejected)
	}
	if len(traces) != 1 {
		t.Fatalf("mapped traces = %#v, want one", traces)
	}
	assertTraceSummary(t, traces[0])
	assertMappedChild(t, traces[0].Spans[1])
}

func assertTraceSummary(t *testing.T, trace domain.Trace) {
	t.Helper()
	if trace.RootName != "answer" {
		t.Fatalf("trace summary = %#v", trace)
	}
	if trace.SpanCount != 2 || trace.TotalTokens != 15 {
		t.Fatalf("trace counts = %#v", trace)
	}
	if trace.ReferenceAnswer == nil || *trace.ReferenceAnswer != "reference" {
		t.Fatalf("reference answer = %#v", trace.ReferenceAnswer)
	}
}

func assertMappedChild(t *testing.T, child domain.Span) {
	t.Helper()
	if child.OperationName != "chat" || child.ScorableKind != "rag_answer" {
		t.Fatalf("mapped child = %#v", child)
	}
	if !child.IsScorable {
		t.Fatalf("mapped child is not scorable: %#v", child)
	}
	if child.Attributes["service.name"] != "fallback" || child.Attributes["shared"] != "span" {
		t.Fatalf("merged attributes = %#v", child.Attributes)
	}
	if len(child.Events) != 1 || child.Events[0].Name != "completed" {
		t.Fatalf("mapped events = %#v", child.Events)
	}
}

func TestMapResourceSpansRejectsInvalidSpanAndKeepsValidSpan(t *testing.T) {
	request, err := DecodeJSON([]byte(`{
  "resourceSpans": [{
    "resource": {"attributes": [{"key":"service.name","value":{"stringValue":"app"}}]},
    "scopeSpans": [{"spans": [
      {"traceId":"00","spanId":"01","name":"bad","startTimeUnixNano":"2","endTimeUnixNano":"1"},
      {"traceId":"00112233445566778899aabbccddeeff","spanId":"0102030405060708",
       "name":"good","startTimeUnixNano":"1","endTimeUnixNano":"2"}
    ]}]
  }]
}`))
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	traces, rejected, err := MapResourceSpans(
		Groups(request)[0],
		domain.Application{ID: uuid.Must(uuid.NewV7())},
	)
	if err != nil {
		t.Fatalf("map resource spans: %v", err)
	}
	if rejected.Spans != 1 || len(traces) != 1 || traces[0].RootName != "good" {
		t.Fatalf("mapped traces/rejections = %#v/%#v", traces, rejected)
	}
}

func TestMapResourceSpansRejectsZeroIDsAndOverflowingEventTime(t *testing.T) {
	request, err := DecodeJSON([]byte(`{
  "resourceSpans": [{"scopeSpans": [{"spans": [
    {"traceId":"00000000000000000000000000000000","spanId":"0102030405060708",
     "name":"zero-trace","startTimeUnixNano":"1","endTimeUnixNano":"2"},
    {"traceId":"00112233445566778899aabbccddeeff","spanId":"0102030405060708",
     "name":"event-overflow","startTimeUnixNano":"1","endTimeUnixNano":"2",
     "events":[{"timeUnixNano":"18446744073709551615","name":"bad"}]}
  ]}]}]
}`))
	if err != nil {
		t.Fatalf("decode invalid spans: %v", err)
	}
	traces, rejected, err := MapResourceSpans(
		Groups(request)[0],
		domain.Application{ID: uuid.Must(uuid.NewV7())},
	)
	if err != nil {
		t.Fatalf("map invalid spans: %v", err)
	}
	if rejected.Spans != 2 || len(traces) != 0 {
		t.Fatalf("mapped traces/rejections = %#v/%#v", traces, rejected)
	}
}

func TestGroupsFallsBackToServiceName(t *testing.T) {
	request, err := DecodeJSON([]byte(`{"resourceSpans":[{"resource":{"attributes":[` +
		`{"key":"service.name","value":{"stringValue":"fallback"}}]},"scopeSpans":[]}]}`))
	if err != nil {
		t.Fatalf("decode resource: %v", err)
	}
	if slug := Groups(request)[0].Slug(); slug != "fallback" {
		t.Fatalf("slug = %q, want fallback", slug)
	}
}

var traceFixture = `{
  "resourceSpans": [{
    "resource": {"attributes": [
      {"key":"assay.application.slug","value":{"stringValue":"support-bot"}},
      {"key":"service.name","value":{"stringValue":"fallback"}},
      {"key":"shared","value":{"stringValue":"resource"}}
    ]},
    "scopeSpans": [{"spans": [
      {
        "traceId":"00112233445566778899aabbccddeeff",
        "spanId":"0102030405060708",
        "name":"answer",
        "kind":1,
        "startTimeUnixNano":"1787911200000000000",
        "endTimeUnixNano":"1787911202000000000",
        "status":{"code":1}
      },
      {
        "traceId":"00112233445566778899aabbccddeeff",
        "spanId":"1112131415161718",
        "parentSpanId":"0102030405060708",
        "name":"generation",
        "kind":3,
        "startTimeUnixNano":"1787911200100000000",
        "endTimeUnixNano":"1787911201000000000",
        "status":{"code":1},
        "attributes":[
          {"key":"shared","value":{"stringValue":"span"}},
          {"key":"gen_ai.operation.name","value":{"stringValue":"chat"}},
          {"key":"gen_ai.usage.input_tokens","value":{"intValue":"10"}},
          {"key":"gen_ai.usage.output_tokens","value":{"intValue":"5"}},
          {"key":"assay.scorable","value":{"boolValue":true}},
          {"key":"assay.scorable.kind","value":{"stringValue":"rag_answer"}},
          {"key":"assay.reference.answer","value":{"stringValue":"reference"}}
        ],
        "events":[{"timeUnixNano":"1787911201000000000","name":"completed","attributes":[]}]
      }
    ]}]
  }]
}`
