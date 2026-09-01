from dataclasses import FrozenInstanceError
from typing import cast

import pytest

from assay.exceptions import AssayAPIError, AssayError, AssayImportError
from assay.models import Chunk, Page, ResponseMapping, TargetEndpoint


def test_shared_models_are_immutable() -> None:
    chunk = Chunk(id="k0", text="Assay evaluates AI systems.")
    page = Page(items=(chunk,), next_cursor="next")

    assert page.items == (chunk,)
    attribute = "text"
    with pytest.raises(FrozenInstanceError):
        setattr(chunk, attribute, "changed")


def test_target_endpoint_copies_mutable_inputs_and_redacts_secret() -> None:
    headers = {"Authorization": "Bearer {{ .secret }}"}
    template = {"question": "{{ .item.input.question }}"}
    endpoint = TargetEndpoint(
        url="https://example.test/answer",
        response_mapping=ResponseMapping(output="$.answer"),
        headers=headers,
        request_template=template,
        secret="target-secret",
    )

    headers["Authorization"] = "changed"
    template["question"] = "changed"

    assert endpoint.headers["Authorization"] == "Bearer {{ .secret }}"
    assert endpoint.request_template["question"] == "{{ .item.input.question }}"
    assert "target-secret" not in repr(endpoint)
    with pytest.raises(TypeError):
        cast(dict[str, str], endpoint.headers)["new"] = "value"


def test_api_error_renders_only_safe_fields() -> None:
    error = AssayAPIError(
        operation="create project",
        status_code=400,
        title="Bad Request",
        detail="invalid name",
    )

    assert isinstance(error, AssayError)
    assert str(error) == "create project: HTTP 400 Bad Request: invalid name"


def test_import_error_reports_committed_progress() -> None:
    error = AssayImportError(
        "import dataset: request failed",
        committed_items=1_000,
        batch_number=2,
    )

    assert error.committed_items == 1_000
    assert error.batch_number == 2
    assert str(error) == "import dataset: request failed"
