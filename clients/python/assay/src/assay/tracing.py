"""Assay tracing lifecycle, decorators, and explicit span helpers."""

from __future__ import annotations

import inspect
import math
import os
import threading
from collections.abc import Callable, Iterable, Mapping, Sequence
from contextlib import AbstractContextManager
from dataclasses import dataclass
from functools import wraps
from typing import Literal, ParamSpec, TypeVar, cast, overload

from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, SpanExporter, SpanProcessor
from opentelemetry.trace import Span, Tracer
from typing_extensions import override

from assay._exporter import AssaySpanExporter
from assay._serialization import serialize_capture
from assay.conventions import (
    ASSAY_APPLICATION_SLUG,
    ASSAY_CAPTURE_ERROR,
    ASSAY_DATASET_ID,
    ASSAY_REFERENCE_ANSWER,
    ASSAY_REFERENCE_ID,
    ASSAY_REFERENCE_SOURCE,
    ASSAY_RUN_ID,
    ASSAY_SCORABLE,
    ASSAY_SCORABLE_KIND,
    ASSAY_UNIT_ID,
    GEN_AI_INPUT_MESSAGES,
    GEN_AI_OUTPUT_MESSAGES,
    context_attributes,
    message_attribute,
    validate_chunks,
)
from assay.exceptions import AssayConfigurationError
from assay.models import AttributeValue, Chunk

P = ParamSpec("P")
R = TypeVar("R")
MessageRole = Literal["user", "assistant"]

_ExporterFactory = Callable[[str, str], SpanExporter]
_ProcessorFactory = Callable[[SpanExporter], SpanProcessor]


@dataclass(frozen=True, slots=True)
class _Config:
    endpoint: str
    api_key: str
    application: str
    service_name: str
    capture: bool
    max_capture_bytes: int


@dataclass(slots=True)
class _State:
    config: _Config
    provider: TracerProvider
    tracer: Tracer


_state: _State | None = None
_state_lock = threading.Lock()
_exporter_factory: _ExporterFactory = AssaySpanExporter
_processor_factory: _ProcessorFactory = BatchSpanProcessor


def init(
    endpoint: str | None = None,
    api_key: str | None = None,
    application: str | None = None,
    *,
    service_name: str | None = None,
    capture: bool = False,
    max_capture_bytes: int = 65_536,
) -> None:
    """Initialize Assay's private tracer provider.

    Explicit values take precedence over `ASSAY_ENDPOINT`, `ASSAY_API_KEY`, and
    `ASSAY_APPLICATION`. Reinitialization requires an identical effective configuration or a prior
    call to :func:`shutdown`.
    """
    config = _Config(
        endpoint=_required_value(endpoint, "ASSAY_ENDPOINT", "endpoint"),
        api_key=_required_value(api_key, "ASSAY_API_KEY", "API key"),
        application=_required_value(application, "ASSAY_APPLICATION", "application"),
        service_name=_service_name(service_name, application),
        capture=_capture_default(capture),
        max_capture_bytes=_capture_cap(max_capture_bytes),
    )
    global _state
    with _state_lock:
        if _state is not None:
            if _state.config == config:
                return
            raise AssayConfigurationError(
                "Assay is already initialized; call assay.shutdown() before reinitializing"
            )
        exporter = _exporter_factory(config.endpoint, config.api_key)
        provider = TracerProvider(
            resource=Resource.create(
                {
                    ASSAY_APPLICATION_SLUG: config.application,
                    "service.name": config.service_name,
                }
            )
        )
        provider.add_span_processor(_processor_factory(exporter))
        tracer = provider.get_tracer("assay", "0.2.0")
        _state = _State(config=config, provider=provider, tracer=tracer)


def flush(timeout_millis: int = 30_000) -> bool:
    """Flush spans completed before this call within the supplied timeout."""
    if timeout_millis <= 0:
        raise AssayConfigurationError("flush timeout must be positive")
    state = _required_state()
    return state.provider.force_flush(timeout_millis)


def shutdown() -> None:
    """Flush and stop the private provider; repeated calls are safe."""
    global _state
    with _state_lock:
        state = _state
        _state = None
    if state is not None:
        state.provider.shutdown()


@overload
def trace(func: Callable[P, R], /) -> Callable[P, R]: ...


@overload
def trace(
    func: None = None,
    /,
    *,
    name: str | None = None,
    capture: bool | None = None,
    redact: Callable[[object], object] | None = None,
    attributes: Mapping[str, AttributeValue] | None = None,
) -> Callable[[Callable[P, R]], Callable[P, R]]: ...


def trace(
    func: Callable[P, R] | None = None,
    /,
    *,
    name: str | None = None,
    capture: bool | None = None,
    redact: Callable[[object], object] | None = None,
    attributes: Mapping[str, AttributeValue] | None = None,
) -> Callable[P, R] | Callable[[Callable[P, R]], Callable[P, R]]:
    """Trace a synchronous or asynchronous function with optional content capture."""

    def decorate(target: Callable[P, R]) -> Callable[P, R]:
        span_name = _function_span_name(target, name)
        signature = inspect.signature(target)
        if inspect.iscoroutinefunction(target):

            @wraps(target)
            async def async_wrapper(*args: P.args, **kwargs: P.kwargs):
                state = _required_state()
                with state.tracer.start_as_current_span(
                    span_name,
                    attributes=dict(attributes or {}),
                ) as current:
                    _capture_input(
                        current,
                        signature,
                        args,
                        kwargs,
                        state=state,
                        capture_override=capture,
                        redact=redact,
                    )
                    result = await target(*args, **kwargs)
                    _capture_output(
                        current,
                        result,
                        state=state,
                        capture_override=capture,
                        redact=redact,
                    )
                    return result

            return cast(Callable[P, R], async_wrapper)

        @wraps(target)
        def sync_wrapper(*args: P.args, **kwargs: P.kwargs) -> R:
            state = _required_state()
            with state.tracer.start_as_current_span(
                span_name,
                attributes=dict(attributes or {}),
            ) as current:
                _capture_input(
                    current,
                    signature,
                    args,
                    kwargs,
                    state=state,
                    capture_override=capture,
                    redact=redact,
                )
                result = target(*args, **kwargs)
                _capture_output(
                    current,
                    result,
                    state=state,
                    capture_override=capture,
                    redact=redact,
                )
                return result

        return sync_wrapper

    if func is not None:
        return decorate(func)
    return decorate


def span(
    name: str,
    *,
    kind: str | None = None,
    scorable: bool = False,
    unit_id: str | None = None,
    dataset_id: str | None = None,
    run_id: str | None = None,
    reference: str | None = None,
    attributes: Mapping[str, AttributeValue] | None = None,
) -> AssaySpan:
    """Create an explicit Assay span context with evaluation helpers."""
    state = _required_state()
    span_attributes = _span_attributes(
        kind=kind,
        scorable=scorable,
        unit_id=unit_id,
        dataset_id=dataset_id,
        run_id=run_id,
        reference=reference,
        attributes=attributes,
    )
    return AssaySpan(
        state.tracer.start_as_current_span(
            _non_blank(name, "span name"), attributes=span_attributes
        ),
        state.config.max_capture_bytes,
    )


class AssaySpan(AbstractContextManager["AssaySpan"]):
    """Active span wrapper exposing Assay semantic-convention setters."""

    def __init__(self, context: AbstractContextManager[Span], max_capture_bytes: int) -> None:
        self._context = context
        self._max_capture_bytes = max_capture_bytes
        self._span: Span | None = None

    @override
    def __enter__(self) -> AssaySpan:
        self._span = self._context.__enter__()
        return self

    @override
    def __exit__(self, exc_type, exc_value, traceback) -> bool | None:
        try:
            return self._context.__exit__(exc_type, exc_value, traceback)
        finally:
            self._span = None

    def set_input(self, value: object) -> None:
        """Set the captured final user input message."""
        self._set_message(GEN_AI_INPUT_MESSAGES, "user", value)

    def set_output(self, value: object) -> None:
        """Set the captured first assistant output message."""
        self._set_message(GEN_AI_OUTPUT_MESSAGES, "assistant", value)

    def set_context(self, chunks: Iterable[Chunk | Mapping[str, object]]) -> None:
        """Set complete standard and flattened retrieval context attributes."""
        validated = validate_chunks(chunks)
        attributes = context_attributes(validated, self._max_capture_bytes)
        current = self._current()
        for name, value in attributes.items():
            current.set_attribute(name, value)

    def set_reference(
        self,
        text: str,
        reference_id: str | None = None,
        source: str | None = None,
    ) -> None:
        """Set a reference answer and optional audit identifiers."""
        reference = _non_blank(text, "reference")
        validated_id = _optional_non_blank(reference_id, "reference ID")
        validated_source = _optional_non_blank(source, "reference source")
        current = self._current()
        current.set_attribute(ASSAY_REFERENCE_ANSWER, reference)
        if validated_id is not None:
            current.set_attribute(ASSAY_REFERENCE_ID, validated_id)
        if validated_source is not None:
            current.set_attribute(ASSAY_REFERENCE_SOURCE, validated_source)

    def set_attribute(self, name: str, value: AttributeValue) -> None:
        """Set one validated primitive OpenTelemetry attribute."""
        attribute_name = _non_blank(name, "attribute name")
        _validate_attribute(value)
        self._current().set_attribute(attribute_name, value)

    def _set_message(
        self,
        name: str,
        role: MessageRole,
        value: object,
    ) -> None:
        content = serialize_capture(value, self._max_capture_bytes)
        self._current().set_attribute(name, message_attribute(role, content))

    def _current(self) -> Span:
        if self._span is None:
            raise RuntimeError("Assay span context is not active")
        return self._span


def _capture_input(
    current: Span,
    signature: inspect.Signature,
    args: tuple[object, ...],
    kwargs: Mapping[str, object],
    *,
    state: _State,
    capture_override: bool | None,
    redact: Callable[[object], object] | None,
) -> None:
    if not _capture_enabled(state, capture_override):
        return
    try:
        bound = signature.bind(*args, **kwargs)
        bound.apply_defaults()
        value: object = dict(bound.arguments)
    except Exception:
        current.set_attribute(ASSAY_CAPTURE_ERROR, "serialization")
        return
    _capture_message(
        current,
        GEN_AI_INPUT_MESSAGES,
        "user",
        value,
        state=state,
        redact=redact,
    )


def _capture_output(
    current: Span,
    value: object,
    *,
    state: _State,
    capture_override: bool | None,
    redact: Callable[[object], object] | None,
) -> None:
    if _capture_enabled(state, capture_override):
        _capture_message(
            current,
            GEN_AI_OUTPUT_MESSAGES,
            "assistant",
            value,
            state=state,
            redact=redact,
        )


def _capture_message(
    current: Span,
    name: str,
    role: MessageRole,
    value: object,
    *,
    state: _State,
    redact: Callable[[object], object] | None,
) -> None:
    if redact is not None:
        try:
            value = redact(value)
        except Exception:
            current.set_attribute(ASSAY_CAPTURE_ERROR, "redaction")
            return
    try:
        content = serialize_capture(value, state.config.max_capture_bytes)
        encoded = message_attribute(role, content)
    except Exception:
        current.set_attribute(ASSAY_CAPTURE_ERROR, "serialization")
        return
    try:
        current.set_attribute(name, encoded)
    except Exception:
        current.set_attribute(ASSAY_CAPTURE_ERROR, "attribute")


def _capture_enabled(state: _State, override: bool | None) -> bool:
    return state.config.capture if override is None else override


def _span_attributes(
    *,
    kind: str | None,
    scorable: bool,
    unit_id: str | None,
    dataset_id: str | None,
    run_id: str | None,
    reference: str | None,
    attributes: Mapping[str, AttributeValue] | None,
) -> dict[str, AttributeValue]:
    result: dict[str, AttributeValue] = dict(attributes) if attributes is not None else {}
    for name, value in result.items():
        _non_blank(name, "attribute name")
        _validate_attribute(value)
    if scorable:
        result[ASSAY_SCORABLE] = True
    optional_values: dict[str, str | None] = {
        ASSAY_SCORABLE_KIND: _optional_non_blank(kind, "scorable kind"),
        ASSAY_UNIT_ID: _optional_non_blank(unit_id, "unit ID"),
        ASSAY_DATASET_ID: _optional_non_blank(dataset_id, "dataset ID"),
        ASSAY_RUN_ID: _optional_non_blank(run_id, "run ID"),
        ASSAY_REFERENCE_ANSWER: _optional_non_blank(reference, "reference"),
    }
    for name, value in optional_values.items():
        if value is not None:
            result[name] = value
    return result


def _validate_attribute(value: AttributeValue) -> None:
    if isinstance(value, bool | str | int | float):
        if isinstance(value, float) and not math.isfinite(value):
            raise ValueError("attribute float must be finite")
        return
    if not isinstance(value, Sequence) or isinstance(value, bytes | bytearray | memoryview):
        raise ValueError("attribute value must be an OpenTelemetry primitive or sequence")
    if not value:
        raise ValueError("attribute sequence must not be empty")
    expected_type = type(value[0])
    if expected_type not in {bool, str, int, float}:
        raise ValueError("attribute sequence contains an unsupported type")
    if any(type(item) is not expected_type for item in value):
        raise ValueError("attribute sequence must contain one primitive type")
    if expected_type is float and any(not math.isfinite(cast(float, item)) for item in value):
        raise ValueError("attribute float must be finite")


def _required_state() -> _State:
    with _state_lock:
        if _state is None:
            raise AssayConfigurationError("Assay is not initialized; call assay.init() first")
        return _state


def _required_value(explicit: str | None, environment: str, label: str) -> str:
    value = explicit if explicit is not None else os.getenv(environment)
    if value is None or not value.strip():
        raise AssayConfigurationError(f"{label} is required")
    return value.strip()


def _service_name(explicit: str | None, application: str | None) -> str:
    if explicit is not None:
        return _non_blank(explicit, "service name")
    return _required_value(application, "ASSAY_APPLICATION", "application")


def _capture_default(value: bool) -> bool:
    if not isinstance(value, bool):
        raise AssayConfigurationError("capture must be a boolean")
    return value


def _capture_cap(value: int) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
        raise AssayConfigurationError("max capture bytes must be positive")
    return value


def _function_span_name(target: Callable[..., object], explicit: str | None) -> str:
    if explicit is not None:
        return _non_blank(explicit, "span name")
    module = getattr(target, "__module__", target.__class__.__module__)
    qualified_name = getattr(target, "__qualname__", target.__class__.__qualname__)
    return f"{module}.{qualified_name}"


def _non_blank(value: str, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{label} must not be blank")
    return value.strip()


def _optional_non_blank(value: str | None, label: str) -> str | None:
    if value is None:
        return None
    return _non_blank(value, label)
