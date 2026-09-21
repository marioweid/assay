"""Typed OpenTelemetry GenAI messages and bounded JSON serialization."""

from __future__ import annotations

import json
import math
from collections.abc import Callable, Mapping, Sequence
from typing import Literal, cast

from typing_extensions import TypedDict

_MAX_NESTING = 100


class TextPart(TypedDict):
    """One text message part."""

    type: Literal["text"]
    content: str


class _ToolCallRequired(TypedDict):
    type: Literal["tool_call"]
    name: str


class ToolCallPart(_ToolCallRequired, total=False):
    """One client-side tool request."""

    arguments: object
    id: str | None


class _ToolResultRequired(TypedDict):
    type: Literal["tool_call_response"]
    response: object


class ToolResultPart(_ToolResultRequired, total=False):
    """One client-side tool response."""

    id: str | None


MessagePart = TextPart | ToolCallPart | ToolResultPart


class _MessageRequired(TypedDict):
    role: str
    parts: Sequence[MessagePart]


class Message(_MessageRequired, total=False):
    """One structured GenAI message with ordered parts."""

    name: str | None


def serialize_messages(
    messages: Sequence[Message],
    max_bytes: int,
    *,
    redact: Callable[[object], object] | None = None,
) -> str:
    """Validate and encode structured messages without truncating JSON.

    Args:
        messages: Complete ordered message collection.
        max_bytes: Maximum encoded UTF-8 size, including the JSON envelope.
        redact: Optional callback applied to the complete collection before validation.

    Returns:
        Compact UTF-8 JSON matching the pinned OpenTelemetry GenAI schema.

    Raises:
        ValueError: If redaction fails, the shape is invalid, or the result exceeds the limit.
    """
    if isinstance(max_bytes, bool) or not isinstance(max_bytes, int) or max_bytes <= 0:
        raise ValueError("max capture bytes must be positive")
    candidate = _redact(messages, redact)
    try:
        normalized = _normalize_messages(candidate, set(), 0)
        encoded = json.dumps(
            normalized,
            ensure_ascii=False,
            separators=(",", ":"),
            allow_nan=False,
        )
        size = len(encoded.encode("utf-8"))
    except Exception:
        raise ValueError("structured messages are invalid") from None
    if size > max_bytes:
        raise ValueError("structured messages exceed the capture byte limit")
    return encoded


def _redact(
    messages: Sequence[Message],
    redact: Callable[[object], object] | None,
) -> object:
    if redact is None:
        return messages
    try:
        return redact(messages)
    except Exception:
        raise ValueError("message redaction failed") from None


def _normalize_messages(value: object, seen: set[int], depth: int) -> list[object]:
    values = _sequence(value, seen, depth)
    try:
        return [_normalize_message(item, seen, depth + 1) for item in values]
    finally:
        seen.remove(id(value))


def _normalize_message(value: object, seen: set[int], depth: int) -> dict[str, object]:
    fields = _mapping(value, seen, depth)
    try:
        if set(fields) - {"role", "parts", "name"}:
            raise ValueError
        role = fields.get("role")
        if not isinstance(role, str) or not role.strip():
            raise ValueError
        result: dict[str, object] = {
            "role": role,
            "parts": _normalize_parts(fields.get("parts"), seen, depth + 1),
        }
        if "name" in fields:
            name = fields["name"]
            if name is not None and not isinstance(name, str):
                raise ValueError
            result["name"] = name
        return result
    finally:
        seen.remove(id(value))


def _normalize_parts(value: object, seen: set[int], depth: int) -> list[object]:
    parts = _sequence(value, seen, depth)
    try:
        return [_normalize_part(part, seen, depth + 1) for part in parts]
    finally:
        seen.remove(id(value))


def _normalize_part(value: object, seen: set[int], depth: int) -> dict[str, object]:
    fields = _mapping(value, seen, depth)
    try:
        part_type = fields.get("type")
        if part_type == "text":
            return _text_part(fields)
        if part_type == "tool_call":
            return _tool_call_part(fields, seen, depth)
        if part_type == "tool_call_response":
            return _tool_result_part(fields, seen, depth)
        raise ValueError
    finally:
        seen.remove(id(value))


def _text_part(fields: Mapping[object, object]) -> dict[str, object]:
    if set(fields) != {"type", "content"} or not isinstance(fields["content"], str):
        raise ValueError
    return {"type": "text", "content": fields["content"]}


def _tool_call_part(
    fields: Mapping[object, object],
    seen: set[int],
    depth: int,
) -> dict[str, object]:
    if set(fields) - {"type", "name", "arguments", "id"}:
        raise ValueError
    name = fields.get("name")
    if not isinstance(name, str):
        raise ValueError
    result: dict[str, object] = {"type": "tool_call", "name": name}
    if "arguments" in fields:
        result["arguments"] = _normalize_json(fields["arguments"], seen, depth + 1)
    _optional_id(fields, result)
    return result


def _tool_result_part(
    fields: Mapping[object, object],
    seen: set[int],
    depth: int,
) -> dict[str, object]:
    if set(fields) - {"type", "response", "id"} or "response" not in fields:
        raise ValueError
    result: dict[str, object] = {
        "type": "tool_call_response",
        "response": _normalize_json(fields["response"], seen, depth + 1),
    }
    _optional_id(fields, result)
    return result


def _optional_id(fields: Mapping[object, object], result: dict[str, object]) -> None:
    if "id" not in fields:
        return
    value = fields["id"]
    if value is not None and not isinstance(value, str):
        raise ValueError
    result["id"] = value


def _normalize_json(value: object, seen: set[int], depth: int) -> object:
    _check_depth(depth)
    if value is None or isinstance(value, bool | int | str):
        return value
    if isinstance(value, float):
        if not math.isfinite(value):
            raise ValueError
        return value
    if isinstance(value, list):
        _enter(value, seen)
        try:
            return [_normalize_json(item, seen, depth + 1) for item in value]
        finally:
            seen.remove(id(value))
    if isinstance(value, Mapping):
        fields = _mapping(value, seen, depth)
        try:
            if any(not isinstance(key, str) for key in fields):
                raise ValueError
            return {key: _normalize_json(item, seen, depth + 1) for key, item in fields.items()}
        finally:
            seen.remove(id(value))
    raise ValueError


def _sequence(value: object, seen: set[int], depth: int) -> Sequence[object]:
    _check_depth(depth)
    if not isinstance(value, Sequence) or isinstance(value, str | bytes | bytearray | memoryview):
        raise ValueError
    _enter(value, seen)
    return value


def _mapping(value: object, seen: set[int], depth: int) -> Mapping[object, object]:
    _check_depth(depth)
    if not isinstance(value, Mapping):
        raise ValueError
    _enter(value, seen)
    return cast(Mapping[object, object], value)


def _enter(value: object, seen: set[int]) -> None:
    identity = id(value)
    if identity in seen:
        raise ValueError
    seen.add(identity)


def _check_depth(depth: int) -> None:
    if depth > _MAX_NESTING:
        raise ValueError
