import json

import httpx
import pytest

from assay.cli import main
from assay.client import Client

STAMP = "2026-09-01T00:00:00Z"
SCORE = {
    "id": 1,
    "scorer": "groundedness",
    "value": 0.2,
    "threshold": 0.5,
    "passed": False,
    "rationale": "unsupported",
    "details": {},
    "prompt_template_id": "v1",
    "judge_model": "fake",
    "judge_provider": "fake",
    "judge_tokens": 0,
    "trace_id": "trace-1",
    "judged_input": "question",
    "judged_output": "answer",
    "judged_context": [{"id": "k0", "text": "evidence"}],
    "created_at": STAMP,
}


def test_score_export_follows_pages_and_preserves_evidence(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    monkeypatch.setenv("ASSAY_ENDPOINT", "https://assay.test")
    monkeypatch.setenv("ASSAY_ADMIN_TOKEN", "admin")

    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.params["passed"] == "false"
        assert request.url.params["application_id"] == "app-1"
        assert request.headers["Authorization"] == "Bearer admin"
        if "cursor" not in request.url.params:
            return httpx.Response(200, json={"items": [SCORE], "next_cursor": "next"})
        return httpx.Response(200, json={"items": [{**SCORE, "id": 2}]})

    with httpx.Client(transport=httpx.MockTransport(handler)) as transport:
        result = main(
            ["scores", "export", "app-1", "--failed", "--format", "jsonl"], _http_client=transport
        )
    assert result == 0
    rows = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
    assert [row["id"] for row in rows] == [1, 2]
    assert rows[0]["judged_output"] == "answer"


def test_metrics_returns_typed_daily_results() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/applications/app-1/metrics"
        return httpx.Response(
            200,
            json={
                "items": [
                    {
                        "date": STAMP,
                        "scorer": "groundedness",
                        "mean": 0.5,
                        "pass_rate": 0.5,
                        "n": 2,
                    }
                ]
            },
        )

    with (
        httpx.Client(transport=httpx.MockTransport(handler)) as transport,
        Client("https://assay.test", admin_token="admin", _http_client=transport) as client,
    ):
        points = client.metrics.list("app-1")
    assert points[0].mean == 0.5
    assert points[0].n == 2


def test_regression_import_delegates_to_the_trace_evidence_endpoint() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(
            201,
            json={
                "id": "item-1",
                "dataset_id": "dataset-1",
                "input": {"question": "question"},
                "output": "answer",
                "expected_output": "corrected",
                "context": [{"id": "k0", "text": "evidence"}],
                "metadata": {"trace_id": "trace-1"},
                "created_at": STAMP,
                "updated_at": STAMP,
            },
        )

    with (
        httpx.Client(transport=httpx.MockTransport(handler)) as transport,
        Client("https://assay.test", admin_token="admin", _http_client=transport) as client,
    ):
        item = client.datasets.from_trace(
            "dataset-1", "trace-1", scorer="groundedness", expected_output="corrected"
        )
    assert item.output == "answer"
    assert len(requests) == 1
    assert requests[0].method == "POST"
    assert requests[0].url.path == "/v1/datasets/dataset-1/from-trace"
    assert json.loads(requests[0].content) == {
        "expected_output": "corrected", "scorer": "groundedness", "trace_id": "trace-1"
    }
