import json
from collections.abc import Callable

import httpx
import pytest

from assay.client import Client
from assay.exceptions import AssayAPIError, AssayConfigurationError, AssayProtocolError
from assay.models import Chunk, DatasetItemInput, JudgeConfig

STAMP = "2026-08-31T12:00:00Z"
DATASET = {
    "id": "dataset-1",
    "application_id": "application-1",
    "name": "regression",
    "description": None,
    "created_at": STAMP,
    "updated_at": STAMP,
}
DATASET_ITEM = {
    "id": "item-1",
    "dataset_id": "dataset-1",
    "external_id": None,
    "input": {"question": "What?"},
    "output": None,
    "expected_output": None,
    "context": [],
    "metadata": {},
    "created_at": STAMP,
    "updated_at": STAMP,
}
RUN_ITEM = {
    "eval_run_id": "run-1",
    "dataset_item_id": "item-1",
    "status": "succeeded",
    "error": None,
    "started_at": STAMP,
    "finished_at": STAMP,
    "created_at": STAMP,
    "updated_at": STAMP,
    "generated_output": "Answer",
    "generated_context": [],
    "generated_at": STAMP,
    "snapshot": DATASET_ITEM,
    "snapshot_origin": "creation",
    "scores": [],
}
SUMMARY = {
    "n": 1,
    "mean_delta": 0.2,
    "matched": 1,
    "changed_cases": 0,
    "baseline_only": 0,
    "candidate_only": 0,
    "unscored": 0,
}
TRACE = {
    "id": "trace-1",
    "application_id": "application-1",
    "otel_trace_id": "0" * 32,
    "root_name": "request",
    "start_time": STAMP,
    "end_time": STAMP,
    "status": "ok",
    "span_count": 1,
    "total_tokens": 5,
    "total_cost": None,
    "reference_answer": None,
    "attributes": {},
    "score_summaries": [
        {
            "scorer": "groundedness",
            "value": 0.9,
            "threshold": 0.8,
            "passed": True,
            "created_at": STAMP,
        }
    ],
    "created_at": STAMP,
    "updated_at": STAMP,
}


def test_dataset_mutations_send_complete_contracts() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "DELETE":
            return httpx.Response(204)
        return httpx.Response(
            200,
            json=DATASET if request.url.path == "/v1/datasets/dataset-1" else DATASET_ITEM,
        )

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)
    item = DatasetItemInput(input={"question": "What?"})

    assert client.datasets.update("dataset-1", clear_description=True).id == "dataset-1"
    assert client.datasets.get_item("dataset-1", "item-1").id == "item-1"
    assert client.datasets.replace_item("dataset-1", "item-1", item=item).id == "item-1"
    client.datasets.delete_item("dataset-1", "item-1")

    assert [(request.method, request.url.path) for request in requests] == [
        ("PATCH", "/v1/datasets/dataset-1"),
        ("GET", "/v1/datasets/dataset-1/items/item-1"),
        ("PUT", "/v1/datasets/dataset-1/items/item-1"),
        ("DELETE", "/v1/datasets/dataset-1/items/item-1"),
    ]
    assert json.loads(requests[0].read()) == {"clear_description": True}
    assert json.loads(requests[2].read()) == {
        "context": [],
        "expected_output": None,
        "external_id": None,
        "input": {"question": "What?"},
        "metadata": {},
        "output": None,
    }
    assert all(request.headers["authorization"] == "Bearer admin" for request in requests)
    http_client.close()


def test_dataset_item_identifiers_are_url_quoted() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=DATASET_ITEM)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    client.datasets.get_item("dataset/one", "item?two")

    assert requests[0].url.raw_path == b"/v1/datasets/dataset%2Fone/items/item%3Ftwo"
    http_client.close()


def test_dataset_item_api_errors_do_not_echo_response_content() -> None:
    http_client = httpx.Client(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(404, json={"captured": "secret payload"})
        )
    )
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayAPIError) as captured:
        client.datasets.get_item("dataset", "item")

    assert "secret payload" not in str(captured.value)
    http_client.close()


def test_run_inspection_comparison_and_delete_contracts() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "DELETE":
            return httpx.Response(204)
        if request.url.path.endswith("/comparison"):
            return httpx.Response(
                200,
                json={
                    "baseline_run_id": "run-1",
                    "candidate_run_id": "run-2",
                    "scorer": "groundedness",
                    "items": [
                        {
                            "dataset_item_id": "item-1",
                            "kind": "matched",
                            "baseline": RUN_ITEM,
                            "candidate": RUN_ITEM,
                            "delta": 0.2,
                        }
                    ],
                    "next_cursor": "next",
                    "summary": SUMMARY,
                    "warnings": [],
                },
            )
        return httpx.Response(200, json=RUN_ITEM)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    assert client.runs.get_item("run-1", "item-1").snapshot.input["question"] == "What?"
    comparison = client.runs.compare(
        "run-1", "run-2", scorer="groundedness", limit=25, cursor="cursor"
    )
    client.runs.delete("run-1")

    assert comparison.summary.mean_delta == 0.2
    assert comparison.items[0].kind == "matched"
    assert requests[1].url.params == httpx.QueryParams(
        {
            "limit": "25",
            "cursor": "cursor",
            "other_run_id": "run-2",
            "scorer": "groundedness",
        }
    )
    assert requests[2].method == "DELETE"
    assert all(request.headers["authorization"] == "Bearer admin" for request in requests)
    http_client.close()


def test_trace_filters_admin_fallback_and_admin_only_actions() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "DELETE":
            return httpx.Response(204)
        if request.url.path.endswith("/scoring-eligibility"):
            return httpx.Response(
                200,
                json={
                    "items": [
                        {
                            "scorer": "groundedness",
                            "eligible": False,
                            "reasons": [{"code": "missing_output", "message": "No output"}],
                        }
                    ]
                },
            )
        return httpx.Response(200, json={"items": [TRACE], "next_cursor": None})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    page = client.traces.list(
        application_id="application-1",
        q="request",
        scorer="groundedness",
        passed=True,
    )
    eligibility = client.traces.eligibility("trace-1")
    client.traces.delete("trace-1")

    assert page.items[0].score_summaries[0].passed
    assert eligibility[0].reasons[0].code == "missing_output"
    assert requests[0].url.params["q"] == "request"
    assert requests[0].url.params["scorer"] == "groundedness"
    assert requests[0].url.params["passed"] == "true"
    assert all(request.headers["authorization"] == "Bearer admin" for request in requests)
    http_client.close()


def test_admin_only_trace_actions_ignore_project_key() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "DELETE":
            return httpx.Response(204)
        return httpx.Response(200, json={"items": []})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client(
        "https://assay.test",
        api_key="asy_secret",
        admin_token="admin",
        _http_client=http_client,
    )

    client.traces.eligibility("trace-1")
    client.traces.delete("trace-1")

    assert all(request.headers["authorization"] == "Bearer admin" for request in requests)
    assert all("x-api-key" not in request.headers for request in requests)
    http_client.close()


def test_project_key_wins_for_trace_reads() -> None:
    request_headers: httpx.Headers | None = None

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal request_headers
        request_headers = request.headers
        return httpx.Response(200, json={"items": [], "next_cursor": None})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client(
        "https://assay.test",
        api_key="asy_secret",
        admin_token="admin",
        _http_client=http_client,
    )

    client.traces.list()

    assert request_headers is not None
    assert request_headers["x-api-key"] == "asy_secret"
    assert "authorization" not in request_headers
    http_client.close()


@pytest.mark.parametrize(
    "call",
    [
        lambda client: client.projects.update(
            "project",
            judge_config=JudgeConfig(base_url="https://judge.test", model="judge"),
            clear_judge_config=True,
        ),
        lambda client: client.datasets.update("dataset", description="set", clear_description=True),
        lambda client: client.datasets.replace_item(
            "dataset", "item", item=DatasetItemInput(input={})
        ),
        lambda client: client.datasets.replace_item(
            "dataset",
            "item",
            item=DatasetItemInput(
                input={"question": "question"}, context=(Chunk(id="", text="text"),)
            ),
        ),
        lambda client: client.runs.compare("run-1", "run-2", scorer="groundedness", limit=501),
        lambda client: client.traces.list(passed=True),
        lambda client: client.traces.list(q="x" * 201),
    ],
)
def test_product_workflow_validation_happens_before_io(
    call: Callable[[Client], object],
) -> None:
    called = False

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(500)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client(
        "https://assay.test",
        api_key="asy_secret",
        admin_token="admin",
        _http_client=http_client,
    )

    with pytest.raises(AssayConfigurationError):
        call(client)

    assert not called
    http_client.close()


@pytest.mark.parametrize(
    ("payload", "field"),
    [
        (
            {
                "baseline_run_id": "run-1",
                "candidate_run_id": "run-2",
                "scorer": "groundedness",
                "items": [
                    {
                        "dataset_item_id": "item-1",
                        "kind": "unscored",
                        "candidate": None,
                        "delta": None,
                    }
                ],
                "summary": SUMMARY,
                "warnings": [],
            },
            "baseline",
        ),
        (
            {
                "baseline_run_id": "run-1",
                "candidate_run_id": "run-2",
                "scorer": "groundedness",
                "items": [],
                "summary": {key: value for key, value in SUMMARY.items() if key != "mean_delta"},
                "warnings": [],
            },
            "mean_delta",
        ),
    ],
)
def test_comparison_requires_nullable_fields(payload: object, field: str) -> None:
    http_client = httpx.Client(
        transport=httpx.MockTransport(lambda _: httpx.Response(200, json=payload))
    )
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayProtocolError, match=f"invalid response field '{field}'"):
        client.runs.compare("run-1", "run-2", scorer="groundedness")

    http_client.close()


def test_trace_list_requires_score_summaries() -> None:
    trace_without_summaries = {
        key: value for key, value in TRACE.items() if key != "score_summaries"
    }
    http_client = httpx.Client(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(
                200, json={"items": [trace_without_summaries], "next_cursor": None}
            )
        )
    )
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayProtocolError, match="invalid response field 'score_summaries'"):
        client.traces.list()

    http_client.close()


def test_malformed_nested_run_item_is_rejected_atomically() -> None:
    malformed = {**RUN_ITEM, "snapshot": {**DATASET_ITEM, "input": "secret"}}
    http_client = httpx.Client(
        transport=httpx.MockTransport(lambda _: httpx.Response(200, json=malformed))
    )
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayProtocolError, match="invalid response field 'input'"):
        client.runs.get_item("run-1", "item-1")

    http_client.close()
