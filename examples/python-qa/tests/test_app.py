from collections.abc import Iterator
from contextlib import nullcontext
from types import SimpleNamespace
from typing import cast
from unittest.mock import Mock

import assay
import assay.tracing
import pytest
from openai import OpenAI
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.trace import StatusCode

import app
import server


@pytest.fixture
def spans(monkeypatch: pytest.MonkeyPatch) -> Iterator[InMemorySpanExporter]:
    exporter = InMemorySpanExporter()
    monkeypatch.setattr(assay.tracing, "_exporter_factory", lambda _url, _key: exporter)
    monkeypatch.setattr(assay.tracing, "_processor_factory", SimpleSpanProcessor)
    assay.init(endpoint="http://localhost:8080", api_key="test", application="qa", capture=True)
    yield exporter
    assay.shutdown()


def test_answer_exports_context_output_model_and_usage(spans: InMemorySpanExporter) -> None:
    client = Mock(spec=OpenAI)
    client.responses = Mock()
    client.responses.create.return_value = SimpleNamespace(
        output_text="Assay stores traces in Postgres.",
        usage=SimpleNamespace(input_tokens=20, output_tokens=8),
    )
    answer = app.answer_question(
        "Where does Assay store traces?", cast(OpenAI, client), "test-model"
    )
    assert answer == "Assay stores traces in Postgres."
    exported = spans.get_finished_spans()
    generation = next(span for span in exported if span.name == "generate-answer")
    assert generation.attributes is not None
    assert generation.attributes["assay.scorable"] is True
    assert generation.attributes["gen_ai.request.model"] == "test-model"
    assert generation.attributes["gen_ai.usage.input_tokens"] == 20
    assert "Postgres" in str(generation.attributes)
    assert any(span.name == "answer-question" for span in exported)
    assert generation.parent is not None


def test_general_question_is_answered_without_groundedness_scoring(
    spans: InMemorySpanExporter,
) -> None:
    client = Mock(spec=OpenAI)
    client.responses = Mock()
    client.responses.create.return_value = SimpleNamespace(
        output_text="Paris is the capital of France.", usage=None
    )
    answer = app.answer_question(
        "What is the capital of France?", cast(OpenAI, client), "local-model", scorable=False
    )
    assert "Paris" in answer
    instructions = client.responses.create.call_args.kwargs["instructions"]
    assert "general questions" in instructions.lower()
    assert "only this context" not in instructions
    generation = next(span for span in spans.get_finished_spans() if span.name == "generate-answer")
    assert generation.attributes is not None
    assert "assay.scorable" not in generation.attributes


@pytest.mark.parametrize(
    ("latest_question", "scorable"),
    [
        ("What is the capital of France?", False),
        ("What is an assay in chemistry?", False),
        ("Where does Assay store traces?", True),
    ],
)
def test_chat_scores_only_explicit_assay_questions(
    spans: InMemorySpanExporter, latest_question: str, scorable: bool
) -> None:
    client = Mock(spec=OpenAI)
    client.base_url = "http://ollama:11434/v1"
    client.responses = Mock()
    client.responses.create.return_value = SimpleNamespace(output_text="Answer.", usage=None)
    session = Mock()
    session.application.id = "app-id"
    session.client.traces.list.return_value = SimpleNamespace(items=[])
    chat = server.ChatService(session, cast(OpenAI, client), "local-model", "http://localhost:8080")
    reply = chat.respond(
        server.ChatRequest(
            messages=[
                server.Message(role="user", content="Tell me about Assay."),
                server.Message(role="assistant", content="It records traces."),
                server.Message(role="user", content=latest_question),
            ]
        )
    )
    assert reply.answer == "Answer."
    generation = next(span for span in spans.get_finished_spans() if span.name == "generate-answer")
    assert generation.attributes is not None
    assert generation.attributes.get("assay.scorable", False) is scorable
    assert generation.attributes["gen_ai.provider.name"] == "ollama"


def test_cli_general_question_does_not_schedule_assay_groundedness(
    spans: InMemorySpanExporter,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    client = Mock(spec=OpenAI)
    client.base_url = "http://ollama:11434/v1"
    client.responses = Mock()
    client.responses.create.return_value = SimpleNamespace(output_text="Paris.", usage=None)
    monkeypatch.setenv("ASSAY_ADMIN_TOKEN", "test-admin")
    monkeypatch.setenv("OPENAI_API_KEY", "ollama")
    monkeypatch.setenv("OPENAI_MODEL", "test-model")
    monkeypatch.setattr(app.sys, "argv", ["app.py", "What is the capital of France?"])
    session = SimpleNamespace(application=SimpleNamespace(id="app-id"))
    monkeypatch.setattr(app, "traced_session", lambda _settings: nullcontext(session))
    monkeypatch.setattr(app, "OpenAI", lambda **_kwargs: nullcontext(client))

    assert app.main() == 0
    assert "without groundedness scoring" in capsys.readouterr().out
    generation = next(span for span in spans.get_finished_spans() if span.name == "generate-answer")
    assert generation.attributes is not None
    assert "assay.scorable" not in generation.attributes


def test_empty_model_answer_is_a_traced_failure(spans: InMemorySpanExporter) -> None:
    client = Mock(spec=OpenAI)
    client.responses = Mock()
    client.responses.create.return_value = SimpleNamespace(output_text="", usage=None)
    with pytest.raises(ValueError, match="empty answer"):
        app.answer_question("Where?", cast(OpenAI, client), "test-model")
    generation = next(span for span in spans.get_finished_spans() if span.name == "generate-answer")
    assert generation.status.status_code == StatusCode.ERROR


def test_local_model_settings_override_invalid_fallback_judge() -> None:
    config = app.Settings.from_env(
        {
            "ASSAY_ADMIN_TOKEN": "admin",
            "ASSAY_JUDGE_API_KEY": "invalid-judge",
            "OPENAI_API_KEY": "ollama",
            "OPENAI_MODEL": "qwen3:4b",
            "OPENAI_BASE_URL": "http://ollama:11434/v1",
        }
    )
    assert config.model == "qwen3:4b"
    assert config.openai_key == "ollama"
    assert config.base_url == "http://ollama:11434/v1"


def test_config_uses_existing_judge_settings() -> None:
    config = app.Settings.from_env(
        {
            "ASSAY_ADMIN_TOKEN": "admin",
            "ASSAY_JUDGE_API_KEY": "secret",
            "ASSAY_JUDGE_MODEL": "configured-model",
        }
    )
    assert config.endpoint == "http://localhost:8080"
    assert config.model == "configured-model"
    assert "secret" not in repr(config)
    assert "admin" not in repr(config)


def test_missing_model_fails_before_requests() -> None:
    with pytest.raises(ValueError, match="Set OPENAI_MODEL or ASSAY_JUDGE_MODEL"):
        app.Settings.from_env({"ASSAY_ADMIN_TOKEN": "admin", "OPENAI_API_KEY": "ollama"})


@pytest.mark.parametrize("missing", ["ASSAY_ADMIN_TOKEN", "ASSAY_JUDGE_API_KEY"])
def test_missing_credentials_fail_before_requests(missing: str) -> None:
    env = {
        "ASSAY_ADMIN_TOKEN": "admin",
        "ASSAY_JUDGE_API_KEY": "secret",
        "ASSAY_JUDGE_MODEL": "test-model",
    }
    del env[missing]
    with pytest.raises(ValueError, match="Set"):
        app.Settings.from_env(env)
