"""OpenTelemetry GenAI and Assay semantic-convention helpers."""

from __future__ import annotations

import json
import math
from collections.abc import Iterable, Mapping, Sequence
from typing import Literal

from assay._serialization import truncate_utf8
from assay.models import AttributeValue, Chunk

GEN_AI_INPUT_MESSAGES = "gen_ai.input.messages"
GEN_AI_OUTPUT_MESSAGES = "gen_ai.output.messages"
GEN_AI_RETRIEVAL_DOCUMENTS = "gen_ai.retrieval.documents"
ASSAY_APPLICATION_SLUG = "assay.application.slug"
ASSAY_SCORABLE = "assay.scorable"
ASSAY_SCORABLE_KIND = "assay.scorable.kind"
ASSAY_UNIT_ID = "assay.unit.id"
ASSAY_DATASET_ID = "assay.dataset.id"
ASSAY_RUN_ID = "assay.run.id"
ASSAY_REFERENCE_ANSWER = "assay.reference.answer"
ASSAY_REFERENCE_ID = "assay.reference.id"
ASSAY_REFERENCE_SOURCE = "assay.reference.source"
ASSAY_CAPTURE_ERROR = "assay.capture.error"
ASSAY_CONTEXT_CHUNK_COUNT = "assay.context.chunk.count"


def message_attribute(role: Literal["user", "assistant"], content: str) -> str:
    """Encode one captured message using the M4 scoring content contract."""
    if role not in {"user", "assistant"}:
        raise ValueError("message role must be user or assistant")
    return json.dumps(
        [{"role": role, "content": content}],
        ensure_ascii=False,
        separators=(",", ":"),
    )


def validate_chunks(
    chunks: Iterable[Chunk | Mapping[str, object]],
) -> tuple[Chunk, ...]:
    """Validate and freeze a complete context collection."""
    validated = tuple(_validate_chunk(index, chunk) for index, chunk in enumerate(chunks))
    if not validated:
        raise ValueError("context must contain at least one chunk")
    seen: set[str] = set()
    for chunk in validated:
        if chunk.id in seen:
            raise ValueError(f"chunk id {chunk.id!r} is duplicated")
        seen.add(chunk.id)
    return validated


def context_attributes(
    chunks: Sequence[Chunk],
    max_bytes: int,
) -> dict[str, AttributeValue]:
    """Build standard retrieval JSON plus complete flattened Assay chunk fields."""
    validated = validate_chunks(chunks)
    attributes: dict[str, AttributeValue] = {ASSAY_CONTEXT_CHUNK_COUNT: len(validated)}
    documents: list[dict[str, str | float]] = []
    for index, chunk in enumerate(validated):
        text = truncate_utf8(chunk.text, max_bytes)
        prefix = f"assay.context.chunks.{index}"
        attributes[f"{prefix}.id"] = chunk.id
        attributes[f"{prefix}.text"] = text
        document: dict[str, str | float] = {"id": chunk.id, "text": text}
        if chunk.score is not None:
            attributes[f"{prefix}.score"] = chunk.score
            document["score"] = chunk.score
        if chunk.source is not None:
            attributes[f"{prefix}.source"] = chunk.source
            document["source"] = chunk.source
        documents.append(document)
    attributes[GEN_AI_RETRIEVAL_DOCUMENTS] = json.dumps(
        documents,
        ensure_ascii=False,
        separators=(",", ":"),
    )
    return attributes


def _validate_chunk(index: int, chunk: Chunk | Mapping[str, object]) -> Chunk:
    values = _chunk_values(index, chunk)
    chunk_id = _required_chunk_string(index, "id", values.get("id"))
    text = _required_chunk_string(index, "text", values.get("text"), trim=False)
    score = _optional_chunk_score(index, values.get("score"))
    source = _optional_chunk_source(index, values.get("source"))
    return Chunk(id=chunk_id, text=text, score=score, source=source)


def _chunk_values(
    index: int,
    chunk: Chunk | Mapping[str, object],
) -> Mapping[str, object]:
    if isinstance(chunk, Chunk):
        return {
            "id": chunk.id,
            "text": chunk.text,
            "score": chunk.score,
            "source": chunk.source,
        }
    if isinstance(chunk, Mapping):
        return chunk
    raise ValueError(f"chunk {index} must be a Chunk or mapping")


def _required_chunk_string(
    index: int,
    field: str,
    value: object,
    *,
    trim: bool = True,
) -> str:
    if not isinstance(value, str):
        raise ValueError(f"chunk {index} {field} must be a string")
    if not value.strip():
        raise ValueError(f"chunk {index} {field} must not be blank")
    return value.strip() if trim else value


def _optional_chunk_score(index: int, score: object) -> float | None:
    if score is None:
        return None
    if isinstance(score, bool) or not isinstance(score, int | float):
        raise ValueError(f"chunk {index} score must be a number")
    if not math.isfinite(score):
        raise ValueError(f"chunk {index} score must be a number")
    return float(score)


def _optional_chunk_source(index: int, source: object) -> str | None:
    if source is None:
        return None
    if not isinstance(source, str):
        raise ValueError(f"chunk {index} source must be a string")
    if not source.strip():
        raise ValueError(f"chunk {index} source must not be blank")
    return source.strip()
