import json
from collections.abc import Callable

import httpx
import pytest

from assay.client import Client
from assay.exceptions import AssayConfigurationError, AssayProtocolError
from assay.models import Chunk, DatasetItemInput

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
    "output": "Answer",
    "expected_output": None,
    "context": [{"id": "chunk-1", "text": "Evidence"}],
    "metadata": {"source": "test"},
    "created_at": STAMP,
    "updated_at": STAMP,
}
SCORER = {
    "id": None,
    "application_id": "application-1",
    "scorer": "groundedness",
    "enabled": True,
    "threshold": 0.8,
    "judge_config": None,
    "prompt_template_id": "groundedness.v1",
    "persisted": False,
}
RUN = {
    "id": "run-1",
    "application_id": "application-1",
    "dataset_id": "dataset-1",
    "name": "baseline",
    "status": "queued",
    "mode": "score_existing",
    "params": {},
    "scorers": ["groundedness"],
    "aggregates": {"groundedness": {"mean": 0.9, "pass_rate": 1.0, "n": 1}},
    "total_items": 1,
    "succeeded_items": 0,
    "failed_items": 0,
    "canceled_items": 0,
    "started_at": None,
    "finished_at": None,
    "error": None,
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
    "generated_output": None,
    "generated_context": [],
    "generated_at": None,
}
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
    "eval_run_id": "run-1",
    "dataset_item_id": "item-1",
    "trace_id": None,
    "span_id": None,
    "span_start_time": None,
    "judged_input": None,
    "judged_output": None,
    "judged_context": [],
    "judged_reference": None,
    "created_at": STAMP,
}


def test_evaluation_resource_contracts() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        response = _evaluation_response(request.method, request.url.path)
        if response is None:
            return httpx.Response(204)
        return httpx.Response(200, json=response)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)
    item = DatasetItemInput(
        input={"question": "What?"},
        output="Answer",
        context=(Chunk(id="chunk-1", text="Evidence", score=0.9, source="doc"),),
        metadata={"source": "test"},
    )

    assert client.datasets.create("application-1", "regression").id == "dataset-1"
    assert client.datasets.list(limit=10).next_cursor == "dataset-next"
    assert client.datasets.get("dataset-1").name == "regression"
    client.datasets.delete("dataset-1")
    assert client.datasets.create_items("dataset-1", (item,))[0].context[0].id == "chunk-1"
    assert client.datasets.list_items("dataset-1").next_cursor == "item-next"
    assert client.scorers.list("application-1")[0].threshold == 0.8
    assert client.scorers.set("application-1", "groundedness", threshold=0.8).enabled
    assert (
        client.runs.create("application-1", "dataset-1", "baseline", scorers=("groundedness",)).id
        == "run-1"
    )
    assert client.runs.list(status="queued").next_cursor == "run-next"
    assert client.runs.get("run-1").aggregates["groundedness"].n == 1
    assert client.runs.list_items("run-1").items[0].status == "succeeded"
    assert client.runs.list_scores("run-1").items[0].passed
    assert client.runs.cancel("run-1").status == "queued"

    create_items = _request(requests, "POST", "/v1/datasets/dataset-1/items")
    serialized = json.loads(create_items.read())["items"][0]["context"][0]
    assert serialized == {"id": "chunk-1", "text": "Evidence"}
    _assert_admin_auth(requests)
    http_client.close()


@pytest.mark.parametrize(
    "call",
    [
        lambda client: client.datasets.list(limit=501),
        lambda client: client.datasets.create_items("dataset", ()),
        lambda client: client.datasets.create_items(
            "dataset", tuple(DatasetItemInput(input={}) for _ in range(1001))
        ),
        lambda client: client.scorers.set("application", "unknown"),
        lambda client: client.scorers.set("application", "groundedness", threshold=1.1),
        lambda client: client.runs.create(
            "application", "dataset", "run", scorers=("groundedness", "groundedness")
        ),
        lambda client: client.runs.create(
            "application", "dataset", "run", mode="unknown", scorers=("groundedness",)
        ),
    ],
)
def test_evaluation_validation_happens_before_io(call: Callable[[Client], object]) -> None:
    called = False

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(500)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayConfigurationError):
        call(client)

    assert not called
    http_client.close()


def test_all_page_helper_rejects_repeated_cursor() -> None:
    http_client = httpx.Client(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(
                200,
                json={"items": [DATASET], "next_cursor": "same"},
            )
        )
    )
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayProtocolError, match="repeated a cursor"):
        tuple(client.datasets.iter_all_datasets())

    http_client.close()


def _evaluation_response(method: str, path: str) -> object | None:
    responses: dict[tuple[str, str], object | None] = {
        ("POST", "/v1/datasets"): DATASET,
        ("GET", "/v1/datasets"): {"items": [DATASET], "next_cursor": "dataset-next"},
        ("GET", "/v1/datasets/dataset-1"): DATASET,
        ("DELETE", "/v1/datasets/dataset-1"): None,
        ("POST", "/v1/datasets/dataset-1/items"): {
            "items": [DATASET_ITEM],
            "next_cursor": "item-next",
        },
        ("GET", "/v1/datasets/dataset-1/items"): {
            "items": [DATASET_ITEM],
            "next_cursor": "item-next",
        },
        ("GET", "/v1/applications/application-1/scorers"): {"items": [SCORER]},
        ("PUT", "/v1/applications/application-1/scorers/groundedness"): SCORER,
        ("POST", "/v1/runs"): RUN,
        ("GET", "/v1/runs"): {"items": [RUN], "next_cursor": "run-next"},
        ("GET", "/v1/runs/run-1"): RUN,
        ("GET", "/v1/runs/run-1/items"): {"items": [RUN_ITEM], "next_cursor": None},
        ("GET", "/v1/runs/run-1/scores"): {"items": [SCORE], "next_cursor": "score-next"},
        ("POST", "/v1/runs/run-1/cancel"): RUN,
    }
    return responses[(method, path)]


def _request(requests: list[httpx.Request], method: str, path: str) -> httpx.Request:
    for request in requests:
        if request.method == method and request.url.path == path:
            return request
    raise AssertionError(f"missing {method} {path}")


def _assert_admin_auth(requests: list[httpx.Request]) -> None:
    for request in requests:
        assert request.headers["authorization"] == "Bearer admin"
