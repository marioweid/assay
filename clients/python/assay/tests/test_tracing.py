import asyncio
import inspect
import json
from collections.abc import Callable, Iterator, Mapping
from typing import cast

import pytest
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.trace import StatusCode, get_tracer_provider

import assay
import assay.tracing as tracing_module
from assay.exceptions import AssayConfigurationError
from assay.models import AttributeValue, Chunk


@pytest.fixture(autouse=True)
def reset_tracing() -> Iterator[None]:
    assay.shutdown()
    yield
    assay.shutdown()


@pytest.fixture
def exporter(monkeypatch: pytest.MonkeyPatch) -> InMemorySpanExporter:
    memory = InMemorySpanExporter()
    monkeypatch.setattr(tracing_module, "_exporter_factory", lambda _endpoint, _key: memory)
    monkeypatch.setattr(tracing_module, "_processor_factory", SimpleSpanProcessor)
    return memory


def initialize(**overrides: object) -> None:
    arguments: dict[str, object] = {
        "endpoint": "https://assay.test",
        "api_key": "asy_secret",
        "application": "support-bot",
    }
    arguments.update(overrides)
    cast(Callable[..., None], assay.init)(**arguments)


def test_init_uses_private_provider_and_application_resource(
    exporter: InMemorySpanExporter,
) -> None:
    global_provider = get_tracer_provider()
    initialize()

    @assay.trace
    def answer(question: str) -> str:
        return f"Answer: {question}"

    answer("What is Assay?")
    assert assay.flush()

    assert get_tracer_provider() is global_provider
    span = _span_named(exporter, f"{answer.__module__}.{answer.__qualname__}")
    assert span.resource.attributes["assay.application.slug"] == "support-bot"
    assert span.resource.attributes["service.name"] == "support-bot"


def test_init_uses_explicit_service_name(exporter: InMemorySpanExporter) -> None:
    initialize(service_name="customer-api")

    with assay.span("operation"):
        pass

    span = _span_named(exporter, "operation")
    assert span.resource.attributes["service.name"] == "customer-api"


def test_decorator_can_be_declared_before_init(exporter: InMemorySpanExporter) -> None:
    @assay.trace
    def answer() -> str:
        return "answer"

    with pytest.raises(AssayConfigurationError, match=r"call assay\.init"):
        answer()

    initialize()

    assert answer() == "answer"
    assert len(exporter.get_finished_spans()) == 1


def test_init_reads_environment_fallbacks(
    exporter: InMemorySpanExporter,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("ASSAY_ENDPOINT", "https://environment.test")
    monkeypatch.setenv("ASSAY_API_KEY", "asy_environment")
    monkeypatch.setenv("ASSAY_APPLICATION", "environment-app")

    assay.init()
    with assay.span("operation"):
        pass

    span = _span_named(exporter, "operation")
    assert span.resource.attributes["assay.application.slug"] == "environment-app"


@pytest.mark.parametrize(
    ("field", "value", "message"),
    [
        ("endpoint", "", "endpoint is required"),
        ("api_key", " ", "API key is required"),
        ("application", "", "application is required"),
        ("max_capture_bytes", 0, "max capture bytes must be positive"),
    ],
)
def test_init_rejects_invalid_configuration(
    field: str,
    value: object,
    message: str,
) -> None:
    with pytest.raises(AssayConfigurationError, match=message):
        initialize(**{field: value})


def test_identical_init_is_noop_but_conflicting_init_fails(
    exporter: InMemorySpanExporter,
) -> None:
    initialize()
    initialize()

    with pytest.raises(AssayConfigurationError, match=r"shutdown.*reinitializing"):
        initialize(application="other-app")

    with assay.span("operation"):
        pass
    assert len(exporter.get_finished_spans()) == 1


def test_shutdown_is_idempotent_and_allows_reinitialization(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    exporters: list[InMemorySpanExporter] = []

    def create_exporter(_endpoint: str, _key: str) -> InMemorySpanExporter:
        exporter = InMemorySpanExporter()
        exporters.append(exporter)
        return exporter

    monkeypatch.setattr(tracing_module, "_exporter_factory", create_exporter)
    monkeypatch.setattr(tracing_module, "_processor_factory", SimpleSpanProcessor)
    initialize()
    assay.shutdown()
    assay.shutdown()

    initialize(application="other-app")
    with assay.span("operation"):
        pass

    assert len(exporters) == 2
    span = _span_named(exporters[1], "operation")
    assert span.resource.attributes["assay.application.slug"] == "other-app"


def test_flush_requires_initialization() -> None:
    with pytest.raises(AssayConfigurationError, match=r"call assay\.init"):
        assay.flush()


def test_trace_does_not_capture_content_by_default(
    exporter: InMemorySpanExporter,
) -> None:
    initialize()

    @assay.trace(name="answer")
    def answer(question: str, count: int = 2) -> str:
        return "Assay evaluates AI systems."

    assert answer("What is Assay?") == "Assay evaluates AI systems."

    span = _span_named(exporter, "answer")
    attributes = _attributes(span.attributes)
    assert "gen_ai.input.messages" not in attributes
    assert "gen_ai.output.messages" not in attributes
    assert "assay.scorable" not in attributes


def test_trace_capture_can_be_enabled(exporter: InMemorySpanExporter) -> None:
    initialize()

    @assay.trace(capture=True)
    def answer(question: str) -> str:
        return question

    answer("captured")

    attributes = _attributes(
        _span_named(exporter, "test_trace_capture_can_be_enabled.<locals>.answer").attributes
    )
    assert json.loads(_message_content(attributes, "gen_ai.input.messages")) == {
        "question": "captured"
    }
    assert _message_content(attributes, "gen_ai.output.messages") == "captured"


def test_trace_redacts_before_capture(exporter: InMemorySpanExporter) -> None:
    initialize(capture=True)

    def redact(value: object) -> object:
        if isinstance(value, Mapping):
            return {"question": "[redacted]"}
        return "[redacted]"

    @assay.trace(name="redacted", redact=redact)
    def answer(question: str) -> str:
        return f"secret output for {question}"

    answer("secret input")

    attributes = _attributes(_span_named(exporter, "redacted").attributes)
    assert "secret" not in repr(attributes)
    assert json.loads(_message_content(attributes, "gen_ai.input.messages")) == {
        "question": "[redacted]"
    }
    assert _message_content(attributes, "gen_ai.output.messages") == "[redacted]"


def test_trace_redaction_failure_is_fail_closed(exporter: InMemorySpanExporter) -> None:
    initialize(capture=True)

    def reject(_: object) -> object:
        raise RuntimeError("secret callback failure")

    @assay.trace(name="failed-redaction", redact=reject)
    def answer(question: str) -> str:
        return f"secret output for {question}"

    assert answer("secret input") == "secret output for secret input"

    attributes = _attributes(_span_named(exporter, "failed-redaction").attributes)
    assert attributes["assay.capture.error"] == "redaction"
    assert "gen_ai.input.messages" not in attributes
    assert "gen_ai.output.messages" not in attributes
    assert "secret" not in repr(attributes)


def test_trace_caps_captured_content(exporter: InMemorySpanExporter) -> None:
    initialize(capture=True, max_capture_bytes=15)

    @assay.trace(name="capped")
    def answer(question: str) -> str:
        return question

    answer("é" * 100)

    attributes = _attributes(_span_named(exporter, "capped").attributes)
    assert len(_message_content(attributes, "gen_ai.output.messages").encode("utf-8")) <= 15


def test_trace_preserves_metadata_and_async_behavior(exporter: InMemorySpanExporter) -> None:
    initialize()

    @assay.trace
    async def answer(question: str) -> str:
        """Answer one question."""
        await asyncio.sleep(0)
        return question

    result = asyncio.run(answer("async answer"))

    assert result == "async answer"
    assert answer.__name__ == "answer"
    assert answer.__doc__ == "Answer one question."
    assert list(inspect.signature(answer).parameters) == ["question"]
    assert len(exporter.get_finished_spans()) == 1


def test_trace_records_and_reraises_function_exception(exporter: InMemorySpanExporter) -> None:
    initialize()

    @assay.trace(name="failure")
    def fail() -> None:
        raise LookupError("application failure")

    with pytest.raises(LookupError, match="application failure"):
        fail()

    span = _span_named(exporter, "failure")
    assert span.status.status_code is StatusCode.ERROR
    assert any(event.name == "exception" for event in span.events)


def test_trace_inherits_current_context_from_another_provider(
    exporter: InMemorySpanExporter,
) -> None:
    initialize()
    parent_provider = TracerProvider()
    parent_tracer = parent_provider.get_tracer("parent.tests")

    with parent_tracer.start_as_current_span("parent") as parent, assay.span("child"):
        pass

    child = _span_named(exporter, "child")
    assert child.parent is not None
    assert child.parent.span_id == parent.get_span_context().span_id
    parent_provider.shutdown()


def test_span_helper_writes_scorable_content_context_and_reference(
    exporter: InMemorySpanExporter,
) -> None:
    initialize()

    with assay.span(
        "generation",
        kind="rag_answer",
        scorable=True,
        unit_id="case-1",
        dataset_id="dataset-1",
        run_id="run-1",
        reference="Assay evaluates AI systems.",
        attributes={"gen_ai.operation.name": "chat"},
    ) as current:
        current.set_input("What is Assay?")
        current.set_output("Assay evaluates AI systems.")
        current.set_context([Chunk(id="k0", text="Assay evaluates AI systems.")])
        current.set_reference(
            "Assay evaluates AI systems.",
            reference_id="reference-1",
            source="curated",
        )
        current.set_attribute("gen_ai.usage.output_tokens", 5)

    attributes = _attributes(_span_named(exporter, "generation").attributes)
    assert attributes["assay.scorable"] is True
    assert attributes["assay.scorable.kind"] == "rag_answer"
    assert attributes["assay.unit.id"] == "case-1"
    assert attributes["assay.dataset.id"] == "dataset-1"
    assert attributes["assay.run.id"] == "run-1"
    assert attributes["assay.reference.answer"] == "Assay evaluates AI systems."
    assert attributes["assay.reference.id"] == "reference-1"
    assert attributes["assay.reference.source"] == "curated"
    assert attributes["assay.context.chunk.count"] == 1
    assert attributes["gen_ai.usage.output_tokens"] == 5


def test_span_context_validation_writes_no_partial_chunk_attributes(
    exporter: InMemorySpanExporter,
) -> None:
    initialize()

    with assay.span("generation") as current, pytest.raises(ValueError, match="duplicated"):
        current.set_context(
            [
                Chunk(id="k0", text="one"),
                Chunk(id="k0", text="two"),
            ]
        )

    attributes = _attributes(_span_named(exporter, "generation").attributes)
    assert not any(key.startswith("assay.context") for key in attributes)


def test_span_rejects_blank_reference_before_writing(exporter: InMemorySpanExporter) -> None:
    initialize()

    with (
        assay.span("generation") as current,
        pytest.raises(ValueError, match="reference must not be blank"),
    ):
        current.set_reference(" ")

    attributes = _attributes(_span_named(exporter, "generation").attributes)
    assert "assay.reference.answer" not in attributes


def _span_named(exporter: InMemorySpanExporter, name: str):
    matches = [span for span in exporter.get_finished_spans() if span.name.endswith(name)]
    assert len(matches) == 1
    return matches[0]


def _attributes(
    attributes: Mapping[str, AttributeValue] | None,
) -> Mapping[str, AttributeValue]:
    assert attributes is not None
    return attributes


def _message_content(attributes: Mapping[str, AttributeValue], key: str) -> str:
    encoded = attributes[key]
    assert isinstance(encoded, str)
    messages = json.loads(encoded)
    return cast(str, messages[0]["content"])
