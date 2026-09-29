import httpx
import pytest

import assay
from assay.cli import main
from assay.exceptions import AssayAPIError, AssayConfigurationError


def test_local_management_omits_auth_but_keeps_explicit_project_scope() -> None:
    requests: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json={"items": []})

    with httpx.Client(transport=httpx.MockTransport(respond)) as http_client:
        with assay.Client(
            "http://localhost:8080", local_mode=True, _http_client=http_client
        ) as client:
            assert client.projects.list() == ()
            client.traces.list(application_id="app")
        with assay.Client(
            "http://localhost:8080",
            local_mode=True,
            api_key="project-key",
            _http_client=http_client,
        ) as client:
            client.traces.list(application_id="app")
            client.projects.list()
    assert "authorization" not in requests[0].headers
    assert "x-api-key" not in requests[1].headers
    assert requests[2].headers["x-api-key"] == "project-key"
    assert "authorization" not in requests[3].headers
    assert "x-api-key" not in requests[3].headers


def test_local_mode_cannot_bypass_a_token_protected_server() -> None:
    with (
        httpx.Client(
            transport=httpx.MockTransport(
                lambda _: httpx.Response(401, json={"title": "Unauthorized"})
            )
        ) as http_client,
        assay.Client("http://localhost:8080", local_mode=True, _http_client=http_client) as client,
        pytest.raises(AssayAPIError),
    ):
        client.projects.list()


def test_local_mode_environment_is_opt_in_and_validated(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("ASSAY_LOCAL_MODE", raising=False)
    with (
        assay.Client("http://localhost:8080") as client,
        pytest.raises(AssayConfigurationError, match="admin credential"),
    ):
        client.projects.list()
    monkeypatch.setenv("ASSAY_LOCAL_MODE", "invalid")
    with pytest.raises(AssayConfigurationError, match="ASSAY_LOCAL_MODE"):
        assay.Client("http://localhost:8080")
    monkeypatch.setenv("ASSAY_LOCAL_MODE", "true")
    with (
        assay.Client("http://localhost:8080", local_mode=False) as client,
        pytest.raises(AssayConfigurationError, match="admin credential"),
    ):
        client.projects.list()


def test_cli_local_environment_and_tracing_still_requires_key(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("ASSAY_LOCAL_MODE", "true")
    monkeypatch.setenv("ASSAY_ENDPOINT", "http://localhost:8080")
    monkeypatch.delenv("ASSAY_ADMIN_TOKEN", raising=False)
    monkeypatch.delenv("ASSAY_API_KEY", raising=False)

    def respond(request: httpx.Request) -> httpx.Response:
        assert "authorization" not in request.headers
        return httpx.Response(200, json={"items": []})

    with httpx.Client(transport=httpx.MockTransport(respond)) as http_client:
        assert main(["projects", "list"], _http_client=http_client) == 0
    with pytest.raises(AssayConfigurationError, match="API key"):
        assay.init(endpoint="http://localhost:8080", application="demo")
