"""Project-scoped session management client contract."""

from datetime import datetime, timezone

import httpx
import pytest

from assay.client import Client
from assay.exceptions import AssayConfigurationError, AssayProtocolError

STAMP = "2026-09-01T10:00:00Z"
SESSION = {
    "id": "opaque session / one",
    "start_time": STAMP,
    "end_time": STAMP,
    "turn_count": 2,
    "first_operation": "answer",
    "last_trace_id": "trace-2",
}
TURN = {
    "id": "trace-1",
    "root_name": "answer",
    "start_time": STAMP,
    "end_time": STAMP,
    "status": "ok",
    "span_count": 2,
    "total_tokens": 8,
    "attributes": {"gen_ai.input.messages": "[]"},
}


def test_session_reads_are_scoped_and_parse_root_capture() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.url.path == "/v1/sessions":
            return httpx.Response(200, json={"items": [SESSION], "next_cursor": "next"})
        return httpx.Response(200, json={"items": [TURN]})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", api_key="project-key", _http_client=http_client)
    page = client.sessions.list("application-1", limit=1)
    turns = client.sessions.turns("application-1", page.items[0].id, cursor=page.next_cursor)
    recent = client.sessions.recent("application-1", page.items[0].id)

    assert page.items[0].start_time == datetime(2026, 9, 1, 10, tzinfo=timezone.utc)
    assert page.next_cursor == "next"
    assert turns.items[0].attributes["gen_ai.input.messages"] == "[]"
    assert recent[0].id == "trace-1"
    assert requests[0].url.params["application_id"] == "application-1"
    assert requests[1].url.params["application_id"] == "application-1"
    assert requests[1].url.params["cursor"] == "next"
    assert requests[1].url.params["session_id"] == "opaque session / one"
    assert requests[1].url.path == "/v1/session-turns"
    assert requests[2].url.path == "/v1/session-turns/recent"
    assert requests[2].url.params["limit"] == "19"
    assert all(request.headers["x-api-key"] == "project-key" for request in requests)
    http_client.close()


def test_opaque_unicode_whitespace_is_not_trimmed_into_another_session() -> None:
    seen: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        session_id = request.url.params["session_id"]
        seen.append(session_id)
        return httpx.Response(200, json={"items": [{**TURN, "root_name": session_id}]})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", api_key="project-key", _http_client=http_client)
    original = "\u00a0session"
    assert client.sessions.turns("app", original).items[0].root_name == original
    assert client.sessions.turns("app", "session").items[0].root_name == "session"
    assert client.sessions.turns("app", "\u00a0").items[0].root_name == "\u00a0"
    assert seen == [original, "session", "\u00a0"]
    http_client.close()


def test_session_validation_and_malformed_response() -> None:
    requests = 0

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal requests
        requests += 1
        return httpx.Response(200, json={"items": [{**TURN, "attributes": []}]})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    client = Client("https://assay.test", api_key="project-key", _http_client=http_client)
    with pytest.raises(AssayConfigurationError):
        client.sessions.list(" ")
    with pytest.raises(AssayConfigurationError):
        client.sessions.turns("app", " ")
    with pytest.raises(AssayConfigurationError):
        client.sessions.list("app", limit=201)
    with pytest.raises(AssayConfigurationError):
        client.sessions.recent("app", "session", limit=20)
    assert requests == 0
    with pytest.raises(AssayProtocolError):
        client.sessions.turns("app", "session")
    assert requests == 1
    http_client.close()
