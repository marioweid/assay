from dataclasses import dataclass
from typing import cast

import pytest
from hypothesis import given
from hypothesis import strategies as st
from typing_extensions import override

from assay._serialization import otlp_any_value, serialize_capture, truncate_utf8
from assay.exceptions import AssayConfigurationError
from assay.models import AttributeValue


@dataclass(frozen=True)
class Example:
    value: int


class BrokenRepresentation:
    @override
    def __repr__(self) -> str:
        raise RuntimeError("sensitive representation failure")


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("text", "text"),
        (True, "true"),
        (4, "4"),
        (Example(value=4), '{"value":4}'),
        ({"question": "What is Assay?"}, '{"question":"What is Assay?"}'),
        (BrokenRepresentation(), "<unrepresentable BrokenRepresentation>"),
    ],
)
def test_serialize_capture_produces_deterministic_text(value: object, expected: str) -> None:
    assert serialize_capture(value, 1_024) == expected


@pytest.mark.parametrize("max_bytes", [0, -1])
def test_capture_rejects_non_positive_byte_caps(max_bytes: int) -> None:
    with pytest.raises(AssayConfigurationError, match="max capture bytes must be positive"):
        serialize_capture("text", max_bytes)


def test_truncate_utf8_preserves_valid_characters_and_marker() -> None:
    value = "ééé-important"

    result = truncate_utf8(value, 15)

    assert result.endswith("...<truncated>")
    assert len(result.encode("utf-8")) <= 15


@given(value=st.text(), max_bytes=st.integers(min_value=1, max_value=256))
def test_truncate_utf8_never_exceeds_cap(value: str, max_bytes: int) -> None:
    result = truncate_utf8(value, max_bytes)

    assert len(result.encode("utf-8")) <= max_bytes


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("text", {"stringValue": "text"}),
        (True, {"boolValue": True}),
        (4, {"intValue": "4"}),
        (0.5, {"doubleValue": 0.5}),
        (
            ["a", "b"],
            {
                "arrayValue": {
                    "values": [{"stringValue": "a"}, {"stringValue": "b"}],
                }
            },
        ),
        ((), {"arrayValue": {"values": []}}),
    ],
)
def test_otlp_any_value(value: AttributeValue, expected: dict[str, object]) -> None:
    assert otlp_any_value(value) == expected


@given(values=st.lists(st.integers(), min_size=1))
def test_integer_arrays_use_decimal_string_values(values: list[int]) -> None:
    encoded = otlp_any_value(values)

    assert encoded == {"arrayValue": {"values": [{"intValue": str(value)} for value in values]}}


@pytest.mark.parametrize("value", [[1, True], ["text", 1], object(), None])
def test_otlp_any_value_rejects_unsupported_values(value: object) -> None:
    with pytest.raises(TypeError, match="unsupported OpenTelemetry attribute type"):
        otlp_any_value(cast(AttributeValue, value))
