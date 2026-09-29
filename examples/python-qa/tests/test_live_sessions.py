"""Disposable chat acceptance against real Assay and PostgreSQL, without a model provider."""

from collections.abc import Iterator
from os import getenv
from types import SimpleNamespace
from typing import cast
from uuid import uuid4

import assay
import pytest
from fastapi.testclient import TestClient
from openai import OpenAI

import server
from app import TraceSession
from chat_sessions import COOKIE_NAME, SessionSigner

ORIGIN = {"Origin": "http://testserver"}
SIGNING_KEY = b"s" * 32  # Fixed only inside this disposable test; never use in a deployment.


class ModelFixture:
    def __init__(self) -> None:
        self.prompts: list[str] = []

    def create(self, *, input: str, **_kwargs: object) -> SimpleNamespace:
        self.prompts.append(input)
        return SimpleNamespace(output_text=f"reply {len(self.prompts)}", usage=None)


@pytest.fixture
def live_chat() -> Iterator[tuple[server.ChatService, ModelFixture, str]]:
    endpoint = getenv("ASSAY_ACCEPTANCE_ENDPOINT")
    admin_token = getenv("ASSAY_ACCEPTANCE_ADMIN_TOKEN")
    if not endpoint or not admin_token:
        pytest.skip("run through tests/acceptance/run.sh")
    with assay.Client(endpoint, admin_token=admin_token) as admin:
        suffix = uuid4().hex
        project = admin.projects.create(f"disposable-chat-{suffix}")
        try:
            key = admin.keys.create(project.id, "chat-acceptance")
            application = admin.applications.create(
                project.id, "Disposable chat", f"disposable-chat-{suffix}"
            )
            assay.init(
                endpoint=endpoint, api_key=key.key, application=application.slug, capture=True
            )
            with assay.Client(endpoint, admin_token=admin_token, api_key=key.key) as trace_client:
                fixture = ModelFixture()
                model = cast(
                    OpenAI,
                    SimpleNamespace(
                        base_url="http://model.invalid/v1",
                        responses=SimpleNamespace(create=fixture.create),
                    ),
                )
                service = server.ChatService(
                    TraceSession(application, trace_client),
                    model,
                    "disposable-model",
                    endpoint,
                    SessionSigner(SIGNING_KEY),
                )
                yield service, fixture, endpoint
        finally:
            assay.shutdown()
            admin.projects.delete(project.id)


def test_live_chat_restores_context_and_isolates_browsers(
    live_chat: tuple[server.ChatService, ModelFixture, str], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("ASSAY_CHAT_ORIGIN", "http://testserver")
    service, model, endpoint = live_chat
    with (
        TestClient(server.create_app(service)) as first,
        TestClient(server.create_app(service)) as second,
    ):
        first_session = first.get("/api/session")
        alice = first_session.json()
        bob = second.get("/api/session").json()
        assert alice["session_id"] != bob["session_id"]
        assert alice["browser_id"] != bob["browser_id"]
        assert "httponly" in first_session.headers["set-cookie"].lower()
        assert (
            first.post("/api/chat", json={"message": "First question"}, headers=ORIGIN).json()[
                "answer"
            ]
            == "reply 1"
        )
        assert (
            second.post("/api/chat", json={"message": "Bob question"}, headers=ORIGIN).json()[
                "answer"
            ]
            == "reply 2"
        )
        assert (
            first.post("/api/chat", json={"message": "Follow-up"}, headers=ORIGIN).json()["answer"]
            == "reply 3"
        )
        assert "user: First question\nassistant: reply 1" in model.prompts[2]
        assert "Bob question" not in model.prompts[2]
        assert [turn["question"] for turn in first.get("/api/session").json()["turns"]] == [
            "First question",
            "Follow-up",
        ]
        assert (
            second.get("/api/session", params={"session_id": alice["session_id"]}).json()[
                "session_id"
            ]
            == bob["session_id"]
        )
        signed_cookie = first.cookies.get(COOKIE_NAME)
        assert signed_cookie is not None
        second.cookies.set(COOKIE_NAME, alice["session_id"])
        assert (
            second.post("/api/chat", json={"message": "forged"}, headers=ORIGIN).status_code == 401
        )

    # Recreate the chat service with the same signing secret and Assay backend: no local
    # transcript is retained in the application instance or browser-supplied JSON.
    restarted = server.ChatService(
        service.session,
        service.client,
        service.model,
        endpoint,
        SessionSigner(SIGNING_KEY),
    )
    with TestClient(server.create_app(restarted)) as reopened:
        reopened.cookies.set(COOKIE_NAME, signed_cookie)
        restored = reopened.get("/api/session").json()
        assert restored["session_id"] == alice["session_id"]
        assert len(restored["turns"]) == 2
        rotated = reopened.post("/api/session", headers=ORIGIN).json()
        assert rotated["session_id"] != alice["session_id"]
        assert rotated["browser_id"] == alice["browser_id"]
        assert reopened.get("/api/session").json()["turns"] == []
