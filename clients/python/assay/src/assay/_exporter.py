"""Private OpenTelemetry JSON/HTTP span exporter for Assay."""

from __future__ import annotations

import json
import threading
from collections.abc import Mapping, Sequence
from importlib import metadata
from typing import cast
from urllib.parse import urlsplit, urlunsplit

import httpx
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import Event, ReadableSpan
from opentelemetry.sdk.trace.export import SpanExporter, SpanExportResult
from opentelemetry.sdk.util.instrumentation import InstrumentationScope
from opentelemetry.trace import Link, SpanContext, SpanKind, StatusCode
from typing_extensions import override

from assay._serialization import otlp_any_value
from assay.exceptions import AssayConfigurationError
from assay.models import AttributeValue

_SPAN_KIND = {
    SpanKind.INTERNAL: 1,
    SpanKind.SERVER: 2,
    SpanKind.CLIENT: 3,
    SpanKind.PRODUCER: 4,
    SpanKind.CONSUMER: 5,
}
_STATUS_CODE = {
    StatusCode.UNSET: 0,
    StatusCode.OK: 1,
    StatusCode.ERROR: 2,
}


def build_export_request(spans: Sequence[ReadableSpan]) -> dict[str, object]:
    """Map completed public SDK spans to an OTLP JSON export request."""
    grouped: dict[
        Resource,
        dict[InstrumentationScope | None, list[ReadableSpan]],
    ] = {}
    for span in spans:
        scopes = grouped.setdefault(span.resource, {})
        scopes.setdefault(span.instrumentation_scope, []).append(span)

    resource_spans = []
    for resource, scopes in grouped.items():
        scope_spans = [
            _scope_spans(scope, grouped_spans) for scope, grouped_spans in scopes.items()
        ]
        group: dict[str, object] = {
            "resource": {
                "attributes": _key_values(resource.attributes),
                "droppedAttributesCount": 0,
            },
            "scopeSpans": scope_spans,
        }
        if resource.schema_url:
            group["schemaUrl"] = resource.schema_url
        resource_spans.append(group)
    return {"resourceSpans": resource_spans}


def _scope_spans(
    scope: InstrumentationScope | None,
    spans: Sequence[ReadableSpan],
) -> dict[str, object]:
    scope_payload: dict[str, object] = {
        "name": scope.name if scope else "",
        "attributes": _key_values(scope.attributes if scope else None),
        "droppedAttributesCount": 0,
    }
    if scope and scope.version:
        scope_payload["version"] = scope.version
    result: dict[str, object] = {
        "scope": scope_payload,
        "spans": [_span(span) for span in spans],
    }
    if scope and scope.schema_url:
        result["schemaUrl"] = scope.schema_url
    return result


def _span(span: ReadableSpan) -> dict[str, object]:
    context = _required_context(span.context)
    if span.start_time is None or span.end_time is None:
        raise TypeError("OpenTelemetry span is missing timestamps")
    result: dict[str, object] = {
        "traceId": _trace_id(context.trace_id),
        "spanId": _span_id(context.span_id),
        "traceState": context.trace_state.to_header(),
        "flags": int(context.trace_flags),
        "name": span.name,
        "kind": _SPAN_KIND[span.kind],
        "startTimeUnixNano": str(span.start_time),
        "endTimeUnixNano": str(span.end_time),
        "attributes": _key_values(span.attributes),
        "droppedAttributesCount": span.dropped_attributes,
        "events": [_event(event) for event in span.events],
        "droppedEventsCount": span.dropped_events,
        "links": [_link(link) for link in span.links],
        "droppedLinksCount": span.dropped_links,
        "status": _status(span.status.status_code, span.status.description),
    }
    if span.parent is not None:
        result["parentSpanId"] = _span_id(span.parent.span_id)
    return result


def _event(event: Event) -> dict[str, object]:
    if event.timestamp is None:
        raise TypeError("OpenTelemetry event is missing a timestamp")
    return {
        "timeUnixNano": str(event.timestamp),
        "name": event.name,
        "attributes": _key_values(event.attributes),
        "droppedAttributesCount": event.dropped_attributes,
    }


def _link(link: Link) -> dict[str, object]:
    context = _required_context(link.context)
    return {
        "traceId": _trace_id(context.trace_id),
        "spanId": _span_id(context.span_id),
        "traceState": context.trace_state.to_header(),
        "flags": int(context.trace_flags),
        "attributes": _key_values(link.attributes),
        "droppedAttributesCount": link.dropped_attributes,
    }


def _status(code: StatusCode, description: str | None) -> dict[str, object]:
    result: dict[str, object] = {"code": _STATUS_CODE[code]}
    if description:
        result["message"] = description
    return result


def _key_values(attributes: Mapping[str, object] | None) -> list[dict[str, object]]:
    if not attributes:
        return []
    return [
        {
            "key": key,
            "value": otlp_any_value(cast(AttributeValue, attributes[key])),
        }
        for key in sorted(attributes)
    ]


def _required_context(context: SpanContext | None) -> SpanContext:
    if context is None or not context.is_valid:
        raise TypeError("OpenTelemetry span context is missing or invalid")
    return context


def _trace_id(value: int) -> str:
    return f"{value:032x}"


def _span_id(value: int) -> str:
    return f"{value:016x}"


class AssaySpanExporter(SpanExporter):
    """Export completed spans to Assay using OTLP's JSON representation."""

    def __init__(
        self,
        endpoint: str,
        api_key: str,
        *,
        client: httpx.Client | None = None,
        max_request_bytes: int = 64 << 20,
    ) -> None:
        if not api_key.strip():
            raise AssayConfigurationError("API key must not be blank")
        if max_request_bytes <= 0:
            raise AssayConfigurationError("max request bytes must be positive")
        self._url = _export_url(endpoint)
        self._api_key = api_key
        self._max_request_bytes = max_request_bytes
        self._owns_client = client is None
        self._client = client or httpx.Client(
            timeout=httpx.Timeout(connect=5.0, read=10.0, write=10.0, pool=5.0)
        )
        self._lock = threading.Lock()
        self._closed = False

    @override
    def export(self, spans: Sequence[ReadableSpan]) -> SpanExportResult:
        """Export one batch without raising into instrumented application code."""
        if not spans:
            return SpanExportResult.SUCCESS
        with self._lock:
            if self._closed:
                return SpanExportResult.FAILURE
            try:
                payload = json.dumps(
                    build_export_request(spans),
                    allow_nan=False,
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode("utf-8")
                if len(payload) > self._max_request_bytes:
                    return SpanExportResult.FAILURE
                response = self._client.post(
                    self._url,
                    content=payload,
                    headers={
                        "Content-Type": "application/json",
                        "User-Agent": _user_agent(),
                        "x-api-key": self._api_key,
                    },
                )
                return _response_result(response)
            except Exception:
                return SpanExportResult.FAILURE

    @override
    def shutdown(self) -> None:
        """Stop exports and close an internally owned HTTP client."""
        with self._lock:
            if self._closed:
                return
            self._closed = True
            if self._owns_client:
                self._client.close()

    @override
    def force_flush(self, timeout_millis: int = 30_000) -> bool:
        """Report whether the exporter remains available; it has no local buffer."""
        del timeout_millis
        with self._lock:
            return not self._closed


def _response_result(response: httpx.Response) -> SpanExportResult:
    if response.status_code < 200 or response.status_code >= 300:
        return SpanExportResult.FAILURE
    body = response.json()
    if not isinstance(body, dict):
        return SpanExportResult.FAILURE
    partial = body.get("partialSuccess")
    if partial is None:
        return SpanExportResult.SUCCESS
    if not isinstance(partial, dict):
        return SpanExportResult.FAILURE
    rejected = partial.get("rejectedSpans", "0")
    if isinstance(rejected, bool) or not isinstance(rejected, str | int):
        return SpanExportResult.FAILURE
    try:
        rejected_count = int(rejected)
    except ValueError:
        return SpanExportResult.FAILURE
    return SpanExportResult.FAILURE if rejected_count > 0 else SpanExportResult.SUCCESS


def _export_url(endpoint: str) -> str:
    parsed = urlsplit(endpoint.strip())
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise AssayConfigurationError("endpoint must be an HTTP or HTTPS base URL")
    path = parsed.path.rstrip("/")
    if path not in {"", "/v1/traces"} or parsed.query or parsed.fragment:
        raise AssayConfigurationError("endpoint must be a base URL or end with /v1/traces")
    return urlunsplit((parsed.scheme, parsed.netloc, "/v1/traces", "", ""))


def _user_agent() -> str:
    try:
        version = metadata.version("assay-sdk")
    except metadata.PackageNotFoundError:
        version = "unknown"
    return f"assay-sdk/{version}"
