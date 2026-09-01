from importlib import metadata, resources

import assay


def test_exposes_functional_release_version() -> None:
    assert assay.__version__ == "0.2.0"


def test_distribution_exposes_console_entry_point() -> None:
    entry_points = metadata.entry_points(group="console_scripts", name="assay")

    assert len(entry_points) == 1
    assert next(iter(entry_points)).value == "assay.cli:main"


def test_distribution_includes_typed_marker() -> None:
    marker = resources.files("assay").joinpath("py.typed")

    assert marker.is_file()
