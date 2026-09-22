import json
from pathlib import Path

import httpx
import pytest

from assay.cli import main

STAMP = "2026-09-01T12:00:00Z"
PROJECT = {
    "id": "project-1",
    "name": "support",
    "judge_config": None,
    "created_at": STAMP,
    "updated_at": STAMP,
}
CREATED_KEY = {
    "id": "key-1",
    "project_id": "project-1",
    "name": "ci",
    "key_prefix": "asy_test",
    "last_used_at": None,
    "revoked_at": None,
    "created_at": STAMP,
    "updated_at": STAMP,
    "key": "asy_test_secret",
}
APPLICATION = {
    "id": "application-1",
    "project_id": "project-1",
    "name": "Support Bot",
    "slug": "support-bot",
    "config": {},
    "auto_score_scorers": [],
    "target_endpoint": None,
    "created_at": STAMP,
    "updated_at": STAMP,
}
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
    "external_id": "case-1",
    "input": {"question": "What is Assay?"},
    "output": "An evaluation platform.",
    "expected_output": None,
    "context": [],
    "metadata": {},
    "created_at": STAMP,
    "updated_at": STAMP,
}
SCORER_CONFIG = {
    "id": "scorer-config-1",
    "application_id": "application-1",
    "scorer": "groundedness",
    "enabled": True,
    "threshold": 0.8,
    "judge_config": None,
    "prompt_template_id": "groundedness.v1",
    "persisted": True,
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
    "aggregates": {},
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
TRACE = {
    "id": "trace-1",
    "application_id": "application-1",
    "otel_trace_id": "00112233445566778899aabbccddeeff",
    "root_name": "answer",
    "start_time": STAMP,
    "end_time": STAMP,
    "status": "ok",
    "span_count": 1,
    "total_tokens": 10,
    "total_cost": None,
    "reference_answer": None,
    "attributes": {},
    "score_summaries": [],
    "spans": [],
    "scores": [],
    "scoring_tasks": [],
    "created_at": STAMP,
    "updated_at": STAMP,
}
SCORING_TASK = {
    "id": "task-1",
    "trace_id": "trace-1",
    "scorer": "correctness",
    "status": "pending",
    "error": None,
}
SCORE = {
    "id": 1,
    "scorer": "groundedness",
    "scorer_config_id": None,
    "value": 0.0,
    "threshold": 0.8,
    "passed": False,
    "rationale": "Unsupported",
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
    "snapshot": DATASET_ITEM,
    "snapshot_origin": "creation",
    "scores": [SCORE],
}


def test_projects_create_uses_admin_credentials_and_prints_json(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(201, json=PROJECT)

    monkeypatch.setenv("ASSAY_ENDPOINT", "https://assay.test")
    monkeypatch.setenv("ASSAY_ADMIN_TOKEN", "admin-secret")
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    exit_code = main(["projects", "create", "support"], _http_client=http_client)

    assert exit_code == 0
    assert json.loads(capsys.readouterr().out) == {
        "created_at": STAMP.replace("Z", "+00:00"),
        "id": "project-1",
        "judge_config": None,
        "name": "support",
        "updated_at": STAMP.replace("Z", "+00:00"),
    }
    assert requests[0].headers["authorization"] == "Bearer admin-secret"
    assert json.loads(requests[0].read()) == {"name": "support"}
    http_client.close()


@pytest.mark.parametrize(
    ("argv", "method", "path", "response", "expected_body"),
    [
        (["projects", "list"], "GET", "/v1/projects", {"items": [PROJECT]}, None),
        (
            ["keys", "create", "--project", "project-1", "--name", "ci"],
            "POST",
            "/v1/projects/project-1/keys",
            CREATED_KEY,
            {"name": "ci"},
        ),
        (
            [
                "apps",
                "create",
                "--project",
                "project-1",
                "--name",
                "Support Bot",
                "--slug",
                "support-bot",
            ],
            "POST",
            "/v1/applications",
            APPLICATION,
            {"project_id": "project-1", "name": "Support Bot", "slug": "support-bot"},
        ),
        (
            ["apps", "list", "--project", "project-1"],
            "GET",
            "/v1/applications",
            {"items": [APPLICATION]},
            None,
        ),
    ],
)
def test_management_commands_route_through_the_real_client(
    argv: list[str],
    method: str,
    path: str,
    response: object,
    expected_body: object | None,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=response)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(argv, _http_client=http_client) == 0

    output = json.loads(capsys.readouterr().out)
    assert requests[0].method == method
    assert requests[0].url.path == path
    assert requests[0].headers["authorization"] == "Bearer admin-secret"
    if expected_body is not None:
        assert json.loads(requests[0].read()) == expected_body
    if argv[:2] == ["keys", "create"]:
        assert output["key"] == "asy_test_secret"
    http_client.close()


def test_apps_set_endpoint_reads_and_sends_the_configuration_file(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    endpoint_path = tmp_path / "endpoint.json"
    endpoint_path.write_text(
        json.dumps(
            {
                "url": "https://target.test/answer",
                "method": "POST",
                "headers": {"X-Tenant": "support"},
                "request_template": {"question": "$.input.question"},
                "response_mapping": {"output": "$.answer", "context": "$.sources"},
                "timeout_ms": 5000,
                "secret": "bearer-secret",
            }
        )
    )
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=APPLICATION)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    exit_code = main(
        ["apps", "set-endpoint", "application-1", "--file", str(endpoint_path)],
        _http_client=http_client,
    )

    assert exit_code == 0
    assert json.loads(requests[0].read()) == {
        "endpoint": {
            "headers": {"X-Tenant": "support"},
            "method": "POST",
            "request_template": {"question": "$.input.question"},
            "response_mapping": {"context": "$.sources", "output": "$.answer"},
            "secret": "bearer-secret",
            "timeout_ms": 5000,
            "url": "https://target.test/answer",
        }
    }
    capsys.readouterr()
    http_client.close()


def test_datasets_import_ensures_a_dataset_and_imports_the_file(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    dataset_path = tmp_path / "regression.jsonl"
    dataset_path.write_text(
        json.dumps(
            {
                "external_id": "case-1",
                "input": {"question": "What is Assay?"},
                "output": "An evaluation platform.",
            }
        )
    )
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "GET":
            return httpx.Response(200, json={"items": [], "next_cursor": None})
        if request.url.path == "/v1/datasets":
            return httpx.Response(201, json=DATASET)
        return httpx.Response(201, json={"items": [DATASET_ITEM]})

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    exit_code = main(
        ["datasets", "import", "application-1", "--file", str(dataset_path)],
        _http_client=http_client,
    )

    assert exit_code == 0
    assert [request.url.path for request in requests] == [
        "/v1/datasets",
        "/v1/datasets",
        "/v1/datasets/dataset-1/items",
    ]
    assert json.loads(capsys.readouterr().out) == {
        "batches": 1,
        "created_items": 1,
        "dataset_id": "dataset-1",
    }
    http_client.close()


@pytest.mark.parametrize(
    ("argv", "path", "response", "expected_body"),
    [
        (
            ["scorers", "set", "application-1", "groundedness", "--threshold", "0.8"],
            "/v1/applications/application-1/scorers/groundedness",
            SCORER_CONFIG,
            {"threshold": 0.8},
        ),
        (
            [
                "run",
                "create",
                "application-1",
                "--dataset",
                "dataset-1",
                "--name",
                "baseline",
                "--scorers",
                "groundedness",
                "--mode",
                "score_existing",
            ],
            "/v1/runs",
            RUN,
            {
                "application_id": "application-1",
                "dataset_id": "dataset-1",
                "mode": "score_existing",
                "name": "baseline",
                "scorers": ["groundedness"],
            },
        ),
    ],
)
def test_evaluation_commands_send_validated_requests(
    argv: list[str],
    path: str,
    response: object,
    expected_body: object,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=response)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(argv, _http_client=http_client) == 0

    assert requests[0].url.path == path
    assert json.loads(requests[0].read()) == expected_body
    capsys.readouterr()
    http_client.close()


def test_run_watch_streams_updates_and_fails_an_unmet_gate(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    statuses = [
        {**RUN, "status": "running"},
        {
            **RUN,
            "status": "succeeded",
            "succeeded_items": 1,
            "aggregates": {"groundedness": {"mean": 0.75, "pass_rate": 0.7, "n": 1}},
        },
    ]

    def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json=statuses.pop(0))

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    exit_code = main(
        [
            "run",
            "watch",
            "run-1",
            "--poll-interval",
            "0.001",
            "--gate",
            "groundedness:0.8",
        ],
        _http_client=http_client,
    )

    assert exit_code == 1
    output = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
    assert [entry["status"] for entry in output[:2]] == ["running", "succeeded"]
    assert output[-1] == {
        "failures": [{"actual": 0.7, "required": 0.8, "scorer": "groundedness"}],
        "gate": "failed",
    }
    http_client.close()


@pytest.mark.parametrize(
    ("argv", "method", "path", "response", "expected_body"),
    [
        (
            ["traces", "list", "application-1", "--status", "ok"],
            "GET",
            "/v1/traces",
            {"items": [TRACE], "next_cursor": None},
            None,
        ),
        (["traces", "get", "trace-1"], "GET", "/v1/traces/trace-1", TRACE, None),
        (
            ["traces", "score", "--scorer", "correctness", "trace-1"],
            "POST",
            "/v1/traces/score",
            {"items": [SCORING_TASK]},
            {"scorers": ["correctness"], "trace_ids": ["trace-1"]},
        ),
    ],
)
def test_trace_commands_use_project_credentials(
    argv: list[str],
    method: str,
    path: str,
    response: object,
    expected_body: object | None,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=response)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(argv, _http_client=http_client) == 0

    assert requests[0].method == method
    assert requests[0].url.path == path
    assert requests[0].headers["x-api-key"] == "project-secret"
    if expected_body is not None:
        assert json.loads(requests[0].read()) == expected_body
    capsys.readouterr()
    http_client.close()


@pytest.mark.parametrize(
    ("argv", "method", "path", "response"),
    [
        (["projects", "get", "project-1"], "GET", "/v1/projects/project-1", PROJECT),
        (
            ["keys", "list", "project-1"],
            "GET",
            "/v1/projects/project-1/keys",
            {"items": []},
        ),
        (["apps", "get", "application-1"], "GET", "/v1/applications/application-1", APPLICATION),
        (
            ["datasets", "list", "application-1"],
            "GET",
            "/v1/datasets",
            {"items": [DATASET], "next_cursor": None},
        ),
        (
            ["datasets", "items", "list", "dataset-1"],
            "GET",
            "/v1/datasets/dataset-1/items",
            {"items": [DATASET_ITEM], "next_cursor": None},
        ),
        (
            ["scorers", "list", "application-1"],
            "GET",
            "/v1/applications/application-1/scorers",
            {"items": [SCORER_CONFIG]},
        ),
        (["run", "list", "application-1"], "GET", "/v1/runs", {"items": [RUN]}),
        (["run", "get", "run-1"], "GET", "/v1/runs/run-1", RUN),
        (
            ["run", "items", "run-1"],
            "GET",
            "/v1/runs/run-1/items",
            {"items": [RUN_ITEM], "next_cursor": None},
        ),
        (
            ["run", "scores", "run-1"],
            "GET",
            "/v1/runs/run-1/scores",
            {"items": [SCORE], "next_cursor": None},
        ),
        (["run", "item", "run-1", "item-1"], "GET", "/v1/runs/run-1/items/item-1", RUN_ITEM),
        (["run", "cancel", "run-1"], "POST", "/v1/runs/run-1/cancel", RUN),
        (
            ["datasets", "update", "dataset-1", "--clear-description"],
            "PATCH",
            "/v1/datasets/dataset-1",
            DATASET,
        ),
        (
            [
                "run",
                "compare",
                "run-1",
                "run-2",
                "--scorer",
                "groundedness",
            ],
            "GET",
            "/v1/runs/run-1/comparison",
            {
                "baseline_run_id": "run-1",
                "candidate_run_id": "run-2",
                "scorer": "groundedness",
                "items": [],
                "summary": {
                    "n": 0,
                    "mean_delta": None,
                    "matched": 0,
                    "changed_cases": 0,
                    "baseline_only": 0,
                    "candidate_only": 0,
                    "unscored": 0,
                },
                "warnings": [],
            },
        ),
        (
            ["traces", "eligibility", "trace-1"],
            "GET",
            "/v1/traces/trace-1/scoring-eligibility",
            {"items": [{"scorer": "groundedness", "eligible": True, "reasons": []}]},
        ),
    ],
)
def test_product_commands_delegate_to_typed_resources(
    argv: list[str],
    method: str,
    path: str,
    response: object,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=response)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(argv, _http_client=http_client) == 0

    assert (requests[0].method, requests[0].url.path) == (method, path)
    assert requests[0].headers["authorization"] == "Bearer admin-secret"
    output = json.loads(capsys.readouterr().out)
    if argv[:2] == ["run", "scores"]:
        assert output["items"][0]["value"] == 0.0
    http_client.close()


def test_dataset_item_replace_loads_complete_json_file(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    item_path = tmp_path / "item.json"
    item_path.write_text(json.dumps({"input": {"question": "What is Assay?"}}))
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=DATASET_ITEM)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert (
        main(
            [
                "datasets",
                "items",
                "replace",
                "dataset-1",
                "item-1",
                "--file",
                str(item_path),
            ],
            _http_client=http_client,
        )
        == 0
    )

    assert json.loads(requests[0].read()) == {
        "context": [],
        "expected_output": None,
        "external_id": None,
        "input": {"question": "What is Assay?"},
        "metadata": {},
        "output": None,
    }
    capsys.readouterr()
    http_client.close()


def test_trace_list_exposes_search_and_score_filters(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json={"items": [], "next_cursor": None})

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert (
        main(
            [
                "traces",
                "list",
                "application-1",
                "--query",
                "answer",
                "--scorer",
                "groundedness",
                "--failed",
            ],
            _http_client=http_client,
        )
        == 0
    )

    assert requests[0].url.params["q"] == "answer"
    assert requests[0].url.params["scorer"] == "groundedness"
    assert requests[0].url.params["passed"] == "false"
    capsys.readouterr()
    http_client.close()


@pytest.mark.parametrize(
    "argv",
    [
        ["projects", "delete", "project-1"],
        ["keys", "revoke", "project-1", "key-1"],
        ["apps", "delete", "application-1"],
        ["apps", "clear-endpoint", "application-1"],
        ["datasets", "delete", "dataset-1"],
        ["datasets", "items", "delete", "dataset-1", "item-1"],
        ["run", "delete", "run-1"],
        ["traces", "delete", "trace-1"],
    ],
)
def test_destructive_commands_require_explicit_confirmation(
    argv: list[str],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(500)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    with pytest.raises(SystemExit) as captured:
        main(argv, _http_client=http_client)

    assert captured.value.code == 2
    assert not requests
    http_client.close()


@pytest.mark.parametrize(
    ("argv", "path"),
    [
        (["projects", "delete", "project-1", "--yes"], "/v1/projects/project-1"),
        (
            ["keys", "revoke", "project-1", "key-1", "--yes"],
            "/v1/projects/project-1/keys/key-1",
        ),
        (["apps", "delete", "application-1", "--yes"], "/v1/applications/application-1"),
        (["datasets", "delete", "dataset-1", "--yes"], "/v1/datasets/dataset-1"),
        (
            ["datasets", "items", "delete", "dataset-1", "item-1", "--yes"],
            "/v1/datasets/dataset-1/items/item-1",
        ),
        (["run", "delete", "run-1", "--yes"], "/v1/runs/run-1"),
        (["traces", "delete", "trace-1", "--yes"], "/v1/traces/trace-1"),
    ],
)
def test_confirmed_destructive_commands_send_one_request(
    argv: list[str],
    path: str,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(204)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(argv, _http_client=http_client) == 0

    assert len(requests) == 1
    assert (requests[0].method, requests[0].url.path) == ("DELETE", path)
    assert json.loads(capsys.readouterr().out)
    http_client.close()


def test_file_backed_product_commands_send_validated_bodies(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    judge = tmp_path / "judge.json"
    judge.write_text(
        json.dumps({"base_url": "https://judge.test/v1", "model": "judge", "api_key": "secret"})
    )
    application = tmp_path / "application.json"
    application.write_text(json.dumps({"name": "Renamed", "auto_score_scorers": ["groundedness"]}))
    reference = tmp_path / "reference.txt"
    reference.write_text("expected\n")
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.url.path.endswith("/scorers/groundedness"):
            return httpx.Response(200, json=SCORER_CONFIG)
        if request.url.path.startswith("/v1/traces"):
            return httpx.Response(200, json=TRACE)
        if request.url.path.startswith("/v1/applications"):
            return httpx.Response(200, json=APPLICATION)
        return httpx.Response(200, json=PROJECT)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    commands = [
        ["projects", "update", "project-1", "--judge-config-file", str(judge)],
        ["apps", "update", "application-1", "--file", str(application)],
        [
            "scorers",
            "set",
            "application-1",
            "groundedness",
            "--disabled",
            "--judge-config-file",
            str(judge),
            "--prompt-template-id",
            "groundedness.v1",
        ],
        ["apps", "clear-endpoint", "application-1", "--yes"],
        ["traces", "reference", "trace-1", "--file", str(reference)],
    ]

    assert all(main(command, _http_client=http_client) == 0 for command in commands)

    assert json.loads(requests[0].read())["judge_config"]["api_key"] == "secret"
    assert json.loads(requests[1].read()) == {
        "auto_score_scorers": ["groundedness"],
        "name": "Renamed",
    }
    assert json.loads(requests[2].read())["enabled"] is False
    assert json.loads(requests[3].read()) == {"clear": True}
    assert json.loads(requests[4].read()) == {"reference_answer": "expected"}
    capsys.readouterr()
    http_client.close()


def test_dataset_export_follows_pages_as_jsonl(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    cursors: list[str | None] = []

    def handler(request: httpx.Request) -> httpx.Response:
        cursor = request.url.params.get("cursor")
        cursors.append(cursor)
        return httpx.Response(
            200,
            json={
                "items": [DATASET_ITEM],
                "next_cursor": "next" if cursor is None else None,
            },
        )

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(["datasets", "export", "dataset-1"], _http_client=http_client) == 0

    output = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
    assert [item["id"] for item in output] == ["item-1", "item-1"]
    assert cursors == [None, "next"]
    http_client.close()


def test_trace_cli_falls_back_to_admin_without_project_key(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json={"items": [], "next_cursor": None})

    _set_cli_environment(monkeypatch)
    monkeypatch.delenv("ASSAY_API_KEY")
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(["traces", "list", "application-1"], _http_client=http_client) == 0

    assert requests[0].headers["authorization"] == "Bearer admin-secret"
    assert "x-api-key" not in requests[0].headers
    capsys.readouterr()
    http_client.close()


def test_secret_file_validation_error_is_content_free(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    path = tmp_path / "judge.json"
    path.write_text(json.dumps({"base_url": {"secret": "do-not-print"}, "model": "judge"}))
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(500)

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    exit_code = main(
        ["projects", "update", "project-1", "--judge-config-file", str(path)],
        _http_client=http_client,
    )

    assert exit_code == 2
    assert not requests
    assert "do-not-print" not in capsys.readouterr().err
    http_client.close()


def test_cancel_conflict_returns_nonzero_without_retry(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(409, json={"title": "Conflict"})

    _set_cli_environment(monkeypatch)
    http_client = httpx.Client(transport=httpx.MockTransport(handler))

    assert main(["run", "cancel", "run-1"], _http_client=http_client) == 2

    assert len(requests) == 1
    assert "409" in capsys.readouterr().err
    http_client.close()


def test_missing_endpoint_returns_an_actionable_error(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    monkeypatch.delenv("ASSAY_ENDPOINT", raising=False)

    assert main(["projects", "list"]) == 2

    assert capsys.readouterr().err == "assay: error: endpoint is required\n"


def test_help_does_not_offer_secrets_as_process_arguments(
    capsys: pytest.CaptureFixture[str],
) -> None:
    with pytest.raises(SystemExit) as captured:
        main(["--help"])

    assert captured.value.code == 0
    output = capsys.readouterr().out
    assert "--api-key" not in output
    assert "--admin-token" not in output


def _set_cli_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("ASSAY_ENDPOINT", "https://assay.test")
    monkeypatch.setenv("ASSAY_ADMIN_TOKEN", "admin-secret")
    monkeypatch.setenv("ASSAY_API_KEY", "project-secret")
