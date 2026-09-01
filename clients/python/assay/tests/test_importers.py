from pathlib import Path

import pytest

from assay.exceptions import AssayConfigurationError
from assay.importers import parse_csv, parse_dataset_file, parse_jsonl


def test_parse_jsonl_accepts_bom_blank_lines_and_server_shape(tmp_path: Path) -> None:
    path = tmp_path / "items.jsonl"
    path.write_text(
        '\ufeff\n{"external_id":"one","input":{"question":"What?"},'
        '"output":"Answer","context":[{"id":"c1","text":"Evidence"}],'
        '"metadata":{"source":"test"}}\n',
        encoding="utf-8",
    )

    items = parse_jsonl(path)

    assert len(items) == 1
    assert items[0].input["question"] == "What?"
    assert items[0].context[0].text == "Evidence"


def test_parse_csv_supports_question_and_quoted_newline(tmp_path: Path) -> None:
    path = tmp_path / "items.csv"
    path.write_text(
        'question,output,context,metadata\n"What,\nexactly?",Answer,'
        '"[{""id"":""c1"",""text"":""Evidence""}]",'
        '"{""source"":""test""}"\n',
        encoding="utf-8",
    )

    item = parse_csv(path)[0]

    assert item.input == {"question": "What,\nexactly?"}
    assert item.context[0].id == "c1"


@pytest.mark.parametrize(
    ("name", "content", "field"),
    [
        ("items.jsonl", '{"input":"secret value"}\n', "input"),
        ("items.jsonl", "not secret json\n", "object"),
        ("items.csv", "input,question\n{},secret\n", "exactly one"),
        ("items.csv", "question,metadata\nsecret,[]\n", "metadata"),
        ("items.csv", "question,question\none,two\n", "duplicate"),
    ],
)
def test_parser_errors_omit_source_content(
    tmp_path: Path,
    name: str,
    content: str,
    field: str,
) -> None:
    path = tmp_path / name
    path.write_text(content, encoding="utf-8")

    with pytest.raises(AssayConfigurationError, match=field) as captured:
        parse_dataset_file(path)

    assert "secret" not in str(captured.value)


def test_parse_dataset_file_rejects_empty_and_unsupported_files(tmp_path: Path) -> None:
    empty = tmp_path / "empty.jsonl"
    empty.write_text("", encoding="utf-8")
    unsupported = tmp_path / "items.txt"
    unsupported.write_text("content", encoding="utf-8")

    assert parse_dataset_file(empty) == ()
    with pytest.raises(AssayConfigurationError, match="unsupported dataset file extension"):
        parse_dataset_file(unsupported)
