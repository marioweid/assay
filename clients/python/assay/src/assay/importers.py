"""Content-safe JSONL and CSV dataset parsers."""

from __future__ import annotations

import csv
import io
import json
from collections.abc import Mapping, Sequence
from contextlib import suppress
from pathlib import Path
from typing import NoReturn, cast

from assay.exceptions import AssayConfigurationError
from assay.models import Chunk, DatasetItemInput, JSONValue

_INVALID = object()


def parse_dataset_file(path: Path) -> tuple[DatasetItemInput, ...]:
    """Parse a supported dataset file after validating every record."""
    suffix = path.suffix.lower()
    if suffix == ".jsonl":
        return parse_jsonl(path)
    if suffix == ".csv":
        return parse_csv(path)
    raise AssayConfigurationError(f"{path}: unsupported dataset file extension")


def parse_jsonl(path: Path) -> tuple[DatasetItemInput, ...]:
    """Parse newline-delimited server-shaped dataset item objects."""
    text = _read_text(path)
    items: list[DatasetItemInput] = []
    for line_number, line in enumerate(text.splitlines(), start=1):
        if not line.strip():
            continue
        payload = _decode_json(line)
        if not isinstance(payload, dict):
            _invalid(path, line_number, "object")
        items.append(_parse_item(path, line_number, cast(dict[str, object], payload)))
    return tuple(items)


def parse_csv(path: Path) -> tuple[DatasetItemInput, ...]:
    """Parse a CSV dataset with either an input or question column."""
    text = _read_text(path)
    reader = csv.DictReader(io.StringIO(text, newline=""), strict=True)
    headers = reader.fieldnames
    if headers is None:
        return ()
    _validate_headers(path, headers)
    rows: list[tuple[int, dict[str, str | None]]] = []
    failed = False
    try:
        for row in reader:
            rows.append((reader.line_num, row))
    except csv.Error:
        failed = True
    if failed:
        _invalid(path, reader.line_num, "CSV structure")
    items: list[DatasetItemInput] = []
    for line_number, row in rows:
        if None in row:
            _invalid(path, line_number, "columns")
        items.append(_parse_csv_row(path, line_number, row))
    return tuple(items)


def _parse_csv_row(
    path: Path,
    line_number: int,
    row: Mapping[str, str | None],
) -> DatasetItemInput:
    if "question" in row:
        question = _required_cell(path, line_number, "question", row.get("question"))
        input_value: object = {"question": question}
    else:
        input_value = _decode_cell(path, line_number, "input", row.get("input"))
    payload: dict[str, object] = {"input": input_value}
    for name in ("external_id", "output", "expected_output"):
        value = row.get(name)
        if value not in {None, ""}:
            payload[name] = value
    context = row.get("context")
    if context not in {None, ""}:
        payload["context"] = _decode_cell(path, line_number, "context", context)
    metadata = row.get("metadata")
    if metadata not in {None, ""}:
        payload["metadata"] = _decode_cell(path, line_number, "metadata", metadata)
    return _parse_item(path, line_number, payload)


def _parse_item(
    path: Path,
    line_number: int,
    payload: Mapping[str, object],
) -> DatasetItemInput:
    input_value = payload.get("input")
    if not isinstance(input_value, dict):
        _invalid(path, line_number, "input")
    context = _parse_chunks(path, line_number, payload.get("context", []))
    metadata = payload.get("metadata", {})
    if not isinstance(metadata, dict):
        _invalid(path, line_number, "metadata")
    return DatasetItemInput(
        input=cast(Mapping[str, JSONValue], input_value),
        external_id=_optional_text(path, line_number, payload, "external_id"),
        output=_optional_text(path, line_number, payload, "output", allow_blank=True),
        expected_output=_optional_text(
            path, line_number, payload, "expected_output", allow_blank=True
        ),
        context=context,
        metadata=cast(Mapping[str, JSONValue], metadata),
    )


def _parse_chunks(path: Path, line_number: int, value: object) -> tuple[Chunk, ...]:
    if not isinstance(value, list):
        _invalid(path, line_number, "context")
    chunks: list[Chunk] = []
    for chunk in value:
        if not isinstance(chunk, dict):
            _invalid(path, line_number, "context")
        chunk_id = chunk.get("id")
        text = chunk.get("text")
        if not isinstance(chunk_id, str) or not chunk_id.strip():
            _invalid(path, line_number, "context.id")
        if not isinstance(text, str) or not text.strip():
            _invalid(path, line_number, "context.text")
        chunks.append(Chunk(id=chunk_id.strip(), text=text))
    return tuple(chunks)


def _validate_headers(path: Path, headers: Sequence[str]) -> None:
    if len(headers) != len(set(headers)):
        _invalid(path, 1, "duplicate header")
    input_columns = {"input", "question"}.intersection(headers)
    if len(input_columns) != 1:
        _invalid(path, 1, "exactly one of input or question")


def _optional_text(
    path: Path,
    line_number: int,
    payload: Mapping[str, object],
    name: str,
    *,
    allow_blank: bool = False,
) -> str | None:
    value = payload.get(name)
    if value is None:
        return None
    if not isinstance(value, str) or (not allow_blank and not value.strip()):
        _invalid(path, line_number, name)
    return value


def _required_cell(path: Path, line_number: int, name: str, value: str | None) -> str:
    if value is None or not value.strip():
        _invalid(path, line_number, name)
    return value


def _decode_cell(path: Path, line_number: int, name: str, value: str | None) -> object:
    if value is None or not value.strip():
        _invalid(path, line_number, name)
    decoded = _decode_json(value)
    if decoded is _INVALID:
        _invalid(path, line_number, name)
    return decoded


def _decode_json(value: str) -> object:
    decoded: object = _INVALID
    with suppress(json.JSONDecodeError):
        decoded = json.loads(value)
    return decoded


def _read_text(path: Path) -> str:
    text: str | None = None
    with suppress(OSError, UnicodeError):
        text = path.read_text(encoding="utf-8-sig")
    if text is None:
        raise AssayConfigurationError(f"{path}: unable to read dataset file")
    return text


def _invalid(path: Path, line_number: int, field: str) -> NoReturn:
    raise AssayConfigurationError(f"{path}: line {line_number}: invalid {field}")
