import json
from pathlib import Path

import httpx
import pytest

from assay.client import Client, RunsResource
from assay.exceptions import (
    AssayConfigurationError,
    AssayImportError,
    AssayTimeoutError,
)

STAMP = "2026-08-31T12:00:00Z"
DATASET = {
    "id": "dataset-1",
    "application_id": "application-1",
    "name": "regression",
    "description": None,
    "created_at": STAMP,
    "updated_at": STAMP,
}
RUN = {
    "id": "run-1",
    "application_id": "application-1",
    "dataset_id": "dataset-1",
    "name": "baseline",
    "status": "succeeded",
    "mode": "score_existing",
    "params": {},
    "scorers": ["groundedness"],
    "aggregates": {},
    "total_items": 1,
    "succeeded_items": 1,
    "failed_items": 0,
    "canceled_items": 0,
    "started_at": STAMP,
    "finished_at": STAMP,
    "error": None,
    "created_at": STAMP,
    "updated_at": STAMP,
}
ITEM = {
    "id": "item-1",
    "dataset_id": "dataset-1",
    "external_id": None,
    "input": {},
    "output": None,
    "expected_output": None,
    "context": [],
    "metadata": {},
    "created_at": STAMP,
    "updated_at": STAMP,
}


def test_dataset_ensure_finds_creates_and_resolves_one_conflict() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if calls == 1:
            return httpx.Response(200, json={"items": [], "next_cursor": None})
        if calls == 2:
            return httpx.Response(409, json={"title": "Conflict"})
        return httpx.Response(200, json={"items": [DATASET], "next_cursor": None})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    dataset = client.datasets.ensure("application-1", "regression")

    assert dataset.id == "dataset-1"
    assert calls == 3
    http_client.close()


def test_dataset_ensure_rejects_ambiguous_exact_name() -> None:
    http_client = httpx.Client(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(200, json={"items": [DATASET, DATASET]})
        )
    )
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayConfigurationError, match="ambiguous dataset name"):
        client.datasets.ensure("application-1", "regression")
    http_client.close()


def test_import_file_reports_second_batch_failure(tmp_path: Path) -> None:
    path = tmp_path / "items.jsonl"
    path.write_text("\n".join(json.dumps({"input": {"n": n}}) for n in range(1001)))
    calls = 0

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if calls == 1:
            return httpx.Response(201, json={"items": [ITEM] * 1000})
        return httpx.Response(500)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayImportError) as captured:
        client.datasets.import_file("dataset-1", path)

    assert captured.value.committed_items == 1000
    assert captured.value.batch_number == 2
    assert calls == 2
    http_client.close()


def test_import_file_validates_empty_file_and_batch_size_before_io(tmp_path: Path) -> None:
    path = tmp_path / "empty.jsonl"
    path.write_text("")
    called = False

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(500)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    with pytest.raises(AssayConfigurationError):
        client.datasets.import_file("dataset-1", path)
    with pytest.raises(AssayConfigurationError):
        client.datasets.import_file("dataset-1", path, batch_size=1001)
    assert not called
    http_client.close()


def test_import_file_returns_actual_counts_for_custom_batches(tmp_path: Path) -> None:
    path = tmp_path / "items.jsonl"
    path.write_text("\n".join(json.dumps({"input": {"n": n}}) for n in range(1001)))
    batch_lengths: list[int] = []

    def handler(request: httpx.Request) -> httpx.Response:
        batch_length = len(json.loads(request.read())["items"])
        batch_lengths.append(batch_length)
        return httpx.Response(201, json={"items": [ITEM] * batch_length})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)

    result = client.datasets.import_file("dataset-1", path, batch_size=600)

    assert result.created_items == 1001
    assert result.batches == 2
    assert batch_lengths == [600, 401]
    http_client.close()


def test_run_wait_uses_injected_monotonic_clock() -> None:
    statuses = ["pending", "running", "succeeded"]
    sleeps: list[float] = []

    def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={**RUN, "status": statuses.pop(0)})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)
    client.runs = RunsResource(
        client._transport,
        sleep=sleeps.append,
        monotonic=lambda: 0.0,
    )

    run = client.runs.wait("run-1", timeout=10.0, poll_interval=0.5)

    assert run.status == "succeeded"
    assert sleeps == [0.5, 0.5]
    http_client.close()


def test_run_wait_reports_each_successful_read() -> None:
    statuses = ["pending", "running", "succeeded"]
    observed: list[str] = []

    def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={**RUN, "status": statuses.pop(0)})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)
    client.runs = RunsResource(client._transport, sleep=lambda _: None, monotonic=lambda: 0.0)

    run = client.runs.wait("run-1", on_update=lambda update: observed.append(update.status))

    assert run.status == "succeeded"
    assert observed == ["pending", "running", "succeeded"]
    http_client.close()


def test_run_wait_times_out_before_sleep() -> None:
    clock = iter((0.0, 2.0))
    slept = False

    def sleep(_: float) -> None:
        nonlocal slept
        slept = True

    http_client = httpx.Client(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(200, json={**RUN, "status": "running"})
        )
    )
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)
    client.runs = RunsResource(client._transport, sleep=sleep, monotonic=lambda: next(clock))

    with pytest.raises(AssayTimeoutError, match="wait for run run-1: timed out"):
        client.runs.wait("run-1", timeout=1.0)
    assert not slept
    http_client.close()


def test_run_wait_retries_transient_reads() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if calls < 3:
            raise httpx.ConnectError("sensitive", request=request)
        return httpx.Response(200, json=RUN)

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", admin_token="admin", _http_client=http_client)
    client.runs = RunsResource(client._transport, sleep=lambda _: None, monotonic=lambda: 0.0)

    assert client.runs.wait("run-1").status == "succeeded"
    assert calls == 3
    http_client.close()
