"""Typed client and safe HTTP transport for the Assay API."""

from __future__ import annotations

import math
import time
from collections.abc import Callable, Iterator, Mapping
from contextlib import suppress
from datetime import datetime
from pathlib import Path
from types import TracebackType
from typing import Literal, TypeVar, cast
from urllib.parse import quote

import httpx

from assay._parsing import (
    parse_api_key,
    parse_application,
    parse_collection,
    parse_created_api_key,
    parse_dataset,
    parse_dataset_item,
    parse_eval_run,
    parse_eval_run_item,
    parse_page,
    parse_project,
    parse_score,
    parse_scorer_config,
    parse_scoring_task,
    parse_trace,
)
from assay.exceptions import (
    AssayAPIError,
    AssayConfigurationError,
    AssayError,
    AssayImportError,
    AssayProtocolError,
    AssayTimeoutError,
    AssayTransportError,
)
from assay.importers import parse_dataset_file
from assay.models import (
    APIKey,
    Application,
    CreatedAPIKey,
    Dataset,
    DatasetItem,
    DatasetItemInput,
    EvalRun,
    EvalRunItem,
    ImportResult,
    JSONValue,
    JudgeConfig,
    Page,
    Project,
    Score,
    ScorerConfig,
    ScoringTask,
    TargetEndpoint,
    Trace,
)

AuthMode = Literal["none", "admin", "project"]
JsonObject = dict[str, object]
USER_AGENT = "assay-sdk/0.2.0"
_INVALID_JSON = object()
T = TypeVar("T")


class _Transport:
    def __init__(
        self,
        endpoint: str,
        *,
        api_key: str | None,
        admin_token: str | None,
        timeout: float,
        http_client: httpx.Client | None = None,
    ) -> None:
        self._endpoint = _validate_endpoint(endpoint)
        self._api_key = api_key
        self._admin_token = admin_token
        self._owns_client = http_client is None
        self._client = http_client or httpx.Client(timeout=timeout)

    def request(
        self,
        operation: str,
        method: str,
        path: str,
        *,
        auth: AuthMode,
        json: Mapping[str, object] | None = None,
        params: httpx.QueryParams | None = None,
        expect_json: bool = True,
    ) -> JsonObject | None:
        headers = {"User-Agent": USER_AGENT, **self._auth_headers(auth)}
        response: httpx.Response | None = None
        failure: type[AssayError] | None = None
        try:
            response = self._client.request(
                method,
                f"{self._endpoint}{path}",
                headers=headers,
                json=json,
                params=params,
            )
        except httpx.TimeoutException:
            failure = AssayTimeoutError
        except httpx.RequestError:
            failure = AssayTransportError

        if failure is not None:
            raise failure(f"{operation}: request failed")
        if response is None:
            raise AssayTransportError(f"{operation}: request failed")
        if response.is_error:
            raise _api_error(operation, response)
        if response.status_code == httpx.codes.NO_CONTENT or not expect_json:
            return None
        return _json_object(operation, response)

    def check(self, operation: str, path: str, *, auth: AuthMode) -> None:
        self.request(operation, "GET", path, auth=auth, expect_json=False)

    def close(self) -> None:
        if self._owns_client:
            self._client.close()

    def _auth_headers(self, auth: AuthMode) -> dict[str, str]:
        if auth == "none":
            return {}
        if auth == "admin":
            if not self._admin_token:
                raise AssayConfigurationError("admin credential is required")
            return {"Authorization": f"Bearer {self._admin_token}"}
        if not self._api_key:
            raise AssayConfigurationError("project credential is required")
        return {"X-API-Key": self._api_key}


class Client:
    """Synchronous, context-managed client for Assay management APIs."""

    def __init__(
        self,
        endpoint: str,
        *,
        api_key: str | None = None,
        admin_token: str | None = None,
        timeout: float = 10.0,
        _http_client: httpx.Client | None = None,
    ) -> None:
        _validate_timeout(timeout)
        self._transport = _Transport(
            endpoint,
            api_key=api_key,
            admin_token=admin_token,
            timeout=timeout,
            http_client=_http_client,
        )
        self.projects = ProjectsResource(self._transport)
        self.keys = KeysResource(self._transport)
        self.applications = ApplicationsResource(self._transport)
        self.datasets = DatasetsResource(self._transport)
        self.scorers = ScorersResource(self._transport)
        self.runs = RunsResource(self._transport)
        self.traces = TracesResource(self._transport)

    def ready(self) -> None:
        """Raise if the Assay service is not ready."""
        self._transport.check("readiness check", "/readyz", auth="none")

    def close(self) -> None:
        """Release transport resources owned by this client."""
        self._transport.close()

    def __enter__(self) -> Client:
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc_value: BaseException | None,
        traceback: TracebackType | None,
    ) -> None:
        self.close()


class ProjectsResource:
    """Admin-authenticated project management operations."""

    def __init__(self, transport: _Transport) -> None:
        self._transport = transport

    def create(self, name: str, judge_config: JudgeConfig | None = None) -> Project:
        name = _required_text(name, "project name")
        body: dict[str, object] = {"name": name}
        if judge_config is not None:
            body["judge_config"] = _judge_body(judge_config)
        operation = "create project"
        payload = self._transport.request(
            operation, "POST", "/v1/projects", auth="admin", json=body
        )
        return parse_project(operation, _payload(operation, payload))

    def list(self) -> tuple[Project, ...]:
        operation = "list projects"
        payload = self._transport.request(operation, "GET", "/v1/projects", auth="admin")
        return parse_collection(operation, _payload(operation, payload), parse_project)

    def get(self, project_id: str) -> Project:
        operation = "get project"
        payload = self._transport.request(
            operation, "GET", f"/v1/projects/{_segment(project_id, 'project ID')}", auth="admin"
        )
        return parse_project(operation, _payload(operation, payload))

    def update(
        self,
        project_id: str,
        *,
        name: str | None = None,
        judge_config: JudgeConfig | None = None,
        clear_judge_config: bool = False,
    ) -> Project:
        body: dict[str, object] = {}
        if name is not None:
            body["name"] = _required_text(name, "project name")
        if judge_config is not None:
            body["judge_config"] = _judge_body(judge_config)
        if clear_judge_config:
            body["clear_judge_config"] = True
        operation = "update project"
        payload = self._transport.request(
            operation,
            "PATCH",
            f"/v1/projects/{_segment(project_id, 'project ID')}",
            auth="admin",
            json=body,
        )
        return parse_project(operation, _payload(operation, payload))

    def delete(self, project_id: str) -> None:
        self._transport.request(
            "delete project",
            "DELETE",
            f"/v1/projects/{_segment(project_id, 'project ID')}",
            auth="admin",
        )


class KeysResource:
    """Admin-authenticated project API-key operations."""

    def __init__(self, transport: _Transport) -> None:
        self._transport = transport

    def create(self, project_id: str, name: str) -> CreatedAPIKey:
        operation = "create API key"
        path = f"/v1/projects/{_segment(project_id, 'project ID')}/keys"
        payload = self._transport.request(
            operation,
            "POST",
            path,
            auth="admin",
            json={"name": _required_text(name, "key name")},
        )
        return parse_created_api_key(operation, _payload(operation, payload))

    def list(self, project_id: str) -> tuple[APIKey, ...]:
        operation = "list API keys"
        path = f"/v1/projects/{_segment(project_id, 'project ID')}/keys"
        payload = self._transport.request(operation, "GET", path, auth="admin")
        return parse_collection(operation, _payload(operation, payload), parse_api_key)

    def revoke(self, project_id: str, key_id: str) -> None:
        project = _segment(project_id, "project ID")
        key = _segment(key_id, "key ID")
        self._transport.request(
            "revoke API key",
            "DELETE",
            f"/v1/projects/{project}/keys/{key}",
            auth="admin",
        )


class ApplicationsResource:
    """Admin-authenticated application management operations."""

    def __init__(self, transport: _Transport) -> None:
        self._transport = transport

    def create(
        self,
        project_id: str,
        name: str,
        slug: str,
        *,
        config: Mapping[str, JSONValue] | None = None,
        auto_score_scorers: tuple[str, ...] = (),
    ) -> Application:
        body: dict[str, object] = {
            "project_id": _required_text(project_id, "project ID"),
            "name": _required_text(name, "application name"),
            "slug": _required_text(slug, "application slug"),
        }
        if config is not None:
            body["config"] = _mutable_mapping(config)
        if auto_score_scorers:
            body["auto_score_scorers"] = _string_list(auto_score_scorers, "scorer")
        return self._application_request("create application", "POST", "/v1/applications", body)

    def list(self, project_id: str | None = None) -> tuple[Application, ...]:
        operation = "list applications"
        params = None
        if project_id is not None:
            params = httpx.QueryParams({"project_id": _required_text(project_id, "project ID")})
        payload = self._transport.request(
            operation, "GET", "/v1/applications", auth="admin", params=params
        )
        return parse_collection(operation, _payload(operation, payload), parse_application)

    def get(self, application_id: str) -> Application:
        application = _segment(application_id, "application ID")
        return self._application_request(
            "get application", "GET", f"/v1/applications/{application}"
        )

    def update(
        self,
        application_id: str,
        *,
        name: str | None = None,
        slug: str | None = None,
        config: Mapping[str, JSONValue] | None = None,
        auto_score_scorers: tuple[str, ...] | None = None,
    ) -> Application:
        body: dict[str, object] = {}
        if name is not None:
            body["name"] = _required_text(name, "application name")
        if slug is not None:
            body["slug"] = _required_text(slug, "application slug")
        if config is not None:
            body["config"] = _mutable_mapping(config)
        if auto_score_scorers is not None:
            body["auto_score_scorers"] = _string_list(auto_score_scorers, "scorer")
        application = _segment(application_id, "application ID")
        return self._application_request(
            "update application", "PATCH", f"/v1/applications/{application}", body
        )

    def delete(self, application_id: str) -> None:
        application = _segment(application_id, "application ID")
        self._transport.request(
            "delete application",
            "DELETE",
            f"/v1/applications/{application}",
            auth="admin",
        )

    def set_endpoint(
        self,
        application_id: str,
        endpoint: TargetEndpoint,
    ) -> Application:
        application = _segment(application_id, "application ID")
        return self._application_request(
            "update application endpoint",
            "PATCH",
            f"/v1/applications/{application}/endpoint",
            {"endpoint": _target_body(endpoint)},
        )

    def clear_endpoint(self, application_id: str) -> Application:
        application = _segment(application_id, "application ID")
        return self._application_request(
            "update application endpoint",
            "PATCH",
            f"/v1/applications/{application}/endpoint",
            {"clear": True},
        )

    def _application_request(
        self,
        operation: str,
        method: str,
        path: str,
        body: Mapping[str, object] | None = None,
    ) -> Application:
        payload = self._transport.request(operation, method, path, auth="admin", json=body)
        return parse_application(operation, _payload(operation, payload))


class DatasetsResource:
    """Admin-authenticated dataset operations."""

    def __init__(self, transport: _Transport) -> None:
        self._transport = transport

    def create(
        self,
        application_id: str,
        name: str,
        description: str | None = None,
    ) -> Dataset:
        body: dict[str, object] = {
            "application_id": _required_text(application_id, "application ID"),
            "name": _required_text(name, "dataset name"),
        }
        if description is not None:
            body["description"] = description
        operation = "create dataset"
        payload = self._transport.request(
            operation, "POST", "/v1/datasets", auth="admin", json=body
        )
        return parse_dataset(operation, _payload(operation, payload))

    def list(
        self,
        *,
        application_id: str | None = None,
        limit: int | None = None,
        cursor: str | None = None,
    ) -> Page[Dataset]:
        params = _page_params(limit, cursor, maximum=500)
        if application_id is not None:
            params = _with_param(params, "application_id", application_id, "application ID")
        operation = "list datasets"
        payload = self._transport.request(
            operation, "GET", "/v1/datasets", auth="admin", params=params
        )
        return parse_page(operation, _payload(operation, payload), parse_dataset)

    def get(self, dataset_id: str) -> Dataset:
        operation = "get dataset"
        payload = self._transport.request(
            operation,
            "GET",
            f"/v1/datasets/{_segment(dataset_id, 'dataset ID')}",
            auth="admin",
        )
        return parse_dataset(operation, _payload(operation, payload))

    def delete(self, dataset_id: str) -> None:
        self._transport.request(
            "delete dataset",
            "DELETE",
            f"/v1/datasets/{_segment(dataset_id, 'dataset ID')}",
            auth="admin",
        )

    def create_items(
        self,
        dataset_id: str,
        items: tuple[DatasetItemInput, ...],
    ) -> tuple[DatasetItem, ...]:
        if not 1 <= len(items) <= 1000:
            raise AssayConfigurationError("dataset item count must be between 1 and 1000")
        operation = "create dataset items"
        body = {"items": [_dataset_item_body(item) for item in items]}
        payload = self._transport.request(
            operation,
            "POST",
            f"/v1/datasets/{_segment(dataset_id, 'dataset ID')}/items",
            auth="admin",
            json=body,
        )
        return parse_collection(operation, _payload(operation, payload), parse_dataset_item)

    def list_items(
        self,
        dataset_id: str,
        *,
        limit: int | None = None,
        cursor: str | None = None,
    ) -> Page[DatasetItem]:
        operation = "list dataset items"
        payload = self._transport.request(
            operation,
            "GET",
            f"/v1/datasets/{_segment(dataset_id, 'dataset ID')}/items",
            auth="admin",
            params=_page_params(limit, cursor, maximum=500),
        )
        return parse_page(operation, _payload(operation, payload), parse_dataset_item)

    def iter_all_datasets(
        self,
        *,
        application_id: str | None = None,
        limit: int = 500,
    ) -> Iterator[Dataset]:
        return _iter_pages(
            lambda cursor: self.list(
                application_id=application_id,
                limit=limit,
                cursor=cursor,
            )
        )

    def iter_all_items(
        self,
        dataset_id: str,
        *,
        limit: int = 500,
    ) -> Iterator[DatasetItem]:
        return _iter_pages(lambda cursor: self.list_items(dataset_id, limit=limit, cursor=cursor))

    def ensure(
        self,
        application_id: str,
        name: str,
        description: str | None = None,
    ) -> Dataset:
        application_id = _required_text(application_id, "application ID")
        name = _required_text(name, "dataset name")
        matches = self._matching_datasets(application_id, name)
        if len(matches) == 1:
            return matches[0]
        if len(matches) > 1:
            raise AssayConfigurationError("ambiguous dataset name")
        try:
            return self.create(application_id, name, description)
        except AssayAPIError as error:
            if error.status_code != 409:
                raise
        matches = self._matching_datasets(application_id, name)
        if len(matches) == 1:
            return matches[0]
        if len(matches) > 1:
            raise AssayConfigurationError("ambiguous dataset name")
        raise AssayConfigurationError("dataset conflict could not be resolved")

    def import_file(
        self,
        dataset_id: str,
        path: Path,
        *,
        batch_size: int = 1000,
    ) -> ImportResult:
        items = parse_dataset_file(path)
        dataset_id = _required_text(dataset_id, "dataset ID")
        if isinstance(batch_size, bool) or not 1 <= batch_size <= 1000:
            raise AssayConfigurationError("batch size must be between 1 and 1000")
        if not items:
            raise AssayConfigurationError("dataset import file contains no items")
        committed = 0
        batches = 0
        for offset in range(0, len(items), batch_size):
            batches += 1
            try:
                created = self.create_items(
                    dataset_id,
                    items[offset : offset + batch_size],
                )
            except AssayError as error:
                raise AssayImportError(
                    f"import dataset items: batch {batches} failed",
                    committed_items=committed,
                    batch_number=batches,
                ) from error
            committed += len(created)
        return ImportResult(dataset_id=dataset_id, created_items=committed, batches=batches)

    def _matching_datasets(self, application_id: str, name: str) -> tuple[Dataset, ...]:
        return tuple(
            dataset
            for dataset in self.iter_all_datasets(application_id=application_id)
            if dataset.name == name
        )


class ScorersResource:
    """Admin-authenticated scorer configuration operations."""

    def __init__(self, transport: _Transport) -> None:
        self._transport = transport

    def list(self, application_id: str) -> tuple[ScorerConfig, ...]:
        operation = "list scorer configs"
        application = _segment(application_id, "application ID")
        payload = self._transport.request(
            operation,
            "GET",
            f"/v1/applications/{application}/scorers",
            auth="admin",
        )
        return parse_collection(operation, _payload(operation, payload), parse_scorer_config)

    def set(
        self,
        application_id: str,
        scorer: str,
        *,
        enabled: bool | None = None,
        threshold: float | None = None,
        judge_config: JudgeConfig | None = None,
        prompt_template_id: str | None = None,
    ) -> ScorerConfig:
        scorer = _scorer(scorer)
        body: dict[str, object] = {}
        if enabled is not None:
            body["enabled"] = enabled
        if threshold is not None:
            body["threshold"] = _threshold(threshold)
        if judge_config is not None:
            body["judge_config"] = _judge_body(judge_config)
        if prompt_template_id is not None:
            body["prompt_template_id"] = _required_text(prompt_template_id, "prompt template ID")
        operation = "put scorer config"
        application = _segment(application_id, "application ID")
        payload = self._transport.request(
            operation,
            "PUT",
            f"/v1/applications/{application}/scorers/{scorer}",
            auth="admin",
            json=body,
        )
        return parse_scorer_config(operation, _payload(operation, payload))


class RunsResource:
    """Admin-authenticated evaluation run operations."""

    def __init__(
        self,
        transport: _Transport,
        *,
        sleep: Callable[[float], None] = time.sleep,
        monotonic: Callable[[], float] = time.monotonic,
    ) -> None:
        self._transport = transport
        self._sleep = sleep
        self._monotonic = monotonic

    def create(
        self,
        application_id: str,
        dataset_id: str,
        name: str,
        *,
        mode: str = "score_existing",
        scorers: tuple[str, ...],
        params: Mapping[str, JSONValue] | None = None,
    ) -> EvalRun:
        if mode not in {"score_existing", "generate_then_score"}:
            raise AssayConfigurationError("invalid evaluation mode")
        body: dict[str, object] = {
            "application_id": _required_text(application_id, "application ID"),
            "dataset_id": _required_text(dataset_id, "dataset ID"),
            "name": _required_text(name, "run name"),
            "mode": mode,
            "scorers": _scorers(scorers),
        }
        if params is not None:
            body["params"] = _mutable_mapping(params)
        return self._run_request("create eval run", "POST", "/v1/runs", body)

    def list(
        self,
        *,
        application_id: str | None = None,
        status: str | None = None,
        limit: int | None = None,
        cursor: str | None = None,
    ) -> Page[EvalRun]:
        params = _page_params(limit, cursor, maximum=500)
        if application_id is not None:
            params = _with_param(params, "application_id", application_id, "application ID")
        if status is not None:
            params = _with_param(params, "status", status, "run status")
        operation = "list eval runs"
        payload = self._transport.request(operation, "GET", "/v1/runs", auth="admin", params=params)
        return parse_page(operation, _payload(operation, payload), parse_eval_run)

    def get(self, run_id: str) -> EvalRun:
        return self._run_request("get eval run", "GET", f"/v1/runs/{_segment(run_id, 'run ID')}")

    def list_items(
        self,
        run_id: str,
        *,
        limit: int | None = None,
        cursor: str | None = None,
    ) -> Page[EvalRunItem]:
        operation = "list eval run items"
        payload = self._transport.request(
            operation,
            "GET",
            f"/v1/runs/{_segment(run_id, 'run ID')}/items",
            auth="admin",
            params=_page_params(limit, cursor, maximum=500),
        )
        return parse_page(operation, _payload(operation, payload), parse_eval_run_item)

    def list_scores(
        self,
        run_id: str,
        *,
        limit: int | None = None,
        cursor: str | None = None,
    ) -> Page[Score]:
        operation = "list eval run scores"
        payload = self._transport.request(
            operation,
            "GET",
            f"/v1/runs/{_segment(run_id, 'run ID')}/scores",
            auth="admin",
            params=_page_params(limit, cursor, maximum=500),
        )
        return parse_page(operation, _payload(operation, payload), parse_score)

    def cancel(self, run_id: str) -> EvalRun:
        return self._run_request(
            "cancel eval run",
            "POST",
            f"/v1/runs/{_segment(run_id, 'run ID')}/cancel",
            {},
        )

    def iter_all_runs(
        self,
        *,
        application_id: str | None = None,
        status: str | None = None,
        limit: int = 500,
    ) -> Iterator[EvalRun]:
        return _iter_pages(
            lambda cursor: self.list(
                application_id=application_id,
                status=status,
                limit=limit,
                cursor=cursor,
            )
        )

    def iter_all_items(self, run_id: str, *, limit: int = 500) -> Iterator[EvalRunItem]:
        return _iter_pages(lambda cursor: self.list_items(run_id, limit=limit, cursor=cursor))

    def iter_all_scores(self, run_id: str, *, limit: int = 500) -> Iterator[Score]:
        return _iter_pages(lambda cursor: self.list_scores(run_id, limit=limit, cursor=cursor))

    def wait(
        self,
        run_id: str,
        *,
        timeout: float | None = None,
        poll_interval: float = 1.0,
        transient_errors: int = 3,
        on_update: Callable[[EvalRun], None] | None = None,
    ) -> EvalRun:
        run_id = _required_text(run_id, "run ID")
        _polling_values(timeout, poll_interval, transient_errors)
        deadline = self._monotonic() + timeout if timeout is not None else None
        failures = 0
        while True:
            if deadline is not None and self._monotonic() >= deadline:
                raise AssayTimeoutError(f"wait for run {run_id}: timed out")
            try:
                run = self.get(run_id)
            except (AssayTransportError, AssayTimeoutError):
                failures += 1
                if failures > transient_errors:
                    raise
            else:
                failures = 0
                if on_update is not None:
                    on_update(run)
                if run.status in {"succeeded", "failed", "canceled"}:
                    return run
            delay = _poll_delay(deadline, poll_interval, self._monotonic(), run_id)
            self._sleep(delay)

    def _run_request(
        self,
        operation: str,
        method: str,
        path: str,
        body: Mapping[str, object] | None = None,
    ) -> EvalRun:
        payload = self._transport.request(operation, method, path, auth="admin", json=body)
        return parse_eval_run(operation, _payload(operation, payload))


class TracesResource:
    """Project-authenticated trace inspection and scoring operations."""

    def __init__(self, transport: _Transport) -> None:
        self._transport = transport

    def list(
        self,
        *,
        application_id: str | None = None,
        start: datetime | None = None,
        end: datetime | None = None,
        status: str | None = None,
        limit: int | None = None,
        cursor: str | None = None,
    ) -> Page[Trace]:
        params = _page_params(limit, cursor, maximum=200)
        if application_id is not None:
            params = _with_param(params, "application_id", application_id, "application ID")
        if start is not None:
            params = _with_raw_param(params, "start", _query_timestamp(start, "start"))
        if end is not None:
            params = _with_raw_param(params, "end", _query_timestamp(end, "end"))
        if status is not None:
            params = _with_param(params, "status", status, "trace status")
        operation = "list traces"
        payload = self._transport.request(
            operation, "GET", "/v1/traces", auth="project", params=params
        )
        return parse_page(operation, _payload(operation, payload), parse_trace)

    def get(self, trace_id: str) -> Trace:
        operation = "get trace"
        payload = self._transport.request(
            operation,
            "GET",
            f"/v1/traces/{_segment(trace_id, 'trace ID')}",
            auth="project",
        )
        return parse_trace(operation, _payload(operation, payload))

    def score(
        self,
        trace_ids: tuple[str, ...],
        scorers: tuple[str, ...],
    ) -> tuple[ScoringTask, ...]:
        body = {
            "trace_ids": _unique_texts(trace_ids, "trace IDs"),
            "scorers": _scorers(scorers),
        }
        operation = "score traces"
        payload = self._transport.request(
            operation, "POST", "/v1/traces/score", auth="project", json=body
        )
        return parse_collection(operation, _payload(operation, payload), parse_scoring_task)

    def set_reference(self, trace_id: str, reference_answer: str) -> Trace:
        operation = "attach trace reference"
        payload = self._transport.request(
            operation,
            "PATCH",
            f"/v1/traces/{_segment(trace_id, 'trace ID')}/reference",
            auth="project",
            json={"reference_answer": _required_text(reference_answer, "reference answer")},
        )
        return parse_trace(operation, _payload(operation, payload))

    def iter_all_traces(
        self,
        *,
        application_id: str | None = None,
        limit: int = 200,
    ) -> Iterator[Trace]:
        return _iter_pages(
            lambda cursor: self.list(
                application_id=application_id,
                limit=limit,
                cursor=cursor,
            )
        )


def _validate_endpoint(endpoint: str) -> str:
    url: httpx.URL | None = None
    with suppress(httpx.InvalidURL):
        url = httpx.URL(endpoint)
    if (
        url is None
        or url.scheme not in {"http", "https"}
        or not url.host
        or url.userinfo
        or url.path not in {"", "/"}
        or url.query
        or url.fragment
    ):
        raise AssayConfigurationError("endpoint must be an HTTP or HTTPS base URL")
    return str(url).rstrip("/")


def _validate_timeout(timeout: float) -> None:
    if isinstance(timeout, bool) or not math.isfinite(timeout) or timeout <= 0:
        raise AssayConfigurationError("timeout must be a positive finite number")


def _api_error(operation: str, response: httpx.Response) -> AssayAPIError:
    title: str | None = None
    detail: str | None = None
    payload = _decode_json(response)
    if isinstance(payload, dict):
        raw_title = payload.get("title")
        raw_detail = payload.get("detail")
        title = raw_title if isinstance(raw_title, str) else None
        detail = raw_detail if isinstance(raw_detail, str) else None
    return AssayAPIError(
        operation=operation,
        status_code=response.status_code,
        title=title,
        detail=detail,
    )


def _json_object(operation: str, response: httpx.Response) -> JsonObject:
    payload = _decode_json(response)
    if not isinstance(payload, dict):
        raise AssayProtocolError(f"{operation}: invalid JSON response")
    return cast(JsonObject, payload)


def _decode_json(response: httpx.Response) -> object:
    try:
        return response.json()
    except ValueError:
        return _INVALID_JSON


def _payload(operation: str, payload: JsonObject | None) -> JsonObject:
    if payload is None:
        raise AssayProtocolError(f"{operation}: invalid JSON response")
    return payload


def _required_text(value: str, name: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise AssayConfigurationError(f"{name} must not be blank")
    return value.strip()


def _segment(value: str, name: str) -> str:
    return quote(_required_text(value, name), safe="")


def _judge_body(config: JudgeConfig) -> dict[str, object]:
    body: dict[str, object] = {
        "base_url": config.base_url,
        "model": config.model,
    }
    if config.api_key is not None:
        body["api_key"] = config.api_key
    return body


def _target_body(endpoint: TargetEndpoint) -> dict[str, object]:
    response_mapping: dict[str, object] = {"output": endpoint.response_mapping.output}
    if endpoint.response_mapping.context is not None:
        response_mapping["context"] = endpoint.response_mapping.context
    body: dict[str, object] = {
        "url": endpoint.url,
        "method": endpoint.method,
        "headers": dict(endpoint.headers),
        "request_template": _mutable_mapping(endpoint.request_template),
        "response_mapping": response_mapping,
        "timeout_ms": endpoint.timeout_ms,
    }
    if endpoint.secret is not None:
        body["secret"] = endpoint.secret
    return body


def _mutable_mapping(values: Mapping[str, JSONValue]) -> dict[str, object]:
    return {key: _mutable_json(value) for key, value in values.items()}


def _mutable_json(value: JSONValue) -> object:
    if isinstance(value, Mapping):
        return {key: _mutable_json(item) for key, item in value.items()}
    if isinstance(value, list | tuple):
        return [_mutable_json(item) for item in value]
    return value


def _string_list(values: tuple[str, ...], name: str) -> list[str]:
    return [_required_text(value, name) for value in values]


def _page_params(
    limit: int | None,
    cursor: str | None,
    *,
    maximum: int,
) -> httpx.QueryParams | None:
    values: dict[str, str | int] = {}
    if limit is not None:
        if isinstance(limit, bool) or not 1 <= limit <= maximum:
            raise AssayConfigurationError(f"page limit must be between 1 and {maximum}")
        values["limit"] = limit
    if cursor is not None:
        values["cursor"] = _required_text(cursor, "page cursor")
    return httpx.QueryParams(values) if values else None


def _with_param(
    params: httpx.QueryParams | None,
    key: str,
    value: str,
    name: str,
) -> httpx.QueryParams:
    values = dict(params.multi_items()) if params is not None else {}
    values[key] = _required_text(value, name)
    return httpx.QueryParams(values)


def _with_raw_param(
    params: httpx.QueryParams | None,
    key: str,
    value: str,
) -> httpx.QueryParams:
    values = dict(params.multi_items()) if params is not None else {}
    values[key] = value
    return httpx.QueryParams(values)


def _dataset_item_body(item: DatasetItemInput) -> dict[str, object]:
    body: dict[str, object] = {"input": _mutable_mapping(item.input)}
    if item.external_id is not None:
        body["external_id"] = _required_text(item.external_id, "external ID")
    if item.output is not None:
        body["output"] = item.output
    if item.expected_output is not None:
        body["expected_output"] = item.expected_output
    if item.context:
        body["context"] = [
            {
                "id": _required_text(chunk.id, "chunk ID"),
                "text": _required_text(chunk.text, "chunk text"),
            }
            for chunk in item.context
        ]
    if item.metadata:
        body["metadata"] = _mutable_mapping(item.metadata)
    return body


def _scorer(value: str) -> str:
    if value not in {"groundedness", "correctness"}:
        raise AssayConfigurationError("scorer must be groundedness or correctness")
    return value


def _scorers(values: tuple[str, ...]) -> list[str]:
    if not values or len(set(values)) != len(values):
        raise AssayConfigurationError("scorers must be non-empty and unique")
    return [_scorer(value) for value in values]


def _threshold(value: float) -> float:
    if isinstance(value, bool) or not math.isfinite(value) or not 0 <= value <= 1:
        raise AssayConfigurationError("threshold must be a finite number between 0 and 1")
    return float(value)


def _query_timestamp(value: datetime, name: str) -> str:
    if value.tzinfo is None or value.utcoffset() is None:
        raise AssayConfigurationError(f"{name} must include a timezone")
    return value.isoformat()


def _unique_texts(values: tuple[str, ...], name: str) -> list[str]:
    validated = [_required_text(value, name) for value in values]
    if not validated or len(set(validated)) != len(validated):
        raise AssayConfigurationError(f"{name} must be non-empty and unique")
    return validated


def _iter_pages(fetch: Callable[[str | None], Page[T]]) -> Iterator[T]:
    cursor: str | None = None
    seen: set[str] = set()
    while True:
        page = fetch(cursor)
        yield from page.items
        if page.next_cursor is None:
            return
        if page.next_cursor in seen:
            raise AssayProtocolError("paginated response repeated a cursor")
        seen.add(page.next_cursor)
        cursor = page.next_cursor


def _polling_values(
    timeout: float | None,
    poll_interval: float,
    transient_errors: int,
) -> None:
    if timeout is not None and (
        isinstance(timeout, bool) or not math.isfinite(timeout) or timeout <= 0
    ):
        raise AssayConfigurationError("wait timeout must be a positive finite number")
    if isinstance(poll_interval, bool) or not math.isfinite(poll_interval) or poll_interval <= 0:
        raise AssayConfigurationError("poll interval must be a positive finite number")
    if (
        isinstance(transient_errors, bool)
        or not isinstance(transient_errors, int)
        or transient_errors < 0
    ):
        raise AssayConfigurationError("transient errors must be a non-negative integer")


def _poll_delay(
    deadline: float | None,
    poll_interval: float,
    now: float,
    run_id: str,
) -> float:
    if deadline is None:
        return poll_interval
    remaining = deadline - now
    if remaining <= 0:
        raise AssayTimeoutError(f"wait for run {run_id}: timed out")
    return min(poll_interval, remaining)
