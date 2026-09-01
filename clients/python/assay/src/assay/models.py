"""Shared immutable value objects for the Assay SDK."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from datetime import datetime
from types import MappingProxyType
from typing import Generic, TypeAlias, TypeVar, cast

JSONValue: TypeAlias = bool | int | float | str | list["JSONValue"] | dict[str, "JSONValue"] | None
JSONObject: TypeAlias = dict[str, JSONValue]
AttributeScalar: TypeAlias = bool | str | int | float
AttributeValue: TypeAlias = (
    AttributeScalar | Sequence[bool] | Sequence[str] | Sequence[int] | Sequence[float]
)

T = TypeVar("T")


def _freeze_mapping(values: Mapping[str, object]) -> Mapping[str, JSONValue]:
    frozen = {key: _freeze_json(value) for key, value in values.items()}
    return cast(Mapping[str, JSONValue], MappingProxyType(frozen))


def _freeze_json(value: object) -> object:
    if isinstance(value, Mapping):
        return MappingProxyType({str(key): _freeze_json(item) for key, item in value.items()})
    if isinstance(value, list | tuple):
        return tuple(_freeze_json(item) for item in value)
    return value


@dataclass(frozen=True, slots=True)
class Chunk:
    """One retrieved context unit supplied to tracing or evaluation APIs."""

    id: str
    text: str
    score: float | None = None
    source: str | None = None


@dataclass(frozen=True, slots=True)
class Page(Generic[T]):
    """One explicit page from a cursor-paginated API collection."""

    items: tuple[T, ...]
    next_cursor: str | None = None


@dataclass(frozen=True, slots=True)
class JudgeConfig:
    """Writable OpenAI-compatible judge configuration."""

    base_url: str
    model: str
    api_key: str | None = field(default=None, repr=False)


@dataclass(frozen=True, slots=True)
class ResponseMapping:
    """JSONPath expressions used to read a target endpoint response."""

    output: str
    context: str | None = None


@dataclass(frozen=True, slots=True)
class TargetEndpoint:
    """Writable application target endpoint configuration."""

    url: str
    response_mapping: ResponseMapping
    method: str = "POST"
    headers: Mapping[str, str] = field(default_factory=dict)
    request_template: Mapping[str, JSONValue] = field(default_factory=dict)
    timeout_ms: int = 30_000
    secret: str | None = field(default=None, repr=False)

    def __post_init__(self) -> None:
        object.__setattr__(self, "headers", MappingProxyType(dict(self.headers)))
        object.__setattr__(self, "request_template", _freeze_mapping(self.request_template))


@dataclass(frozen=True, slots=True)
class JudgeConfigView:
    """Read-only judge configuration without credential material."""

    base_url: str
    model: str
    has_api_key: bool


@dataclass(frozen=True, slots=True)
class Project:
    """Assay project returned by the management API."""

    id: str
    name: str
    judge_config: JudgeConfigView | None
    created_at: datetime
    updated_at: datetime


@dataclass(frozen=True, slots=True)
class APIKey:
    """Project API-key metadata without plaintext key material."""

    id: str
    project_id: str
    name: str
    key_prefix: str
    last_used_at: datetime | None
    revoked_at: datetime | None
    created_at: datetime
    updated_at: datetime


@dataclass(frozen=True, slots=True)
class CreatedAPIKey(APIKey):
    """New API key including its one-time plaintext value."""

    key: str = field(repr=False)


@dataclass(frozen=True, slots=True)
class ResponseMappingView:
    """Read-only target response JSONPath mapping."""

    output: str
    context: str | None


@dataclass(frozen=True, slots=True)
class TargetEndpointView:
    """Read-only target endpoint without secret material."""

    url: str
    method: str
    headers: Mapping[str, str]
    request_template: Mapping[str, JSONValue]
    response_mapping: ResponseMappingView
    timeout_ms: int
    has_secret: bool

    def __post_init__(self) -> None:
        object.__setattr__(self, "headers", MappingProxyType(dict(self.headers)))
        object.__setattr__(self, "request_template", _freeze_mapping(self.request_template))


@dataclass(frozen=True, slots=True)
class Application:
    """Assay application and its optional target endpoint."""

    id: str
    project_id: str
    name: str
    slug: str
    config: Mapping[str, JSONValue]
    auto_score_scorers: tuple[str, ...]
    target_endpoint: TargetEndpointView | None
    created_at: datetime
    updated_at: datetime

    def __post_init__(self) -> None:
        object.__setattr__(self, "config", _freeze_mapping(self.config))
        object.__setattr__(self, "auto_score_scorers", tuple(self.auto_score_scorers))


@dataclass(frozen=True, slots=True)
class Dataset:
    """Named collection of offline evaluation cases."""

    id: str
    application_id: str
    name: str
    description: str | None
    created_at: datetime
    updated_at: datetime


@dataclass(frozen=True, slots=True)
class DatasetItemInput:
    """Writable dataset item accepted by bulk creation and import workflows."""

    input: Mapping[str, JSONValue]
    external_id: str | None = None
    output: str | None = None
    expected_output: str | None = None
    context: tuple[Chunk, ...] = ()
    metadata: Mapping[str, JSONValue] = field(default_factory=dict)

    def __post_init__(self) -> None:
        object.__setattr__(self, "input", _freeze_mapping(self.input))
        object.__setattr__(self, "context", tuple(self.context))
        object.__setattr__(self, "metadata", _freeze_mapping(self.metadata))


@dataclass(frozen=True, slots=True)
class DatasetItem:
    """Persisted offline evaluation case."""

    id: str
    dataset_id: str
    external_id: str | None
    input: Mapping[str, JSONValue]
    output: str | None
    expected_output: str | None
    context: tuple[Chunk, ...]
    metadata: Mapping[str, JSONValue]
    created_at: datetime
    updated_at: datetime

    def __post_init__(self) -> None:
        object.__setattr__(self, "input", _freeze_mapping(self.input))
        object.__setattr__(self, "context", tuple(self.context))
        object.__setattr__(self, "metadata", _freeze_mapping(self.metadata))


@dataclass(frozen=True, slots=True)
class ImportResult:
    """Summary of a completed batched dataset import."""

    dataset_id: str
    created_items: int
    batches: int


@dataclass(frozen=True, slots=True)
class ScorerConfig:
    """Resolved scorer configuration for an application."""

    id: str | None
    application_id: str
    scorer: str
    enabled: bool
    threshold: float
    judge_config: JudgeConfigView | None
    prompt_template_id: str
    persisted: bool


@dataclass(frozen=True, slots=True)
class ScoreAggregate:
    """Aggregate result for one scorer in an evaluation run."""

    mean: float
    pass_rate: float
    n: int


@dataclass(frozen=True, slots=True)
class EvalRun:
    """Evaluation run and its current aggregate state."""

    id: str
    application_id: str
    dataset_id: str
    name: str
    status: str
    mode: str
    params: Mapping[str, JSONValue]
    scorers: tuple[str, ...]
    aggregates: Mapping[str, ScoreAggregate]
    total_items: int
    succeeded_items: int
    failed_items: int
    canceled_items: int
    started_at: datetime | None
    finished_at: datetime | None
    error: str | None
    created_at: datetime
    updated_at: datetime

    def __post_init__(self) -> None:
        object.__setattr__(self, "params", _freeze_mapping(self.params))
        object.__setattr__(self, "scorers", tuple(self.scorers))
        object.__setattr__(self, "aggregates", MappingProxyType(dict(self.aggregates)))


@dataclass(frozen=True, slots=True)
class EvalRunItem:
    """Execution state for one dataset item in an evaluation run."""

    eval_run_id: str
    dataset_item_id: str
    status: str
    error: str | None
    started_at: datetime | None
    finished_at: datetime | None
    created_at: datetime
    updated_at: datetime
    generated_output: str | None
    generated_context: tuple[Chunk, ...]
    generated_at: datetime | None


@dataclass(frozen=True, slots=True)
class Score:
    """One immutable online or offline score snapshot."""

    id: int
    scorer: str
    scorer_config_id: str | None
    value: float
    threshold: float
    passed: bool
    rationale: str
    details: Mapping[str, JSONValue]
    prompt_template_id: str
    judge_model: str
    judge_provider: str
    judge_tokens: int
    eval_run_id: str | None
    dataset_item_id: str | None
    trace_id: str | None
    span_id: int | None
    span_start_time: datetime | None
    judged_input: str | None
    judged_output: str | None
    judged_context: tuple[Chunk, ...]
    judged_reference: str | None
    created_at: datetime

    def __post_init__(self) -> None:
        object.__setattr__(self, "details", _freeze_mapping(self.details))
        object.__setattr__(self, "judged_context", tuple(self.judged_context))


@dataclass(frozen=True, slots=True)
class SpanEvent:
    """Event attached to an OpenTelemetry span."""

    time: datetime
    name: str
    attributes: Mapping[str, JSONValue]
    dropped_attributes_count: int

    def __post_init__(self) -> None:
        object.__setattr__(self, "attributes", _freeze_mapping(self.attributes))


@dataclass(frozen=True, slots=True)
class Span:
    """Span in a recursively nested trace detail response."""

    id: int
    otel_span_id: str
    parent_span_id: str | None
    name: str
    kind: str
    operation_name: str
    start_time: datetime
    end_time: datetime
    duration_ms: int
    status_code: str
    status_message: str
    is_scorable: bool
    scorable_kind: str
    attributes: Mapping[str, JSONValue]
    events: tuple[SpanEvent, ...]
    input_tokens: int
    output_tokens: int
    reference_answer: str | None
    children: tuple[Span, ...]

    def __post_init__(self) -> None:
        object.__setattr__(self, "attributes", _freeze_mapping(self.attributes))
        object.__setattr__(self, "events", tuple(self.events))
        object.__setattr__(self, "children", tuple(self.children))


@dataclass(frozen=True, slots=True)
class ScoringTask:
    """Asynchronous trace-scoring task state."""

    id: str
    trace_id: str
    scorer: str
    status: str
    error: str | None


@dataclass(frozen=True, slots=True)
class Trace:
    """Project-scoped trace summary or complete detail tree."""

    id: str
    application_id: str
    otel_trace_id: str
    root_name: str
    start_time: datetime
    end_time: datetime
    status: str
    span_count: int
    total_tokens: int
    total_cost: str | None
    reference_answer: str | None
    attributes: Mapping[str, JSONValue]
    spans: tuple[Span, ...]
    scores: tuple[Score, ...]
    scoring_tasks: tuple[ScoringTask, ...]
    created_at: datetime
    updated_at: datetime

    def __post_init__(self) -> None:
        object.__setattr__(self, "attributes", _freeze_mapping(self.attributes))
        object.__setattr__(self, "spans", tuple(self.spans))
        object.__setattr__(self, "scores", tuple(self.scores))
        object.__setattr__(self, "scoring_tasks", tuple(self.scoring_tasks))
