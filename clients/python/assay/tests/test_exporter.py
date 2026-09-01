import json
from pathlib import Path

import httpx
import pytest
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import Event, ReadableSpan
from opentelemetry.sdk.trace.export import SpanExportResult
from opentelemetry.sdk.util.instrumentation import InstrumentationScope
from opentelemetry.trace import (
    Link,
    SpanContext,
    SpanKind,
    Status,
    StatusCode,
    TraceFlags,
    TraceState,
)

from assay._exporter import AssaySpanExporter, build_export_request

TRACE_ID = 0x00112233445566778899AABBCCDDEEFF
SPAN_ID = 0x0102030405060708
PARENT_SPAN_ID = 0x1112131415161718
LINK_TRACE_ID = 0xFFEEDDCCBBAA99887766554433221100
LINK_SPAN_ID = 0x2122232425262728
FIXTURE = Path(__file__).parent / "fixtures" / "otlp_trace.json"


def make_span() -> ReadableSpan:
    context = SpanContext(
        trace_id=TRACE_ID,
        span_id=SPAN_ID,
        is_remote=False,
        trace_flags=TraceFlags(TraceFlags.SAMPLED),
        trace_state=TraceState([("vendor", "state")]),
    )
    parent = SpanContext(
        trace_id=TRACE_ID,
        span_id=PARENT_SPAN_ID,
        is_remote=True,
        trace_flags=TraceFlags(TraceFlags.SAMPLED),
    )
    link_context = SpanContext(
        trace_id=LINK_TRACE_ID,
        span_id=LINK_SPAN_ID,
        is_remote=True,
        trace_flags=TraceFlags(TraceFlags.SAMPLED),
        trace_state=TraceState([("link", "value")]),
    )
    return ReadableSpan(
        name="chat model",
        context=context,
        parent=parent,
        resource=Resource(
            {"assay.application.slug": "support-bot", "replicas": 2},
            schema_url="https://resource.schema.test",
        ),
        attributes={
            "gen_ai.operation.name": "chat",
            "gen_ai.usage.input_tokens": 4,
            "gen_ai.input.messages": ('[{"role":"user","content":"What is Assay?"}]'),
            "gen_ai.output.messages": (
                '[{"role":"assistant","content":"Assay evaluates AI systems."}]'
            ),
            "assay.scorable": True,
            "assay.scorable.kind": "rag_answer",
            "assay.context.chunk.count": 1,
            "assay.context.chunks.0.id": "k0",
            "assay.context.chunks.0.text": "Assay evaluates AI systems.",
            "assay.reference.answer": "Assay evaluates AI systems.",
            "tags": ("one", "two"),
        },
        events=(Event("answer.ready", {"cached": False}, timestamp=1_787_911_200_500_000_000),),
        links=(Link(link_context, {"reason": "retry"}),),
        kind=SpanKind.CLIENT,
        status=Status(StatusCode.ERROR, "model failed"),
        start_time=1_787_911_200_000_000_000,
        end_time=1_787_911_201_000_000_000,
        instrumentation_scope=InstrumentationScope(
            "assay.tests",
            "0.2.0",
            "https://scope.schema.test",
            {"scope.flag": True},
        ),
    )


def test_build_export_request_maps_complete_public_span() -> None:
    request = build_export_request((make_span(),))

    resource_spans = request["resourceSpans"]
    assert isinstance(resource_spans, list)
    assert len(resource_spans) == 1
    resource_group = resource_spans[0]
    assert resource_group["schemaUrl"] == "https://resource.schema.test"
    assert resource_group["resource"] == {
        "attributes": [
            {
                "key": "assay.application.slug",
                "value": {"stringValue": "support-bot"},
            },
            {"key": "replicas", "value": {"intValue": "2"}},
        ],
        "droppedAttributesCount": 0,
    }

    scope_group = resource_group["scopeSpans"][0]
    assert scope_group["schemaUrl"] == "https://scope.schema.test"
    assert scope_group["scope"] == {
        "name": "assay.tests",
        "version": "0.2.0",
        "attributes": [{"key": "scope.flag", "value": {"boolValue": True}}],
        "droppedAttributesCount": 0,
    }

    span = scope_group["spans"][0]
    assert span["traceId"] == "00112233445566778899aabbccddeeff"
    assert span["spanId"] == "0102030405060708"
    assert span["parentSpanId"] == "1112131415161718"
    assert span["traceState"] == "vendor=state"
    assert span["flags"] == 1
    assert span["kind"] == 3
    assert span["startTimeUnixNano"] == "1787911200000000000"
    assert span["endTimeUnixNano"] == "1787911201000000000"
    assert span["status"] == {"code": 2, "message": "model failed"}
    assert span["droppedAttributesCount"] == 0
    assert span["droppedEventsCount"] == 0
    assert span["droppedLinksCount"] == 0
    assert span["events"] == [
        {
            "timeUnixNano": "1787911200500000000",
            "name": "answer.ready",
            "attributes": [{"key": "cached", "value": {"boolValue": False}}],
            "droppedAttributesCount": 0,
        }
    ]
    assert span["links"] == [
        {
            "traceId": "ffeeddccbbaa99887766554433221100",
            "spanId": "2122232425262728",
            "traceState": "link=value",
            "flags": 1,
            "attributes": [{"key": "reason", "value": {"stringValue": "retry"}}],
            "droppedAttributesCount": 0,
        }
    ]


def test_build_export_request_groups_matching_resources_and_scopes() -> None:
    first = make_span()
    second = ReadableSpan(
        name="second",
        context=SpanContext(
            trace_id=TRACE_ID + 1,
            span_id=SPAN_ID + 1,
            is_remote=False,
            trace_flags=TraceFlags(TraceFlags.DEFAULT),
        ),
        resource=first.resource,
        instrumentation_scope=first.instrumentation_scope,
        start_time=1,
        end_time=2,
    )

    request = build_export_request((first, second))

    resource_spans = request["resourceSpans"]
    assert isinstance(resource_spans, list)
    assert len(resource_spans) == 1
    assert len(resource_spans[0]["scopeSpans"][0]["spans"]) == 2


def test_checked_in_fixture_matches_exporter_output() -> None:
    expected = json.loads(FIXTURE.read_text(encoding="utf-8"))

    assert build_export_request((make_span(),)) == expected


@pytest.mark.parametrize(
    ("status_code", "body", "expected"),
    [
        (200, {}, SpanExportResult.SUCCESS),
        (
            200,
            {"partialSuccess": {"rejectedSpans": "1", "errorMessage": "rejected"}},
            SpanExportResult.FAILURE,
        ),
        (400, {"error": "captured answer"}, SpanExportResult.FAILURE),
        (429, {}, SpanExportResult.FAILURE),
        (503, {}, SpanExportResult.FAILURE),
    ],
)
def test_export_classifies_http_responses(
    status_code: int,
    body: dict[str, object],
    expected: SpanExportResult,
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(status_code, json=body)

    client = httpx.Client(transport=httpx.MockTransport(handler))
    exporter = AssaySpanExporter(
        "https://assay.test/",
        "asy_secret",
        client=client,
    )

    result = exporter.export((make_span(),))

    assert result is expected
    assert len(requests) == 1
    request = requests[0]
    assert request.url == httpx.URL("https://assay.test/v1/traces")
    assert request.headers["content-type"] == "application/json"
    assert request.headers["x-api-key"] == "asy_secret"
    assert request.headers["user-agent"].startswith("assay-sdk/")
    assert json.loads(request.content)["resourceSpans"]
    client.close()


def test_export_returns_failure_for_malformed_success_response() -> None:
    transport = httpx.MockTransport(lambda _: httpx.Response(200, content=b"not-json"))
    client = httpx.Client(transport=transport)
    exporter = AssaySpanExporter("https://assay.test", "asy_secret", client=client)

    assert exporter.export((make_span(),)) is SpanExportResult.FAILURE
    client.close()


def test_export_returns_failure_for_transport_error() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadTimeout("captured answer", request=request)

    client = httpx.Client(transport=httpx.MockTransport(handler))
    exporter = AssaySpanExporter("https://assay.test", "asy_secret", client=client)

    assert exporter.export((make_span(),)) is SpanExportResult.FAILURE
    client.close()


def test_export_rejects_oversized_payload_before_network_io() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json={})

    client = httpx.Client(transport=httpx.MockTransport(handler))
    exporter = AssaySpanExporter(
        "https://assay.test",
        "asy_secret",
        client=client,
        max_request_bytes=1,
    )

    assert exporter.export((make_span(),)) is SpanExportResult.FAILURE
    assert requests == []
    client.close()


def test_shutdown_does_not_close_injected_client_and_stops_exports() -> None:
    client = httpx.Client(transport=httpx.MockTransport(lambda _: httpx.Response(200, json={})))
    exporter = AssaySpanExporter("https://assay.test", "asy_secret", client=client)

    exporter.shutdown()

    assert not client.is_closed
    assert exporter.export((make_span(),)) is SpanExportResult.FAILURE
    client.close()


def test_export_empty_batch_succeeds_without_network_io() -> None:
    requests: list[httpx.Request] = []
    client = httpx.Client(
        transport=httpx.MockTransport(
            lambda request: _record_success(requests, request),
        )
    )
    exporter = AssaySpanExporter("https://assay.test", "asy_secret", client=client)

    assert exporter.export(()) is SpanExportResult.SUCCESS
    assert requests == []
    client.close()


def _record_success(
    requests: list[httpx.Request],
    request: httpx.Request,
) -> httpx.Response:
    requests.append(request)
    return httpx.Response(200, json={})
