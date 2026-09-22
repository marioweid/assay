from importlib import metadata, resources

import assay


def test_exposes_functional_release_version() -> None:
    assert assay.__version__ == "0.3.0"


def test_package_exports_structured_message_types() -> None:
    assert assay.Message.__required_keys__ == frozenset({"role", "parts"})
    assert assay.TextPart.__required_keys__ == frozenset({"type", "content"})
    assert assay.ToolCallPart.__required_keys__ == frozenset({"type", "name"})
    assert assay.ToolResultPart.__required_keys__ == frozenset({"type", "response"})


def test_package_exports_product_workflow_models() -> None:
    assert assay.RunComparisonPage.__dataclass_fields__["summary"]
    assert assay.ScoringEligibility.__dataclass_fields__["reasons"]
    assert assay.TraceScoreSummary.__dataclass_fields__["value"]


def test_distribution_exposes_console_entry_point() -> None:
    entry_points = metadata.entry_points(group="console_scripts", name="assay")

    assert len(entry_points) == 1
    assert next(iter(entry_points)).value == "assay.cli:main"


def test_distribution_includes_typed_marker() -> None:
    marker = resources.files("assay").joinpath("py.typed")

    assert marker.is_file()
