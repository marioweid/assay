from unittest.mock import Mock

from fastapi.testclient import TestClient

import server


def test_evaluation_endpoint_maps_dataset_question_to_chat() -> None:
    session = Mock()
    session.application.id = "app-id"
    session.application.target_endpoint = None
    server.configure_evaluation(session, "http://chat:8090/api/chat")
    application_id, endpoint = session.client.applications.set_endpoint.call_args.args
    assert application_id == "app-id"
    assert endpoint.request_template["messages"][0]["content"] == "{{ .item.input.question }}"
    assert endpoint.response_mapping.output == "$.answer"
    session.client.applications.set_endpoint.reset_mock()
    session.application.target_endpoint = endpoint
    server.configure_evaluation(session, "http://chat:8090/api/chat")
    session.client.applications.set_endpoint.assert_not_called()


def test_chat_validates_messages_and_exposes_only_public_config() -> None:
    service = Mock(spec=server.ChatService)
    service.config.return_value = {
        "model": "demo",
        "traces_url": "http://localhost:8080/apps/a/traces",
    }
    service.respond.return_value = server.ChatReply(
        answer="Postgres",
        trace_url="http://localhost:8080/apps/a/traces/t",
        trace_exported=True,
    )
    with TestClient(server.create_app(service)) as client:
        assert client.get("/api/config").json() == service.config.return_value
        response = client.post(
            "/api/chat", json={"messages": [{"role": "user", "content": "Where?"}]}
        )
        assert response.status_code == 200
        assert response.json()["answer"] == "Postgres"
        assert response.json()["trace_exported"] is True
        for messages in (
            [],
            [{"role": "system", "content": "x"}],
            [{"role": "user", "content": " "}],
            [{"role": "assistant", "content": "x"}],
        ):
            assert client.post("/api/chat", json={"messages": messages}).status_code == 422


def test_provider_failure_returns_actionable_error_without_secret() -> None:
    service = Mock(spec=server.ChatService)
    service.respond.side_effect = ValueError("empty answer")
    with TestClient(server.create_app(service)) as client:
        response = client.post("/api/chat", json={"messages": [{"role": "user", "content": "Hi"}]})
    assert response.status_code == 502
    assert "empty answer" in response.json()["detail"]
