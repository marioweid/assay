import copy
import json
import math
from collections.abc import Iterator
from typing import cast

import pytest
from hypothesis import given
from hypothesis import strategies as st
from typing_extensions import override

from assay.messages import Message, serialize_messages

JSON_VALUES = st.recursive(
    st.none()
    | st.booleans()
    | st.integers()
    | st.floats(allow_nan=False, allow_infinity=False)
    | st.text(),
    lambda children: (
        st.lists(children, max_size=3) | st.dictionaries(st.text(), children, max_size=3)
    ),
    max_leaves=10,
)


@given(text=st.text(), payload=JSON_VALUES)
def test_serialize_messages_round_trips_unicode_and_tool_values(
    text: str,
    payload: object,
) -> None:
    messages = [
        {
            "role": "developer",
            "parts": [
                {"type": "text", "content": text},
                {"type": "tool_call", "name": "lookup", "arguments": payload, "id": "call-1"},
                {"type": "tool_call_response", "response": payload, "id": "call-1"},
            ],
            "name": "agent",
        }
    ]
    original = copy.deepcopy(messages)

    encoded = serialize_messages(cast(list[Message], messages), 1_000_000)

    assert json.loads(encoded) == messages
    assert messages == original
    assert len(encoded.encode("utf-8")) <= 1_000_000


@pytest.mark.parametrize(
    "messages",
    [
        [{"role": "", "parts": []}],
        [{"role": "user", "parts": "text"}],
        [{"role": "user", "parts": [{"type": "unknown"}]}],
        [{"role": "user", "parts": [{"type": "text", "content": 1}]}],
        [{"role": "assistant", "parts": [{"type": "tool_call", "name": 1}]}],
        [{"role": "tool", "parts": [{"type": "tool_call_response"}]}],
        [{"role": "tool", "parts": [{"type": "tool_call_response", "response": math.nan}]}],
        [{"role": "tool", "parts": [{"type": "tool_call_response", "response": math.inf}]}],
        [{"role": "tool", "parts": [{"type": "tool_call_response", "response": {1: "x"}}]}],
    ],
)
def test_serialize_messages_rejects_invalid_shapes(messages: object) -> None:
    with pytest.raises(ValueError, match="structured messages are invalid"):
        serialize_messages(cast(list[Message], messages), 1_024)


def test_serialize_messages_accepts_empty_collection() -> None:
    assert serialize_messages([], 2) == "[]"


def test_serialize_messages_preserves_schema_nullable_fields() -> None:
    messages: list[Message] = [
        {
            "role": "tool",
            "name": None,
            "parts": [
                {"type": "tool_call", "name": "lookup", "id": None},
                {"type": "tool_call_response", "response": {}, "id": None},
            ],
        }
    ]

    assert json.loads(serialize_messages(messages, 1_024)) == messages


def test_serialize_messages_rejects_cycles_and_excessive_recursion() -> None:
    cycle: list[object] = []
    cycle.append(cycle)
    recursive: object = None
    for _ in range(101):
        recursive = [recursive]

    for value in (cycle, recursive):
        messages = [{"role": "tool", "parts": [{"type": "tool_call_response", "response": value}]}]
        with pytest.raises(ValueError, match="structured messages are invalid"):
            serialize_messages(cast(list[Message], messages), 10_000)


def test_serialize_messages_rejects_overflow_without_leaking_content() -> None:
    secret = "private-answer"
    messages: list[Message] = [
        {"role": "assistant", "parts": [{"type": "text", "content": secret}]}
    ]

    with pytest.raises(ValueError, match="capture byte limit") as failure:
        serialize_messages(messages, 2)

    assert "capture byte limit" in str(failure.value)
    assert secret not in str(failure.value)


def test_serialize_messages_redacts_collection_before_validation() -> None:
    supplied: list[Message] = [{"role": "user", "parts": [{"type": "text", "content": "secret"}]}]
    seen: list[object] = []

    def redact(value: object) -> object:
        seen.append(value)
        return [{"role": "user", "parts": [{"type": "text", "content": "[redacted]"}]}]

    encoded = serialize_messages(supplied, 1_024, redact=redact)

    assert seen == [supplied]
    assert "secret" not in encoded
    assert json.loads(encoded)[0]["parts"][0]["content"] == "[redacted]"


def test_serialize_messages_hides_collection_failures() -> None:
    secret = "private collection detail"

    class FailingMessage(dict[str, object]):
        @override
        def __iter__(self) -> Iterator[str]:
            raise RuntimeError(secret)

    messages = cast(list[Message], [FailingMessage(role="user", parts=[])])
    with pytest.raises(ValueError, match="structured messages are invalid") as failure:
        serialize_messages(messages, 1_024)

    assert secret not in repr(failure.value)


def test_serialize_messages_hides_redactor_failures() -> None:
    secret = "secret callback detail"

    def reject(_: object) -> object:
        raise RuntimeError(secret)

    with pytest.raises(ValueError, match="message redaction failed") as failure:
        serialize_messages([], 1_024, redact=reject)

    assert str(failure.value) == "message redaction failed"
    assert secret not in repr(failure.value)
