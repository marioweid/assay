import json
from types import SimpleNamespace
from typing import cast
from unittest.mock import Mock

import assay
import pytest
from fastapi.testclient import TestClient

import server
from chat_sessions import ChatIdentity, SessionSigner

ORIGIN = {"Origin": "http://testserver"}


@pytest.fixture(autouse=True)
def test_origin(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("ASSAY_CHAT_ORIGIN", "http://testserver")


def chat_service() -> tuple[server.ChatService, Mock]:
    service = Mock(spec=server.ChatService)
    service.signer = SessionSigner(b"s" * 32)
    service.secure_cookie = False
    service.config.return_value = {
        "model": "demo",
        "traces_url": "http://localhost:8080/apps/a/traces",
    }
    service.history.side_effect = lambda identity, _cursor: server.SessionView(
        session_id=identity.session_id,
        browser_id=identity.browser_id,
        turns=[],
        next_cursor=None,
    )
    service.respond.return_value = server.ChatReply(
        answer="Postgres",
        trace_url="http://localhost:8080/apps/a/traces/t",
        trace_exported=True,
    )
    return cast(server.ChatService, service), service


def test_evaluation_endpoint_uses_protected_stateless_target() -> None:
    session = Mock()
    session.application.id = "app-id"
    session.application.target_endpoint = None
    server.configure_evaluation(session, "http://chat:8090/api/evaluate", "derived-key")
    application_id, endpoint = session.client.applications.set_endpoint.call_args.args
    assert application_id == "app-id"
    assert endpoint.request_template["message"] == "{{ .item.input.question }}"
    assert endpoint.headers["X-Assay-Example-Key"] == "derived-key"
    assert endpoint.response_mapping.output == "$.answer"
    session.client.applications.set_endpoint.reset_mock()
    session.application.target_endpoint = endpoint
    server.configure_evaluation(session, "http://chat:8090/api/evaluate", "derived-key")
    session.client.applications.set_endpoint.assert_not_called()


def test_browser_capability_survives_refresh_and_rotates_only_on_new_chat() -> None:
    service, mocked = chat_service()
    with TestClient(server.create_app(service)) as browser:
        assert browser.get("/api/config").json() == service.config()
        first = browser.get("/api/session")
        assert first.status_code == 200
        assert "httponly" in first.headers["set-cookie"].lower()
        assert "samesite=lax" in first.headers["set-cookie"].lower()
        identity = first.json()
        assert browser.get("/api/session").json()["session_id"] == identity["session_id"]
        reply = browser.post("/api/chat", json={"message": "Where?"}, headers=ORIGIN)
        assert reply.status_code == 200
        assert reply.json()["answer"] == "Postgres"
        assert mocked.respond.call_args.args[0] == "Where?"
        assert mocked.respond.call_args.args[1] == ChatIdentity(
            identity["session_id"], identity["browser_id"]
        )
        rotated = browser.post("/api/session", headers=ORIGIN)
        assert rotated.status_code == 200
        assert rotated.json()["session_id"] != identity["session_id"]
        assert rotated.json()["browser_id"] == identity["browser_id"]
        assert browser.get("/api/session").json()["session_id"] == rotated.json()["session_id"]


def test_known_session_id_and_forged_cookie_cannot_read_another_browser() -> None:
    service, mocked = chat_service()
    with (
        TestClient(server.create_app(service)) as first,
        TestClient(server.create_app(service)) as second,
    ):
        known = first.get("/api/session").json()["session_id"]
        other = second.get("/api/session").json()["session_id"]
        assert other != known
        assert (
            second.get("/api/session", params={"session_id": known}).json()["session_id"] == other
        )
        second.cookies.set("assay_demo_chat", known)
        assert (
            second.post("/api/chat", json={"message": "stolen"}, headers=ORIGIN).status_code == 401
        )
        assert second.get("/api/session").json()["session_id"] not in {known, other}
        assert mocked.respond.call_count == 0


def test_host_header_cannot_define_an_allowed_origin(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("ASSAY_CHAT_ORIGIN")
    monkeypatch.setenv("ASSAY_CHAT_PORT", "8090")
    service, mocked = chat_service()
    with TestClient(server.create_app(service), base_url="http://localhost:8090") as browser:
        browser.get("/api/session")
        assert (
            browser.post(
                "/api/chat",
                json={"message": "allowed"},
                headers={"Origin": "http://localhost:8090"},
            ).status_code
            == 200
        )
        assert (
            browser.post(
                "/api/chat",
                json={"message": "rebound"},
                headers={
                    "Origin": "http://attacker.invalid",
                    "Host": "attacker.invalid",
                },
            ).status_code
            == 403
        )
        assert mocked.respond.call_count == 1


def test_history_is_paged_from_root_capture_and_missing_session_starts_empty() -> None:
    signer = SessionSigner(b"s" * 32)
    identity = signer.create()
    session = Mock()
    session.application.id = "app-id"
    session.client.sessions.turns.return_value = SimpleNamespace(
        items=[
            SimpleNamespace(
                id="trace-1",
                status="ok",
                attributes={
                    "gen_ai.input.messages": json.dumps([{"role": "user", "content": "Question"}]),
                    "gen_ai.output.messages": json.dumps(
                        [{"role": "assistant", "content": "Answer"}]
                    ),
                },
            ),
            SimpleNamespace(id="trace-2", status="error", attributes={}),
        ],
        next_cursor="page-two",
    )
    chat = server.ChatService(
        session,
        Mock(),
        "model",
        "http://localhost:8080",
        signer,
    )
    page = chat.history(identity, None)
    assert page.session_id == identity.session_id
    assert page.next_cursor == "page-two"
    assert page.turns[0].question == "Question"
    assert page.turns[0].answer == "Answer"
    assert page.turns[0].trace_url.endswith("/trace-1")
    assert page.turns[1].question is None and page.turns[1].status == "error"
    chat.history(identity, "page-two")
    assert session.client.sessions.turns.call_args.kwargs["cursor"] == "page-two"
    session.client.sessions.turns.side_effect = assay.AssayAPIError(
        operation="list session turns",
        status_code=404,
    )
    assert chat.history(identity, None).turns == []
    with pytest.raises(assay.AssayAPIError):
        chat.history(identity, "page-two")


def test_mutations_require_origin_and_latest_message_only() -> None:
    service, mocked = chat_service()
    with TestClient(server.create_app(service)) as browser:
        browser.get("/api/session")
        assert browser.post("/api/chat", json={"message": "x"}).status_code == 403
        assert browser.post("/api/session").status_code == 403
        assert (
            browser.post(
                "/api/chat", json={"message": "x"}, headers={"Origin": "http://evil.test"}
            ).status_code
            == 403
        )
        for payload in (
            {"messages": [{"role": "user", "content": "x"}]},
            {"message": " "},
            {"message": "x" * 4001},
        ):
            assert browser.post("/api/chat", json=payload, headers=ORIGIN).status_code == 422
        assert mocked.respond.call_count == 0


def test_provider_failure_has_no_ghost_reply_or_secret() -> None:
    service, mocked = chat_service()
    mocked.respond.side_effect = ValueError("empty answer")
    with TestClient(server.create_app(service)) as browser:
        browser.get("/api/session")
        result = browser.post("/api/chat", json={"message": "Hi"}, headers=ORIGIN)
        assert result.status_code == 502
        assert "empty answer" in result.json()["detail"]
        assert browser.get("/api/session").json()["turns"] == []


def test_offline_evaluation_rejects_missing_key_and_never_uses_chat_cookie() -> None:
    service, mocked = chat_service()
    with TestClient(server.create_app(service)) as browser:
        assert browser.post("/api/evaluate", json={"message": "question"}).status_code == 401
        assert (
            browser.post(
                "/api/evaluate",
                json={"message": "question"},
                headers={"X-Assay-Example-Key": "forged"},
            ).status_code
            == 401
        )
        response = browser.post(
            "/api/evaluate",
            json={"message": "question"},
            headers={"X-Assay-Example-Key": service.signer.evaluation_token()},
        )
        assert response.status_code == 200
        assert mocked.respond.call_args.args == ("question", None)
