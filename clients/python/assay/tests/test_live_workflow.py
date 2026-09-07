"""Opt-in M6 smoke test against a running server and its configured judge.

Set ASSAY_LIVE_TEST_ENDPOINT and ASSAY_ADMIN_TOKEN to run. Uses synthetic data,
makes six judge calls in the normal path, and deletes its temporary project.
"""

import json
import os
import time
from collections.abc import Iterator
from contextlib import redirect_stdout
from io import StringIO
from uuid import uuid4

import pytest

import assay
from assay.cli import main
from assay.client import Client
from assay.models import Application, DatasetItemInput, Score


@pytest.fixture
def live_application(monkeypatch: pytest.MonkeyPatch) -> Iterator[tuple[Client, Application]]:
    endpoint = os.getenv("ASSAY_LIVE_TEST_ENDPOINT")
    if not endpoint:
        pytest.skip("set ASSAY_LIVE_TEST_ENDPOINT to opt into judge calls")
    monkeypatch.setenv("ASSAY_ENDPOINT", endpoint)
    with Client(endpoint, admin_token=os.environ["ASSAY_ADMIN_TOKEN"]) as admin:
        project = admin.projects.create(f"m6-smoke-{uuid4().hex}")
        try:
            key = admin.keys.create(project.id, "smoke")
            application = admin.applications.create(
                project.id,
                "M6 smoke",
                f"m6-{uuid4().hex}",
                auto_score_scorers=("groundedness",),
            )
            assay.init(
                endpoint=endpoint, api_key=key.key, application=application.slug, capture=True
            )
            with Client(
                endpoint, api_key=key.key, admin_token=os.environ["ASSAY_ADMIN_TOKEN"]
            ) as client:
                yield client, application
        finally:
            assay.shutdown()
            admin.projects.delete(project.id)


def wait_for_score(client: Client, application_id: str) -> Score:
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        scores = client.scores.list(application_id).items
        if scores:
            return scores[0]
        time.sleep(0.5)
    pytest.fail("online scoring did not finish within 120 seconds")


def run_gate(application_id: str, dataset_id: str, expected_exit: int) -> None:
    output = StringIO()
    with redirect_stdout(output):
        assert (
            main(
                [
                    "run",
                    "create",
                    application_id,
                    "--dataset",
                    dataset_id,
                    "--scorers",
                    "groundedness",
                ]
            )
            == 0
        )
    run_id = json.loads(output.getvalue())["id"]
    assert (
        main(
            [
                "run",
                "watch",
                run_id,
                "--gate",
                "groundedness:0.8",
                "--timeout",
                "120",
            ]
        )
        == expected_exit
    )


@pytest.mark.integration
def test_live_trace_to_regression_and_gate(
    live_application: tuple[Client, Application],
    capsys: pytest.CaptureFixture[str],
) -> None:
    client, application = live_application
    with assay.span("unsupported-answer", scorable=True) as span:
        span.set_input("What is the capital of France?")
        span.set_output("The capital of France is Berlin.")
        span.set_context([{"id": "k0", "text": "The capital of France is Paris."}])
    assert assay.flush()
    score = wait_for_score(client, application.id)
    assert not score.passed, score.rationale
    assert score.trace_id is not None
    assert main(["scores", "export", application.id, "--failed"]) == 0
    exported = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
    assert exported[0]["trace_id"] == score.trace_id
    assert exported[0]["judged_output"] == "The capital of France is Berlin."

    dataset = client.datasets.create(application.id, "regressions")
    command = ["datasets", "from-trace", dataset.id, score.trace_id, "--scorer", "groundedness"]
    assert main(command) == 0
    assert main(command) == 2
    items = tuple(client.datasets.iter_all_items(dataset.id))
    assert len(items) == 1
    assert items[0].output == score.judged_output
    assert items[0].context == score.judged_context
    run_gate(application.id, dataset.id, 1)
    corrected = client.datasets.create(application.id, "corrected")
    client.datasets.create_items(
        corrected.id,
        (
            DatasetItemInput(
                input=items[0].input,
                output="The capital of France is Paris.",
                context=items[0].context,
                external_id=items[0].external_id,
            ),
        ),
    )
    run_gate(application.id, corrected.id, 0)
    points = client.metrics.list(application.id)
    assert sum(point.n for point in points) == 3
    assert main(["metrics", application.id]) == 0
