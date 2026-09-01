"""Private capture and OTLP value serialization."""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from dataclasses import asdict, is_dataclass
from typing import cast

from assay.exceptions import AssayConfigurationError
from assay.models import AttributeValue

_TRUNCATION_MARKER = "...<truncated>"


def truncate_utf8(value: str, max_bytes: int) -> str:
    """Return valid UTF-8 text whose encoded size does not exceed the byte cap."""
    if max_bytes <= 0:
        raise AssayConfigurationError("max capture bytes must be positive")
    encoded = value.encode("utf-8")
    if len(encoded) <= max_bytes:
        return value

    marker = _TRUNCATION_MARKER.encode("utf-8")
    if max_bytes <= len(marker):
        return marker[:max_bytes].decode("utf-8")

    prefix = encoded[: max_bytes - len(marker)]
    while prefix:
        try:
            return prefix.decode("utf-8") + _TRUNCATION_MARKER
        except UnicodeDecodeError:
            prefix = prefix[:-1]
    return _TRUNCATION_MARKER


def serialize_capture(value: object, max_bytes: int) -> str:
    """Convert a captured value to deterministic, size-capped text."""
    if max_bytes <= 0:
        raise AssayConfigurationError("max capture bytes must be positive")
    if isinstance(value, str):
        return truncate_utf8(value, max_bytes)

    value = _capture_value(value)
    if isinstance(value, str):
        text = value
    else:
        text = json.dumps(
            value,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
    return truncate_utf8(text, max_bytes)


def _capture_value(value: object) -> object:
    if value is None or isinstance(value, bool | int | float | str):
        return value
    if is_dataclass(value) and not isinstance(value, type):
        return _capture_value(asdict(value))
    if isinstance(value, Mapping):
        return {str(key): _capture_value(item) for key, item in value.items()}
    if isinstance(value, Sequence) and not isinstance(value, bytes | bytearray | memoryview):
        return [_capture_value(item) for item in value]
    try:
        return repr(value)
    except Exception:
        return f"<unrepresentable {type(value).__name__}>"


def otlp_any_value(value: AttributeValue) -> dict[str, object]:
    """Encode one validated OpenTelemetry attribute as protobuf JSON AnyValue."""
    if isinstance(value, str):
        return {"stringValue": value}
    if isinstance(value, bool):
        return {"boolValue": value}
    if isinstance(value, int):
        return {"intValue": str(value)}
    if isinstance(value, float):
        return {"doubleValue": value}
    if isinstance(value, Sequence) and not isinstance(value, bytes | bytearray | memoryview):
        return _otlp_array_value(value)
    raise _unsupported_type(value)


def _otlp_array_value(value: Sequence[object]) -> dict[str, object]:
    if not value:
        return {"arrayValue": {"values": []}}
    expected_type = type(value[0])
    if expected_type not in {str, bool, int, float}:
        raise _unsupported_type(value)
    if any(type(item) is not expected_type for item in value):
        raise _unsupported_type(value)
    encoded = [otlp_any_value(cast(AttributeValue, item)) for item in value]
    return {"arrayValue": {"values": encoded}}


def _unsupported_type(value: object) -> TypeError:
    return TypeError(f"unsupported OpenTelemetry attribute type: {type(value).__name__}")
