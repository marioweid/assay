"""Disposable end-to-end SDK acceptance against the local fixture stack."""

import os
import time
from dataclasses import dataclass
from uuid import uuid4

import httpx
import pytest

import assay
from assay.cli import main
from assay.client import Client
from assay.exceptions import AssayAPIError
from assay.models import (
    Chunk,
    Dataset,
    DatasetItem,
    DatasetItemInput,
    EvalRun,
    JudgeConfig,
    ResponseMapping,
    TargetEndpoint,
    Trace,
)

ANSWER = "Assay evaluates AI systems."
CONTEXT = Chunk(id="k0", text=ANSWER)
FAILED_ANSWER = "Assay is a database."
FAILED_QUESTION = "What is Assay incorrectly?"
TRANSIENT_QUESTION = "fixture-transient: What is Assay?"
TERMINAL_QUESTION = "fixture-terminal: fail generation"


@dataclass(frozen=True)
class AcceptanceEnvironment:
    endpoint: str
    admin_token: str
    keep: bool


@dataclass(frozen=True)
class Workspace:
    project_id: str
    key_id: str
    api_key: str
    application_id: str
    application_slug: str


@pytest.fixture
def acceptance_environment() -> AcceptanceEnvironment:
    endpoint = os.getenv("ASSAY_ACCEPTANCE_ENDPOINT")
    admin_token = os.getenv("ASSAY_ACCEPTANCE_ADMIN_TOKEN")
    if not endpoint or not admin_token:
        pytest.skip("run through tests/acceptance/run.sh")
    return AcceptanceEnvironment(
        endpoint=endpoint,
        admin_token=admin_token,
        keep=os.getenv("ASSAY_ACCEPTANCE_KEEP") == "1",
    )


@pytest.mark.integration
def test_sdk_trace_to_evaluation_lifecycle(
    acceptance_environment: AcceptanceEnvironment,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    environment = acceptance_environment
    with Client(environment.endpoint, admin_token=environment.admin_token) as admin:
        workspace = _create_workspace(admin)
        _configure_sdk(monkeypatch, environment, workspace)
        complete = False
        try:
            _assert_capture_off_trace(workspace, admin)
            _assert_missing_reference(workspace, admin)
            trace = _create_scored_trace(workspace, admin)
            dataset, item = _import_trace_once(admin, workspace, trace.id)
            baseline = _assert_failed_quality_run(
                admin, environment, workspace, dataset.id, item.id, monkeypatch
            )
            _assert_retry_and_snapshot(admin, workspace, dataset.id, item.id, baseline.id)
            _assert_execution_failure_and_cancellation(
                admin, environment, workspace, dataset.id, item.id, monkeypatch
            )
            _assert_revoked_key_cannot_ingest(environment, admin, workspace)
            complete = True
        finally:
            assay.shutdown()
            if not complete or not environment.keep:
                admin.projects.delete(workspace.project_id)


def _create_workspace(admin: Client) -> Workspace:
    suffix = uuid4().hex
    project = admin.projects.create(
        f"sdk-acceptance-{suffix}",
        JudgeConfig(base_url="http://fixtures:18090/v1", model="acceptance-fixture"),
    )
    key = admin.keys.create(project.id, "acceptance-ingest")
    application = admin.applications.create(
        project.id,
        "SDK acceptance",
        f"sdk-acceptance-{suffix}",
    )
    admin.applications.set_endpoint(
        application.id,
        TargetEndpoint(
            url="http://fixtures:18090/answer",
            request_template={"query": "{{ .item.input.question }}"},
            response_mapping=ResponseMapping(output="$.answer", context="$.sources[*].text"),
        ),
    )
    return Workspace(project.id, key.id, key.key, application.id, application.slug)


def _assert_capture_off_trace(workspace: Workspace, admin: Client) -> None:
    trace_id = _emit_trace(False)
    trace = _wait_for_trace(admin, workspace.application_id, trace_id)
    assert all(
        "gen_ai.input.messages" not in span.attributes
        and "gen_ai.output.messages" not in span.attributes
        for span in trace.spans
    )
    eligibility = {item.scorer: item.eligible for item in admin.traces.eligibility(trace.id)}
    assert eligibility == {"groundedness": False, "correctness": False}


def _assert_missing_reference(workspace: Workspace, admin: Client) -> None:
    trace_id = _emit_trace(True, reference=False)
    trace = _wait_for_trace(admin, workspace.application_id, trace_id)
    eligibility = {item.scorer: item.eligible for item in admin.traces.eligibility(trace.id)}
    assert eligibility == {"groundedness": True, "correctness": False}


def _create_scored_trace(workspace: Workspace, admin: Client) -> Trace:
    trace_id = _emit_trace(True)
    trace = _wait_for_trace(admin, workspace.application_id, trace_id)
    eligibility = {item.scorer: item.eligible for item in admin.traces.eligibility(trace.id)}
    assert eligibility == {"groundedness": True, "correctness": True}
    tasks = admin.traces.score((trace.id,), ("groundedness", "correctness"))
    assert {task.scorer for task in tasks} == {"groundedness", "correctness"}
    trace = _wait_for_scores(admin, trace.id)
    scores = {score.scorer: score for score in trace.scores}
    assert scores["groundedness"].value == 0
    assert scores["correctness"].value == 0
    return trace


def _emit_trace(capture: bool, *, reference: bool = True) -> str:
    assay.init(capture=capture)
    try:
        with assay.span("sdk-acceptance-request") as request:
            trace_id = request.trace_id
            request.set_attribute("acceptance.capture", capture)
            if capture:
                with assay.span("sdk-acceptance-answer", scorable=True) as answer:
                    answer.set_messages(
                        input=[
                            {
                                "role": "user",
                                "parts": [{"type": "text", "content": FAILED_QUESTION}],
                            }
                        ],
                        output=[
                            {
                                "role": "assistant",
                                "parts": [{"type": "text", "content": FAILED_ANSWER}],
                            }
                        ],
                    )
                    answer.set_context((CONTEXT,))
                    if reference:
                        answer.set_reference(ANSWER)
        assert assay.flush()
        return trace_id
    finally:
        assay.shutdown()


def _wait_for_trace(admin: Client, application_id: str, trace_id: str) -> Trace:
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        traces = admin.traces.list(application_id=application_id, q=trace_id).items
        found = next((trace for trace in traces if trace.otel_trace_id == trace_id), None)
        if found is not None:
            return admin.traces.get(found.id)
        time.sleep(0.2)
    pytest.fail(f"trace {trace_id} was not persisted")


def _wait_for_scores(admin: Client, trace_id: str) -> Trace:
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        trace = admin.traces.get(trace_id)
        if {score.scorer for score in trace.scores} == {"groundedness", "correctness"}:
            return trace
        time.sleep(0.2)
    pytest.fail(f"trace {trace_id} was not scored")


def _import_trace_once(
    admin: Client, workspace: Workspace, trace_id: str
) -> tuple[Dataset, DatasetItem]:
    dataset = admin.datasets.create(workspace.application_id, "SDK acceptance regressions")
    item = admin.datasets.from_trace(dataset.id, trace_id, scorer="groundedness")
    assert item.input["question"] == FAILED_QUESTION
    assert item.output == FAILED_ANSWER
    with pytest.raises(AssayAPIError) as error:
        admin.datasets.from_trace(dataset.id, trace_id, scorer="groundedness")
    assert error.value.status_code == 409
    return dataset, item


def _assert_failed_quality_run(
    admin: Client,
    environment: AcceptanceEnvironment,
    workspace: Workspace,
    dataset_id: str,
    item_id: str,
    monkeypatch: pytest.MonkeyPatch,
) -> EvalRun:
    run = admin.runs.create(
        workspace.application_id,
        dataset_id,
        "failed quality",
        scorers=("groundedness", "correctness"),
    )
    run = admin.runs.wait(run.id, timeout=60, poll_interval=0.2)
    quality = admin.runs.get_item(run.id, item_id)
    assert run.status == "succeeded"
    assert quality.status == "succeeded"
    assert {score.value for score in quality.scores} == {0}
    _configure_cli(monkeypatch, environment.endpoint, workspace.api_key, environment.admin_token)
    assert main(["run", "watch", run.id, "--gate", "groundedness:0.8"]) == 1
    return run


def _assert_retry_and_snapshot(
    admin: Client,
    workspace: Workspace,
    dataset_id: str,
    item_id: str,
    baseline_id: str,
) -> None:
    admin.datasets.replace_item(
        dataset_id,
        item_id,
        item=DatasetItemInput(
            input={"question": TRANSIENT_QUESTION}, expected_output=ANSWER, context=(CONTEXT,)
        ),
    )
    run = admin.runs.create(
        workspace.application_id,
        dataset_id,
        "retry target",
        mode="generate_then_score",
        scorers=("groundedness",),
    )
    run = admin.runs.wait(run.id, timeout=60, poll_interval=0.2)
    generated = admin.runs.get_item(run.id, item_id)
    baseline = admin.runs.get_item(baseline_id, item_id)
    assert run.status == "succeeded", generated.error
    assert generated.generated_output == ANSWER
    assert generated.scores[0].value == 1
    assert baseline.snapshot.input["question"] == FAILED_QUESTION
    assert baseline.scores[0].value == 0


def _assert_execution_failure_and_cancellation(
    admin: Client,
    environment: AcceptanceEnvironment,
    workspace: Workspace,
    dataset_id: str,
    item_id: str,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    admin.datasets.replace_item(
        dataset_id,
        item_id,
        item=DatasetItemInput(
            input={"question": TERMINAL_QUESTION}, expected_output=ANSWER, context=(CONTEXT,)
        ),
    )
    failed = admin.runs.create(
        workspace.application_id,
        dataset_id,
        "target failure",
        mode="generate_then_score",
        scorers=("groundedness",),
    )
    failed = admin.runs.wait(failed.id, timeout=60, poll_interval=0.2)
    execution = admin.runs.get_item(failed.id, item_id)
    assert failed.status == "failed"
    assert execution.status == "failed"
    assert execution.error is not None
    assert execution.scores == ()
    _configure_cli(monkeypatch, environment.endpoint, workspace.api_key, environment.admin_token)
    assert main(["run", "watch", failed.id]) == 1
    admin.runs.delete(failed.id)

    canceled = admin.runs.create(
        workspace.application_id,
        dataset_id,
        "cancel target failure",
        mode="generate_then_score",
        scorers=("groundedness",),
    )
    canceled = admin.runs.cancel(canceled.id)
    assert canceled.status == "canceled"
    admin.runs.delete(canceled.id)


def _configure_sdk(
    monkeypatch: pytest.MonkeyPatch,
    environment: AcceptanceEnvironment,
    workspace: Workspace,
) -> None:
    monkeypatch.setenv("ASSAY_ENDPOINT", environment.endpoint)
    monkeypatch.setenv("ASSAY_API_KEY", workspace.api_key)
    monkeypatch.setenv("ASSAY_APPLICATION", workspace.application_slug)


def _configure_cli(
    monkeypatch: pytest.MonkeyPatch,
    endpoint: str,
    api_key: str,
    admin_token: str,
) -> None:
    monkeypatch.setenv("ASSAY_ENDPOINT", endpoint)
    monkeypatch.setenv("ASSAY_API_KEY", api_key)
    monkeypatch.setenv("ASSAY_ADMIN_TOKEN", admin_token)


def _assert_revoked_key_cannot_ingest(
    environment: AcceptanceEnvironment,
    admin: Client,
    workspace: Workspace,
) -> None:
    admin.keys.revoke(workspace.project_id, workspace.key_id)
    response = httpx.post(
        f"{environment.endpoint}/v1/traces",
        headers={"x-api-key": workspace.api_key},
        json={"resourceSpans": []},
        timeout=10,
    )
    assert response.status_code == 401
