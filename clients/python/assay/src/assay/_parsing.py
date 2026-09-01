"""Strict, secret-safe parsing for Assay API response models."""

from __future__ import annotations

from collections.abc import Callable, Mapping
from contextlib import suppress
from datetime import datetime
from typing import NoReturn, TypeVar, cast

from assay.exceptions import AssayProtocolError
from assay.models import (
    APIKey,
    Application,
    Chunk,
    CreatedAPIKey,
    Dataset,
    DatasetItem,
    EvalRun,
    EvalRunItem,
    JSONValue,
    JudgeConfigView,
    Page,
    Project,
    ResponseMappingView,
    Score,
    ScoreAggregate,
    ScorerConfig,
    ScoringTask,
    Span,
    SpanEvent,
    TargetEndpointView,
    Trace,
)

T = TypeVar("T")


def parse_project(operation: str, payload: Mapping[str, object]) -> Project:
    judge_payload = _optional_object(operation, payload, "judge_config")
    judge = parse_judge(operation, judge_payload) if judge_payload is not None else None
    return Project(
        id=_string(operation, payload, "id"),
        name=_string(operation, payload, "name"),
        judge_config=judge,
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
    )


def parse_api_key(operation: str, payload: Mapping[str, object]) -> APIKey:
    return APIKey(
        id=_string(operation, payload, "id"),
        project_id=_string(operation, payload, "project_id"),
        name=_string(operation, payload, "name"),
        key_prefix=_string(operation, payload, "key_prefix"),
        last_used_at=_optional_timestamp(operation, payload, "last_used_at"),
        revoked_at=_optional_timestamp(operation, payload, "revoked_at"),
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
    )


def parse_created_api_key(
    operation: str,
    payload: Mapping[str, object],
) -> CreatedAPIKey:
    metadata = parse_api_key(operation, payload)
    return CreatedAPIKey(
        id=metadata.id,
        project_id=metadata.project_id,
        name=metadata.name,
        key_prefix=metadata.key_prefix,
        last_used_at=metadata.last_used_at,
        revoked_at=metadata.revoked_at,
        created_at=metadata.created_at,
        updated_at=metadata.updated_at,
        key=_string(operation, payload, "key"),
    )


def parse_application(operation: str, payload: Mapping[str, object]) -> Application:
    endpoint_payload = _optional_object(operation, payload, "target_endpoint")
    endpoint = (
        parse_target_endpoint(operation, endpoint_payload) if endpoint_payload is not None else None
    )
    return Application(
        id=_string(operation, payload, "id"),
        project_id=_string(operation, payload, "project_id"),
        name=_string(operation, payload, "name"),
        slug=_string(operation, payload, "slug"),
        config=_json_object(operation, payload, "config"),
        auto_score_scorers=_string_tuple(operation, payload, "auto_score_scorers"),
        target_endpoint=endpoint,
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
    )


def parse_collection(
    operation: str,
    payload: Mapping[str, object],
    parser: Callable[[str, Mapping[str, object]], T],
) -> tuple[T, ...]:
    raw_items = payload.get("items")
    if not isinstance(raw_items, list):
        _invalid(operation, "items")
    items: list[T] = []
    for item in raw_items:
        if not isinstance(item, dict):
            _invalid(operation, "items")
        items.append(parser(operation, cast(dict[str, object], item)))
    return tuple(items)


def parse_judge(
    operation: str,
    payload: Mapping[str, object],
) -> JudgeConfigView:
    return JudgeConfigView(
        base_url=_string(operation, payload, "base_url"),
        model=_string(operation, payload, "model"),
        has_api_key=_bool(operation, payload, "has_api_key"),
    )


def parse_target_endpoint(
    operation: str,
    payload: Mapping[str, object],
) -> TargetEndpointView:
    response_mapping = _object(operation, payload, "response_mapping")
    return TargetEndpointView(
        url=_string(operation, payload, "url"),
        method=_string(operation, payload, "method"),
        headers=_string_mapping(operation, payload, "headers", default_empty=True),
        request_template=_json_object(operation, payload, "request_template", default_empty=True),
        response_mapping=ResponseMappingView(
            output=_string(operation, response_mapping, "output"),
            context=_optional_string(operation, response_mapping, "context"),
        ),
        timeout_ms=_integer(operation, payload, "timeout_ms"),
        has_secret=_bool(operation, payload, "has_secret"),
    )


def parse_dataset(operation: str, payload: Mapping[str, object]) -> Dataset:
    return Dataset(
        id=_string(operation, payload, "id"),
        application_id=_string(operation, payload, "application_id"),
        name=_string(operation, payload, "name"),
        description=_optional_string(operation, payload, "description"),
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
    )


def parse_dataset_item(operation: str, payload: Mapping[str, object]) -> DatasetItem:
    return DatasetItem(
        id=_string(operation, payload, "id"),
        dataset_id=_string(operation, payload, "dataset_id"),
        external_id=_optional_string(operation, payload, "external_id"),
        input=_json_object(operation, payload, "input"),
        output=_optional_string(operation, payload, "output"),
        expected_output=_optional_string(operation, payload, "expected_output"),
        context=_chunks(operation, payload, "context"),
        metadata=_json_object(operation, payload, "metadata"),
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
    )


def parse_scorer_config(operation: str, payload: Mapping[str, object]) -> ScorerConfig:
    judge_payload = _optional_object(operation, payload, "judge_config")
    return ScorerConfig(
        id=_optional_string(operation, payload, "id"),
        application_id=_string(operation, payload, "application_id"),
        scorer=_string(operation, payload, "scorer"),
        enabled=_bool(operation, payload, "enabled"),
        threshold=_number(operation, payload, "threshold"),
        judge_config=parse_judge(operation, judge_payload) if judge_payload else None,
        prompt_template_id=_string(operation, payload, "prompt_template_id"),
        persisted=_bool(operation, payload, "persisted"),
    )


def parse_eval_run(operation: str, payload: Mapping[str, object]) -> EvalRun:
    return EvalRun(
        id=_string(operation, payload, "id"),
        application_id=_string(operation, payload, "application_id"),
        dataset_id=_string(operation, payload, "dataset_id"),
        name=_string(operation, payload, "name"),
        status=_string(operation, payload, "status"),
        mode=_string(operation, payload, "mode"),
        params=_json_object(operation, payload, "params"),
        scorers=_string_tuple(operation, payload, "scorers"),
        aggregates=_aggregates(operation, payload),
        total_items=_integer(operation, payload, "total_items"),
        succeeded_items=_integer(operation, payload, "succeeded_items"),
        failed_items=_integer(operation, payload, "failed_items"),
        canceled_items=_integer(operation, payload, "canceled_items"),
        started_at=_optional_timestamp(operation, payload, "started_at"),
        finished_at=_optional_timestamp(operation, payload, "finished_at"),
        error=_optional_string(operation, payload, "error"),
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
    )


def parse_eval_run_item(operation: str, payload: Mapping[str, object]) -> EvalRunItem:
    return EvalRunItem(
        eval_run_id=_string(operation, payload, "eval_run_id"),
        dataset_item_id=_string(operation, payload, "dataset_item_id"),
        status=_string(operation, payload, "status"),
        error=_optional_string(operation, payload, "error"),
        started_at=_optional_timestamp(operation, payload, "started_at"),
        finished_at=_optional_timestamp(operation, payload, "finished_at"),
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
        generated_output=_optional_string(operation, payload, "generated_output"),
        generated_context=_chunks(operation, payload, "generated_context", default_empty=True),
        generated_at=_optional_timestamp(operation, payload, "generated_at"),
    )


def parse_score(operation: str, payload: Mapping[str, object]) -> Score:
    return Score(
        id=_integer(operation, payload, "id"),
        scorer=_string(operation, payload, "scorer"),
        scorer_config_id=_optional_string(operation, payload, "scorer_config_id"),
        value=_number(operation, payload, "value"),
        threshold=_number(operation, payload, "threshold"),
        passed=_bool(operation, payload, "passed"),
        rationale=_string(operation, payload, "rationale"),
        details=_json_object(operation, payload, "details"),
        prompt_template_id=_string(operation, payload, "prompt_template_id"),
        judge_model=_string(operation, payload, "judge_model"),
        judge_provider=_string(operation, payload, "judge_provider"),
        judge_tokens=_integer(operation, payload, "judge_tokens"),
        eval_run_id=_optional_string(operation, payload, "eval_run_id"),
        dataset_item_id=_optional_string(operation, payload, "dataset_item_id"),
        trace_id=_optional_string(operation, payload, "trace_id"),
        span_id=_optional_integer(operation, payload, "span_id"),
        span_start_time=_optional_timestamp(operation, payload, "span_start_time"),
        judged_input=_optional_string(operation, payload, "judged_input"),
        judged_output=_optional_string(operation, payload, "judged_output"),
        judged_context=_chunks(operation, payload, "judged_context", default_empty=True),
        judged_reference=_optional_string(operation, payload, "judged_reference"),
        created_at=_timestamp(operation, payload, "created_at"),
    )


def parse_scoring_task(operation: str, payload: Mapping[str, object]) -> ScoringTask:
    return ScoringTask(
        id=_string(operation, payload, "id"),
        trace_id=_string(operation, payload, "trace_id"),
        scorer=_string(operation, payload, "scorer"),
        status=_string(operation, payload, "status"),
        error=_optional_string(operation, payload, "error"),
    )


def parse_trace(operation: str, payload: Mapping[str, object]) -> Trace:
    return Trace(
        id=_string(operation, payload, "id"),
        application_id=_string(operation, payload, "application_id"),
        otel_trace_id=_string(operation, payload, "otel_trace_id"),
        root_name=_string(operation, payload, "root_name"),
        start_time=_timestamp(operation, payload, "start_time"),
        end_time=_timestamp(operation, payload, "end_time"),
        status=_string(operation, payload, "status"),
        span_count=_integer(operation, payload, "span_count"),
        total_tokens=_integer(operation, payload, "total_tokens"),
        total_cost=_optional_string(operation, payload, "total_cost"),
        reference_answer=_optional_string(operation, payload, "reference_answer"),
        attributes=_json_object(operation, payload, "attributes"),
        spans=tuple(
            _parse_span(operation, item, depth=0)
            for item in _objects(operation, payload, "spans", default_empty=True)
        ),
        scores=tuple(
            parse_score(operation, item)
            for item in _objects(operation, payload, "scores", default_empty=True)
        ),
        scoring_tasks=tuple(
            parse_scoring_task(operation, item)
            for item in _objects(operation, payload, "scoring_tasks", default_empty=True)
        ),
        created_at=_timestamp(operation, payload, "created_at"),
        updated_at=_timestamp(operation, payload, "updated_at"),
    )


def parse_page(
    operation: str,
    payload: Mapping[str, object],
    parser: Callable[[str, Mapping[str, object]], T],
) -> Page[T]:
    cursor = _optional_string(operation, payload, "next_cursor")
    return Page(items=parse_collection(operation, payload, parser), next_cursor=cursor)


def _parse_span(operation: str, payload: Mapping[str, object], *, depth: int) -> Span:
    if depth >= 256:
        _invalid(operation, "children")
    return Span(
        id=_integer(operation, payload, "id"),
        otel_span_id=_string(operation, payload, "otel_span_id"),
        parent_span_id=_optional_string(operation, payload, "parent_span_id"),
        name=_string(operation, payload, "name"),
        kind=_string(operation, payload, "kind"),
        operation_name=_string_default(operation, payload, "operation_name"),
        start_time=_timestamp(operation, payload, "start_time"),
        end_time=_timestamp(operation, payload, "end_time"),
        duration_ms=_integer(operation, payload, "duration_ms"),
        status_code=_string(operation, payload, "status_code"),
        status_message=_string_default(operation, payload, "status_message"),
        is_scorable=_bool(operation, payload, "is_scorable"),
        scorable_kind=_string_default(operation, payload, "scorable_kind"),
        attributes=_json_object(operation, payload, "attributes"),
        events=tuple(
            _parse_event(operation, item) for item in _objects(operation, payload, "events")
        ),
        input_tokens=_integer(operation, payload, "input_tokens"),
        output_tokens=_integer(operation, payload, "output_tokens"),
        reference_answer=_optional_string(operation, payload, "reference_answer"),
        children=tuple(
            _parse_span(operation, item, depth=depth + 1)
            for item in _objects(operation, payload, "children")
        ),
    )


def _parse_event(operation: str, payload: Mapping[str, object]) -> SpanEvent:
    return SpanEvent(
        time=_timestamp(operation, payload, "time"),
        name=_string(operation, payload, "name"),
        attributes=_json_object(operation, payload, "attributes"),
        dropped_attributes_count=_integer(operation, payload, "dropped_attributes_count"),
    )


def _string(operation: str, payload: Mapping[str, object], name: str) -> str:
    value = payload.get(name)
    if not isinstance(value, str):
        _invalid(operation, name)
    return value


def _optional_string(
    operation: str,
    payload: Mapping[str, object],
    name: str,
) -> str | None:
    value = payload.get(name)
    if value is None:
        return None
    if not isinstance(value, str):
        _invalid(operation, name)
    return value


def _bool(operation: str, payload: Mapping[str, object], name: str) -> bool:
    value = payload.get(name)
    if not isinstance(value, bool):
        _invalid(operation, name)
    return value


def _integer(operation: str, payload: Mapping[str, object], name: str) -> int:
    value = payload.get(name)
    if isinstance(value, bool) or not isinstance(value, int):
        _invalid(operation, name)
    return value


def _optional_integer(
    operation: str,
    payload: Mapping[str, object],
    name: str,
) -> int | None:
    if payload.get(name) is None:
        return None
    return _integer(operation, payload, name)


def _number(operation: str, payload: Mapping[str, object], name: str) -> float:
    value = payload.get(name)
    if isinstance(value, bool) or not isinstance(value, int | float):
        _invalid(operation, name)
    return float(value)


def _timestamp(operation: str, payload: Mapping[str, object], name: str) -> datetime:
    value = _string(operation, payload, name)
    parsed: datetime | None = None
    with suppress(ValueError):
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed is None or parsed.tzinfo is None:
        _invalid(operation, name)
    return parsed


def _optional_timestamp(
    operation: str,
    payload: Mapping[str, object],
    name: str,
) -> datetime | None:
    if payload.get(name) is None:
        return None
    return _timestamp(operation, payload, name)


def _object(
    operation: str,
    payload: Mapping[str, object],
    name: str,
) -> Mapping[str, object]:
    value = payload.get(name)
    if not isinstance(value, dict):
        _invalid(operation, name)
    return cast(dict[str, object], value)


def _optional_object(
    operation: str,
    payload: Mapping[str, object],
    name: str,
) -> Mapping[str, object] | None:
    if payload.get(name) is None:
        return None
    return _object(operation, payload, name)


def _json_object(
    operation: str,
    payload: Mapping[str, object],
    name: str,
    *,
    default_empty: bool = False,
) -> Mapping[str, JSONValue]:
    if default_empty and payload.get(name) is None:
        return {}
    return cast(Mapping[str, JSONValue], _object(operation, payload, name))


def _string_mapping(
    operation: str,
    payload: Mapping[str, object],
    name: str,
    *,
    default_empty: bool = False,
) -> Mapping[str, str]:
    if default_empty and payload.get(name) is None:
        return dict[str, str]()
    values = _object(operation, payload, name)
    if any(not isinstance(value, str) for value in values.values()):
        _invalid(operation, name)
    return cast(Mapping[str, str], values)


def _string_tuple(
    operation: str,
    payload: Mapping[str, object],
    name: str,
) -> tuple[str, ...]:
    value = payload.get(name)
    if not isinstance(value, list) or any(not isinstance(item, str) for item in value):
        _invalid(operation, name)
    return cast(tuple[str, ...], tuple(value))


def _string_default(operation: str, payload: Mapping[str, object], name: str) -> str:
    if name not in payload:
        return ""
    return _string(operation, payload, name)


def _objects(
    operation: str,
    payload: Mapping[str, object],
    name: str,
    default_empty: bool = False,
) -> tuple[Mapping[str, object], ...]:
    value = payload.get(name)
    if value is None and default_empty:
        return ()
    if not isinstance(value, list) or any(not isinstance(item, dict) for item in value):
        _invalid(operation, name)
    return cast(tuple[Mapping[str, object], ...], tuple(value))


def _chunks(
    operation: str,
    payload: Mapping[str, object],
    name: str,
    default_empty: bool = False,
) -> tuple[Chunk, ...]:
    chunks: list[Chunk] = []
    for item in _objects(operation, payload, name, default_empty):
        chunks.append(
            Chunk(
                id=_string(operation, item, "id"),
                text=_string(operation, item, "text"),
            )
        )
    return tuple(chunks)


def _aggregates(
    operation: str,
    payload: Mapping[str, object],
) -> Mapping[str, ScoreAggregate]:
    values = _object(operation, payload, "aggregates")
    aggregates: dict[str, ScoreAggregate] = {}
    for scorer, value in values.items():
        if not isinstance(value, dict):
            _invalid(operation, "aggregates")
        aggregate = cast(dict[str, object], value)
        aggregates[scorer] = ScoreAggregate(
            mean=_number(operation, aggregate, "mean"),
            pass_rate=_number(operation, aggregate, "pass_rate"),
            n=_integer(operation, aggregate, "n"),
        )
    return aggregates


def _invalid(operation: str, name: str) -> NoReturn:
    raise AssayProtocolError(f"{operation}: invalid response field '{name}'")
