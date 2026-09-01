from collections.abc import Callable
from datetime import datetime, timezone
from typing import cast

import httpx
import pytest

from assay.client import Client
from assay.exceptions import AssayConfigurationError, AssayProtocolError
from assay.models import JudgeConfig, ResponseMapping, TargetEndpoint

PROJECT = {
    "id": "project-1",
    "name": "demo",
    "judge_config": {
        "base_url": "https://judge.test/v1",
        "model": "judge-model",
        "has_api_key": True,
    },
    "created_at": "2026-08-31T12:00:00Z",
    "updated_at": "2026-08-31T12:01:00Z",
}
KEY = {
    "id": "key-1",
    "project_id": "project-1",
    "name": "ci",
    "key_prefix": "asy_1234",
    "last_used_at": None,
    "created_at": "2026-08-31T12:00:00Z",
    "updated_at": "2026-08-31T12:01:00Z",
    "revoked_at": None,
}
TARGET = {
    "url": "https://app.test/answer",
    "method": "POST",
    "headers": {"X-Environment": "test"},
    "request_template": {"question": "{{ input }}"},
    "response_mapping": {"output": "$.answer", "context": "$.context"},
    "timeout_ms": 5000,
    "has_secret": True,
}
APPLICATION = {
    "id": "application-1",
    "project_id": "project-1",
    "name": "production",
    "slug": "production",
    "config": {"nested": [1, 2]},
    "auto_score_scorers": ["groundedness"],
    "target_endpoint": TARGET,
    "created_at": "2026-08-31T12:00:00Z",
    "updated_at": "2026-08-31T12:01:00Z",
}


def _client(handler: httpx.MockTransport) -> tuple[Client, httpx.Client]:
    http_client = httpx.Client(transport=handler)
    return (
        Client("https://assay.test", admin_token="admin-secret", _http_client=http_client),
        http_client,
    )


def test_project_management_contract() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "POST":
            return httpx.Response(201, json=PROJECT)
        if request.method == "GET" and request.url.path == "/v1/projects":
            return httpx.Response(200, json={"items": [PROJECT]})
        if request.method == "GET":
            return httpx.Response(200, json=PROJECT)
        return httpx.Response(204)

    client, http_client = _client(httpx.MockTransport(handler))

    created = client.projects.create("demo")
    projects = client.projects.list()
    fetched = client.projects.get("project-1")
    client.projects.delete("project-1")

    assert created == fetched == projects[0]
    assert created.created_at == datetime(2026, 8, 31, 12, tzinfo=timezone.utc)
    assert created.judge_config is not None
    assert created.judge_config.has_api_key
    assert requests[0].read() == b'{"name":"demo"}'
    assert [request.url.path for request in requests] == [
        "/v1/projects",
        "/v1/projects",
        "/v1/projects/project-1",
        "/v1/projects/project-1",
    ]
    assert all(request.headers["authorization"] == "Bearer admin-secret" for request in requests)
    http_client.close()


def test_key_management_contract_and_secret_repr() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "POST":
            return httpx.Response(201, json={**KEY, "key": "asy_plaintext"})
        if request.method == "GET":
            return httpx.Response(200, json={"items": [KEY]})
        return httpx.Response(204)

    client, http_client = _client(httpx.MockTransport(handler))

    created = client.keys.create("project-1", "ci")
    keys = client.keys.list("project-1")
    client.keys.revoke("project-1", "key-1")

    assert created.key == "asy_plaintext"
    assert "asy_plaintext" not in repr(created)
    assert keys[0].key_prefix == "asy_1234"
    assert requests[0].read() == b'{"name":"ci"}'
    assert requests[2].method == "DELETE"
    assert requests[2].url.path == "/v1/projects/project-1/keys/key-1"
    http_client.close()


def test_application_management_contract_and_immutable_mappings() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "GET" and request.url.path == "/v1/applications":
            return httpx.Response(200, json={"items": [APPLICATION]})
        if request.method == "DELETE":
            return httpx.Response(204)
        return httpx.Response(200, json=APPLICATION)

    client, http_client = _client(httpx.MockTransport(handler))
    target = TargetEndpoint(
        url="https://app.test/answer",
        method="POST",
        headers={"X-Environment": "test"},
        request_template={"question": "{{ input }}"},
        response_mapping=ResponseMapping(output="$.answer", context="$.context"),
        timeout_ms=5000,
        secret="target-secret",
    )

    created = client.applications.create("project-1", "production", "production")
    applications = client.applications.list("project-1")
    fetched = client.applications.get("application-1")
    updated = client.applications.set_endpoint("application-1", target)
    cleared = client.applications.clear_endpoint("application-1")
    client.applications.delete("application-1")

    assert created == fetched == updated == cleared == applications[0]
    assert created.target_endpoint is not None
    assert created.target_endpoint.has_secret
    with pytest.raises(TypeError):
        cast(dict[str, object], created.config)["new"] = "value"
    assert requests[0].read() == (
        b'{"project_id":"project-1","name":"production","slug":"production"}'
    )
    assert b'"secret":"target-secret"' in requests[3].read()
    assert requests[3].method == "PATCH"
    assert requests[4].read() == b'{"clear":true}'
    assert requests[5].method == "DELETE"
    http_client.close()


def test_project_update_serializes_judge_config_and_clear() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=PROJECT)

    client, http_client = _client(httpx.MockTransport(handler))
    judge = JudgeConfig(
        base_url="https://judge.test/v1",
        model="judge-model",
        api_key="judge-secret",
    )

    client.projects.update("project-1", judge_config=judge)
    client.projects.update("project-1", clear_judge_config=True)

    assert b'"api_key":"judge-secret"' in requests[0].read()
    assert requests[1].read() == b'{"clear_judge_config":true}'
    http_client.close()


@pytest.mark.parametrize(
    ("call", "message"),
    [
        (lambda client: client.projects.get(" "), "project ID must not be blank"),
        (lambda client: client.keys.create("project-1", ""), "key name must not be blank"),
        (
            lambda client: client.applications.create("project-1", "name", " "),
            "application slug must not be blank",
        ),
    ],
)
def test_management_rejects_invalid_identifiers_before_io(
    call: Callable[[Client], object],
    message: str,
) -> None:
    called = False

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(500)

    client, http_client = _client(httpx.MockTransport(handler))
    with pytest.raises(AssayConfigurationError, match=message):
        call(client)

    assert not called
    http_client.close()


def test_management_resource_rejects_malformed_response() -> None:
    client, http_client = _client(
        httpx.MockTransport(lambda _: httpx.Response(200, json={"id": "project-1"}))
    )

    with pytest.raises(AssayProtocolError, match="get project: invalid response field 'name'"):
        client.projects.get("project-1")
    http_client.close()
