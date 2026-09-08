from collections.abc import Iterator
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
    answer = app.answer_question("Where are traces stored?", cast(OpenAI, client), "test-model")
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


def test_empty_model_answer_is_a_traced_failure(spans: InMemorySpanExporter) -> None:
    client = Mock(spec=OpenAI)
    client.responses = Mock()
    client.responses.create.return_value = SimpleNamespace(output_text="", usage=None)
    with pytest.raises(ValueError, match="empty answer"):
        app.answer_question("Where?", cast(OpenAI, client), "test-model")
    generation = next(span for span in spans.get_finished_spans() if span.name == "generate-answer")
    assert generation.status.status_code == StatusCode.ERROR


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


@pytest.mark.parametrize("missing", ["ASSAY_ADMIN_TOKEN", "ASSAY_JUDGE_API_KEY"])
def test_missing_credentials_fail_before_requests(missing: str) -> None:
    env = {"ASSAY_ADMIN_TOKEN": "admin", "ASSAY_JUDGE_API_KEY": "secret"}
    del env[missing]
    with pytest.raises(ValueError, match="Set"):
        app.Settings.from_env(env)
