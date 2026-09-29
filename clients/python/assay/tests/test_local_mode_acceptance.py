"""Opt-in local-mode acceptance against a disposable Assay/Postgres stack."""

import os
from collections.abc import Iterator
from contextlib import ExitStack
from urllib.parse import urlparse
from uuid import uuid4

import httpx
import pytest

import assay
from assay.cli import main
from assay.exceptions import AssayAPIError


@pytest.fixture
def local_server() -> Iterator[tuple[str, assay.Client]]:
    endpoint = os.getenv("ASSAY_ACCEPTANCE_ENDPOINT", "")
    project = os.getenv("ASSAY_ACCEPTANCE_PROJECT", "")
    if os.getenv("ASSAY_LOCAL_MODE") != "true" or not endpoint:
        pytest.skip("requires a disposable local-mode acceptance stack")
    url = urlparse(endpoint)
    if (
        not project.startswith("assay-acceptance-")
        or url.hostname not in {"localhost", "127.0.0.1"}
        or url.port in {None, 8080}
    ):
        pytest.fail("refusing a non-disposable local-mode acceptance endpoint")
    with httpx.Client(base_url=endpoint, timeout=10) as http:
        info = http.get("/v1/server-info")
        assert info.status_code == 200
        assert info.json()["local_mode"] is True
    with assay.Client(endpoint, local_mode=True) as admin:
        yield endpoint, admin


@pytest.mark.integration
def test_local_management_ingestion_and_project_isolation(
    local_server: tuple[str, assay.Client],
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    endpoint, admin = local_server
    with ExitStack() as cleanup:
        owner = admin.projects.create(f"local-owner-{uuid4().hex}")
        cleanup.callback(admin.projects.delete, owner.id)
        other = admin.projects.create(f"local-other-{uuid4().hex}")
        cleanup.callback(admin.projects.delete, other.id)
        monkeypatch.setenv("ASSAY_ENDPOINT", endpoint)
        monkeypatch.delenv("ASSAY_ADMIN_TOKEN", raising=False)
        assert main(["projects", "list"]) == 0
        assert owner.id in capsys.readouterr().out
        owner_key = admin.keys.create(owner.id, "owner")
        other_key = admin.keys.create(other.id, "other")
        app = admin.applications.create(owner.id, "Local acceptance", f"local-{uuid4().hex}")
        session_id = f"local-{uuid4().hex}"
        assay.init(endpoint=endpoint, api_key=owner_key.key, application=app.slug)
        try:
            with assay.session(session_id), assay.span("local-turn") as span:
                span.set_input("Synthetic local-mode question")
                span.set_output("Synthetic local-mode answer")
                otel_id = span.trace_id
            assert assay.flush()
        finally:
            assay.shutdown()
        traces = admin.traces.list(application_id=app.id, q=otel_id)
        assert len(traces.items) == 1
        trace_id = traces.items[0].id
        with assay.Client(endpoint, api_key=owner_key.key, local_mode=True) as scoped:
            assert scoped.traces.get(trace_id).id == trace_id
            assert scoped.sessions.list(app.id).items[0].id == session_id
        with assay.Client(endpoint, api_key=other_key.key, local_mode=True) as scoped:
            assert scoped.traces.list(application_id=app.id).items == ()
            assert scoped.sessions.list(app.id).items == ()
            with pytest.raises(AssayAPIError) as error:
                scoped.traces.get(trace_id)
            assert error.value.status_code == 404
        with httpx.Client(base_url=endpoint, timeout=10) as http:
            assert http.get("/v1/projects", headers={"X-API-Key": owner_key.key}).status_code == 401
            for headers in ({}, {"X-API-Key": "invalid"}, {"Authorization": "Bearer not-a-key"}):
                assert http.post("/v1/traces", json={}, headers=headers).status_code == 401
            assert http.delete(f"/v1/traces/{trace_id}").status_code == 204
        assert admin.traces.list(application_id=app.id, q=otel_id).items == ()


@pytest.mark.integration
def test_local_server_enforces_browser_and_host_guards(
    local_server: tuple[str, assay.Client],
) -> None:
    endpoint, _ = local_server
    with httpx.Client(base_url=endpoint, timeout=10) as http:
        for headers in (
            {"Host": "attacker.test"},
            {"Origin": "https://attacker.test"},
            {"Origin": "http://localhost:9999"},
            {"Sec-Fetch-Site": "cross-site"},
            {"Sec-Fetch-Site": "same-site"},
        ):
            assert http.get("/v1/projects", headers=headers).status_code == 403
        info = http.get("/v1/server-info")
        assert info.headers["Cache-Control"] == "no-store"
        assert http.get("/v1/projects", headers={"Origin": endpoint}).status_code == 200
