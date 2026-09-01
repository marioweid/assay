from collections.abc import Callable
from datetime import datetime, timezone

import httpx
import pytest

from assay.client import Client
from assay.exceptions import AssayConfigurationError

STAMP = "2026-08-31T12:00:00Z"
EVENT = {
    "time": STAMP,
    "name": "event",
    "attributes": {"attempt": 1},
    "dropped_attributes_count": 0,
}
CHILD_SPAN = {
    "id": 2,
    "otel_span_id": "0000000000000002",
    "parent_span_id": "0000000000000001",
    "name": "child",
    "kind": "internal",
    "operation_name": "generate",
    "start_time": STAMP,
    "end_time": STAMP,
    "duration_ms": 1,
    "status_code": "ok",
    "status_message": "",
    "is_scorable": True,
    "scorable_kind": "generation",
    "attributes": {},
    "events": [EVENT],
    "input_tokens": 2,
    "output_tokens": 3,
    "reference_answer": None,
    "children": [],
}
ROOT_SPAN = {**CHILD_SPAN, "id": 1, "parent_span_id": None, "children": [CHILD_SPAN]}
SCORE = {
    "id": 1,
    "scorer": "groundedness",
    "scorer_config_id": None,
    "value": 0.9,
    "threshold": 0.8,
    "passed": True,
    "rationale": "Supported",
    "details": {},
    "prompt_template_id": "groundedness.v1",
    "judge_model": "judge",
    "judge_provider": "openai-compatible",
    "judge_tokens": 10,
    "eval_run_id": None,
    "dataset_item_id": None,
    "trace_id": "trace-1",
    "span_id": 2,
    "span_start_time": STAMP,
    "judged_input": "question",
    "judged_output": "answer",
    "judged_context": [{"id": "chunk", "text": "evidence"}],
    "judged_reference": None,
    "created_at": STAMP,
}
TASK = {
    "id": "task-1",
    "trace_id": "trace-1",
    "scorer": "groundedness",
    "status": "queued",
    "error": None,
}
TRACE = {
    "id": "trace-1",
    "application_id": "application-1",
    "otel_trace_id": "0" * 32,
    "root_name": "request",
    "start_time": STAMP,
    "end_time": STAMP,
    "status": "ok",
    "span_count": 2,
    "total_tokens": 5,
    "total_cost": None,
    "reference_answer": "expected",
    "attributes": {"environment": "test"},
    "spans": [ROOT_SPAN],
    "scores": [SCORE],
    "scoring_tasks": [TASK],
    "created_at": STAMP,
    "updated_at": STAMP,
}


def test_trace_resource_contract() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        responses = {
            ("GET", "/v1/traces"): {"items": [TRACE], "next_cursor": "next"},
            ("GET", "/v1/traces/trace-1"): TRACE,
            ("POST", "/v1/traces/score"): {"items": [TASK]},
            ("PATCH", "/v1/traces/trace-1/reference"): TRACE,
        }
        return httpx.Response(200, json=responses[(request.method, request.url.path)])

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", api_key="asy_secret", _http_client=http_client)
    start = datetime(2026, 8, 31, 12, tzinfo=timezone.utc)

    page = client.traces.list(
        application_id="application-1",
        start=start,
        end=start,
        status="ok",
        limit=200,
        cursor="cursor",
    )
    trace = client.traces.get("trace-1")
    tasks = client.traces.score(("trace-1",), ("groundedness",))
    referenced = client.traces.set_reference("trace-1", "expected")

    assert page.next_cursor == "next"
    assert trace.spans[0].children[0].events[0].attributes["attempt"] == 1
    assert trace.scores[0].judged_context[0].text == "evidence"
    assert tasks[0].id == "task-1"
    assert referenced.reference_answer == "expected"
    params = requests[0].url.params
    assert params["start"] == "2026-08-31T12:00:00+00:00"
    assert params["end"] == "2026-08-31T12:00:00+00:00"
    for request in requests:
        assert request.headers["x-api-key"] == "asy_secret"
        assert "authorization" not in request.headers
    http_client.close()


@pytest.mark.parametrize(
    "call",
    [
        lambda client: client.traces.list(limit=201),
        lambda client: client.traces.list(start=datetime(2026, 8, 31)),
        lambda client: client.traces.score((), ("groundedness",)),
        lambda client: client.traces.score(("trace", "trace"), ("groundedness",)),
        lambda client: client.traces.score(("trace",), ("unknown",)),
        lambda client: client.traces.set_reference("trace", " "),
    ],
)
def test_trace_validation_happens_before_io(call: Callable[[Client], object]) -> None:
    called = False

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(500)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", api_key="asy_secret", _http_client=http_client)

    with pytest.raises(AssayConfigurationError):
        call(client)

    assert not called
    http_client.close()
