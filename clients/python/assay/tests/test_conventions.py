import json
from collections.abc import Iterable, Mapping
from typing import Literal, cast

import pytest

from assay.conventions import context_attributes, message_attribute, validate_chunks
from assay.models import Chunk


@pytest.mark.parametrize("role", ["user", "assistant"])
def test_message_attribute_emits_one_valid_genai_message(role: str) -> None:
    encoded = message_attribute(
        cast(Literal["user", "assistant"], role),
        "Assay evaluates AI systems.",
    )

    assert json.loads(encoded) == [{"role": role, "content": "Assay evaluates AI systems."}]
    assert encoded == f'[{{"role":"{role}","content":"Assay evaluates AI systems."}}]'


def test_validate_chunks_accepts_models_and_mappings() -> None:
    chunks = validate_chunks(
        [
            Chunk(id="k0", text="First"),
            {"id": "k1", "text": "Second", "score": 0.8, "source": "docs/2"},
        ]
    )

    assert chunks == (
        Chunk(id="k0", text="First"),
        Chunk(id="k1", text="Second", score=0.8, source="docs/2"),
    )


@pytest.mark.parametrize(
    ("chunks", "message"),
    [
        ([], "context must contain at least one chunk"),
        ([{"id": "", "text": "text"}], "chunk 0 id must not be blank"),
        ([{"id": "k0", "text": " "}], "chunk 0 text must not be blank"),
        (
            [{"id": "k0", "text": "one"}, {"id": "k0", "text": "two"}],
            "chunk id 'k0' is duplicated",
        ),
        ([{"id": "k0", "text": "text", "score": True}], "chunk 0 score must be a number"),
        ([{"id": "k0", "text": "text", "source": ""}], "chunk 0 source must not be blank"),
        ([{"id": "k0"}], "chunk 0 text must be a string"),
        (["chunk"], "chunk 0 must be a Chunk or mapping"),
    ],
)
def test_validate_chunks_rejects_invalid_complete_context(
    chunks: list[object],
    message: str,
) -> None:
    with pytest.raises(ValueError, match=message.replace("'", "\\'")):
        validate_chunks(cast(Iterable[Chunk | Mapping[str, object]], chunks))


def test_context_attributes_emit_standard_and_flattened_context() -> None:
    attributes = context_attributes(
        (
            Chunk(id="k0", text="First", score=0.9, source="docs/1"),
            Chunk(id="k1", text="Second"),
        ),
        max_bytes=1_024,
    )

    assert attributes["assay.context.chunk.count"] == 2
    assert attributes["assay.context.chunks.0.id"] == "k0"
    assert attributes["assay.context.chunks.0.text"] == "First"
    assert attributes["assay.context.chunks.0.score"] == 0.9
    assert attributes["assay.context.chunks.0.source"] == "docs/1"
    assert attributes["assay.context.chunks.1.id"] == "k1"
    assert attributes["assay.context.chunks.1.text"] == "Second"
    documents = json.loads(_string_attribute(attributes, "gen_ai.retrieval.documents"))
    assert documents == [
        {"id": "k0", "text": "First", "score": 0.9, "source": "docs/1"},
        {"id": "k1", "text": "Second"},
    ]


def test_context_attributes_cap_each_chunk_without_invalid_json() -> None:
    attributes = context_attributes(
        (Chunk(id="k0", text="é" * 100),),
        max_bytes=20,
    )

    text = _string_attribute(attributes, "assay.context.chunks.0.text")
    assert len(text.encode("utf-8")) <= 20
    assert (
        json.loads(_string_attribute(attributes, "gen_ai.retrieval.documents"))[0]["text"] == text
    )


def _string_attribute(attributes: Mapping[str, object], key: str) -> str:
    value = attributes[key]
    assert isinstance(value, str)
    return value
