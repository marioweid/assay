"""Command-line interface for Assay management and evaluation workflows."""

from __future__ import annotations

import argparse
import json
import math
import os
import sys
from collections.abc import Callable, Iterable, Mapping, Sequence
from dataclasses import fields, is_dataclass
from datetime import datetime
from pathlib import Path
from typing import TextIO, TypedDict, cast

import httpx

from assay.client import Client
from assay.exceptions import AssayConfigurationError, AssayError
from assay.models import (
    Chunk,
    DatasetItemInput,
    EvalRun,
    JSONValue,
    JudgeConfig,
    ResponseMapping,
    TargetEndpoint,
)

Command = Callable[[argparse.Namespace, Client], int]
SCORERS = ("groundedness", "correctness")


class ApplicationUpdate(TypedDict, total=False):
    """Validated fields accepted by the application PATCH command."""

    name: str
    slug: str
    config: Mapping[str, JSONValue]
    auto_score_scorers: tuple[str, ...]


def main(
    argv: Sequence[str] | None = None,
    *,
    _http_client: httpx.Client | None = None,
) -> int:
    """Run the Assay command-line interface and return its process exit code."""
    parser = _build_parser()
    args = parser.parse_args(argv)
    try:
        endpoint = _value(cast(str | None, args.endpoint), "ASSAY_ENDPOINT", "endpoint")
        with Client(
            endpoint,
            api_key=os.getenv("ASSAY_API_KEY"),
            admin_token=os.getenv("ASSAY_ADMIN_TOKEN"),
            _http_client=_http_client,
        ) as client:
            execute = cast(Command, args.execute)
            return execute(args, client)
    except AssayError as error:
        print(f"assay: error: {error}", file=sys.stderr)
        return 2


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="assay")
    parser.add_argument("--endpoint")
    commands = parser.add_subparsers(required=True)
    _add_project_commands(commands.add_parser("projects"))
    _add_key_commands(commands.add_parser("keys"))
    _add_application_commands(commands.add_parser("apps"))
    _add_dataset_commands(commands.add_parser("datasets"))
    _add_scorer_commands(commands.add_parser("scorers"))
    _add_run_commands(commands.add_parser("run"))
    _add_trace_commands(commands.add_parser("traces"))
    _add_score_commands(commands.add_parser("scores"))
    metrics = commands.add_parser("metrics")
    _add_analytics_filters(metrics)
    metrics.set_defaults(execute=_metrics)
    return parser


def _add_analytics_filters(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("application")
    parser.add_argument("--scorer", choices=SCORERS)
    parser.add_argument("--start", type=datetime.fromisoformat)
    parser.add_argument("--end", type=datetime.fromisoformat)


def _add_score_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    for name in ("list", "export"):
        command = commands.add_parser(name)
        _add_analytics_filters(command)
        command.add_argument("--failed", action="store_true")
        command.add_argument("--format", choices=("jsonl",), default="jsonl")
        command.set_defaults(execute=_scores)


def _scores(args: argparse.Namespace, client: Client) -> int:
    for score in client.scores.iter_all(
        cast(str, args.application),
        start=cast(datetime | None, args.start),
        end=cast(datetime | None, args.end),
        scorer=cast(str | None, args.scorer),
        passed=False if args.failed else None,
    ):
        print(json.dumps(_json_value(score), ensure_ascii=False))
    return 0


def _metrics(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.metrics.list(
            cast(str, args.application),
            start=cast(datetime | None, args.start),
            end=cast(datetime | None, args.end),
            scorer=cast(str | None, args.scorer),
        )
    )


def _add_project_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    create = commands.add_parser("create")
    create.add_argument("name")
    create.set_defaults(execute=_projects_create)
    commands.add_parser("list").set_defaults(execute=_projects_list)
    get = commands.add_parser("get")
    get.add_argument("project")
    get.set_defaults(execute=_projects_get)
    update = commands.add_parser("update")
    update.add_argument("project")
    update.add_argument("--name")
    judges = update.add_mutually_exclusive_group()
    judges.add_argument("--judge-config-file", type=Path)
    judges.add_argument("--clear-judge-config", action="store_true")
    update.set_defaults(execute=_projects_update)
    delete = commands.add_parser("delete")
    delete.add_argument("project")
    delete.add_argument("--yes", action="store_true", required=True)
    delete.set_defaults(execute=_projects_delete)


def _add_key_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    create = commands.add_parser("create")
    create.add_argument("--project", required=True)
    create.add_argument("--name", default="cli")
    create.set_defaults(execute=_keys_create)
    list_command = commands.add_parser("list")
    list_command.add_argument("project")
    list_command.set_defaults(execute=_keys_list)
    revoke = commands.add_parser("revoke")
    revoke.add_argument("project")
    revoke.add_argument("key_id")
    revoke.add_argument("--yes", action="store_true", required=True)
    revoke.set_defaults(execute=_keys_revoke)


def _add_application_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    create = commands.add_parser("create")
    create.add_argument("--project", required=True)
    create.add_argument("--name", required=True)
    create.add_argument("--slug", required=True)
    create.set_defaults(execute=_apps_create)
    list_command = commands.add_parser("list")
    list_command.add_argument("--project")
    list_command.set_defaults(execute=_apps_list)
    get = commands.add_parser("get")
    get.add_argument("application")
    get.set_defaults(execute=_apps_get)
    update = commands.add_parser("update")
    update.add_argument("application")
    update.add_argument("--file", required=True, type=Path)
    update.set_defaults(execute=_apps_update)
    delete = commands.add_parser("delete")
    delete.add_argument("application")
    delete.add_argument("--yes", action="store_true", required=True)
    delete.set_defaults(execute=_apps_delete)
    endpoint = commands.add_parser("set-endpoint")
    endpoint.add_argument("application")
    endpoint.add_argument("--file", required=True, type=Path)
    endpoint.set_defaults(execute=_apps_set_endpoint)
    clear_endpoint = commands.add_parser("clear-endpoint")
    clear_endpoint.add_argument("application")
    clear_endpoint.add_argument("--yes", action="store_true", required=True)
    clear_endpoint.set_defaults(execute=_apps_clear_endpoint)


def _add_dataset_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    list_command = commands.add_parser("list")
    list_command.add_argument("application")
    list_command.set_defaults(execute=_datasets_list)
    get = commands.add_parser("get")
    get.add_argument("dataset")
    get.set_defaults(execute=_datasets_get)
    update = commands.add_parser("update")
    update.add_argument("dataset")
    update.add_argument("--name")
    descriptions = update.add_mutually_exclusive_group()
    descriptions.add_argument("--description")
    descriptions.add_argument("--clear-description", action="store_true")
    update.set_defaults(execute=_datasets_update)
    delete = commands.add_parser("delete")
    delete.add_argument("dataset")
    delete.add_argument("--yes", action="store_true", required=True)
    delete.set_defaults(execute=_datasets_delete)
    export = commands.add_parser("export")
    export.add_argument("dataset")
    export.add_argument("--format", choices=("jsonl",), default="jsonl")
    export.set_defaults(execute=_datasets_export)
    item = commands.add_parser("items").add_subparsers(required=True)
    item_get = item.add_parser("get")
    item_get.add_argument("dataset")
    item_get.add_argument("item_id")
    item_get.set_defaults(execute=_dataset_items_get)
    item_list = item.add_parser("list")
    item_list.add_argument("dataset")
    item_list.set_defaults(execute=_dataset_items_list)
    add = item.add_parser("add")
    add.add_argument("dataset")
    add.add_argument("--file", required=True, type=Path)
    add.set_defaults(execute=_dataset_items_add)
    replace = item.add_parser("replace")
    replace.add_argument("dataset")
    replace.add_argument("item_id")
    replace.add_argument("--file", required=True, type=Path)
    replace.set_defaults(execute=_dataset_items_replace)
    item_delete = item.add_parser("delete")
    item_delete.add_argument("dataset")
    item_delete.add_argument("item_id")
    item_delete.add_argument("--yes", action="store_true", required=True)
    item_delete.set_defaults(execute=_dataset_items_delete)
    import_command = commands.add_parser("import")
    import_command.add_argument("application")
    import_command.add_argument("--file", required=True, type=Path)
    import_command.add_argument("--name")
    import_command.add_argument("--batch-size", type=int, default=1000)
    import_command.set_defaults(execute=_datasets_import)
    regression = commands.add_parser("from-trace")
    regression.add_argument("dataset")
    regression.add_argument("trace_id")
    regression.add_argument("--scorer", choices=SCORERS, required=True)
    regression.add_argument("--expected-output")
    regression.set_defaults(execute=_datasets_from_trace)


def _add_scorer_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    set_command = commands.add_parser("set")
    set_command.add_argument("application")
    set_command.add_argument("scorer", choices=SCORERS)
    set_command.add_argument("--threshold", type=_threshold)
    enabled = set_command.add_mutually_exclusive_group()
    enabled.add_argument("--enabled", action="store_true")
    enabled.add_argument("--disabled", action="store_true")
    set_command.add_argument("--judge-config-file", type=Path)
    set_command.add_argument("--prompt-template-id")
    set_command.set_defaults(execute=_scorers_set)
    list_command = commands.add_parser("list")
    list_command.add_argument("application")
    list_command.set_defaults(execute=_scorers_list)


def _add_run_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    create = commands.add_parser("create")
    create.add_argument("application")
    create.add_argument("--dataset", required=True)
    create.add_argument("--name", default="eval")
    create.add_argument("--scorers", required=True, type=_scorer_list)
    create.add_argument(
        "--mode",
        choices=("score_existing", "generate_then_score"),
        default="score_existing",
    )
    create.set_defaults(execute=_runs_create)
    list_command = commands.add_parser("list")
    list_command.add_argument("application")
    list_command.set_defaults(execute=_runs_list)
    get = commands.add_parser("get")
    get.add_argument("run_id")
    get.set_defaults(execute=_runs_get)
    items = commands.add_parser("items")
    items.add_argument("run_id")
    items.set_defaults(execute=_runs_items)
    scores = commands.add_parser("scores")
    scores.add_argument("run_id")
    scores.set_defaults(execute=_runs_scores)
    export = commands.add_parser("export")
    export.add_argument("run_id")
    export.add_argument("--format", choices=("jsonl",), default="jsonl")
    export.set_defaults(execute=_runs_export)
    watch = commands.add_parser("watch")
    watch.add_argument("run_id")
    watch.add_argument("--gate", action="append", default=[], type=_gate)
    watch.add_argument("--timeout", type=float)
    watch.add_argument("--poll-interval", type=float, default=1.0)
    watch.set_defaults(execute=_runs_watch)
    item = commands.add_parser("item")
    item.add_argument("run_id")
    item.add_argument("item_id")
    item.set_defaults(execute=_runs_item)
    compare = commands.add_parser("compare")
    compare.add_argument("run_id")
    compare.add_argument("other_run_id")
    compare.add_argument("--scorer", choices=SCORERS, required=True)
    compare.add_argument("--limit", type=int, default=100)
    compare.add_argument("--cursor")
    compare.set_defaults(execute=_runs_compare)
    cancel = commands.add_parser("cancel")
    cancel.add_argument("run_id")
    cancel.set_defaults(execute=_runs_cancel)
    delete = commands.add_parser("delete")
    delete.add_argument("run_id")
    delete.add_argument("--yes", action="store_true", required=True)
    delete.set_defaults(execute=_runs_delete)


def _add_trace_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    list_command = commands.add_parser("list")
    list_command.add_argument("application")
    list_command.add_argument("--status")
    list_command.add_argument("--query")
    list_command.add_argument("--scorer", choices=SCORERS)
    pass_filter = list_command.add_mutually_exclusive_group()
    pass_filter.add_argument("--passed", action="store_true")
    pass_filter.add_argument("--failed", action="store_true")
    list_command.set_defaults(execute=_traces_list)
    get = commands.add_parser("get")
    get.add_argument("trace_id")
    get.set_defaults(execute=_traces_get)
    score = commands.add_parser("score")
    score.add_argument("--scorer", action="append", choices=SCORERS, required=True)
    score.add_argument("trace_ids", nargs="+")
    score.set_defaults(execute=_traces_score)
    reference = commands.add_parser("reference")
    reference.add_argument("trace_id")
    reference.add_argument("--file", required=True, type=Path)
    reference.set_defaults(execute=_traces_reference)
    eligibility = commands.add_parser("eligibility")
    eligibility.add_argument("trace_id")
    eligibility.set_defaults(execute=_traces_eligibility)
    delete = commands.add_parser("delete")
    delete.add_argument("trace_id")
    delete.add_argument("--yes", action="store_true", required=True)
    delete.set_defaults(execute=_traces_delete)


def _projects_create(args: argparse.Namespace, client: Client) -> int:
    return _success(client.projects.create(cast(str, args.name)))


def _projects_list(_: argparse.Namespace, client: Client) -> int:
    return _success(client.projects.list())


def _projects_get(args: argparse.Namespace, client: Client) -> int:
    return _success(client.projects.get(cast(str, args.project)))


def _projects_update(args: argparse.Namespace, client: Client) -> int:
    judge_path = cast(Path | None, args.judge_config_file)
    judge = _load_judge_config(judge_path) if judge_path is not None else None
    return _success(
        client.projects.update(
            cast(str, args.project),
            name=cast(str | None, args.name),
            judge_config=judge,
            clear_judge_config=cast(bool, args.clear_judge_config),
        )
    )


def _projects_delete(args: argparse.Namespace, client: Client) -> int:
    project_id = cast(str, args.project)
    client.projects.delete(project_id)
    return _success({"deleted": project_id})


def _keys_create(args: argparse.Namespace, client: Client) -> int:
    return _success(client.keys.create(cast(str, args.project), cast(str, args.name)))


def _keys_list(args: argparse.Namespace, client: Client) -> int:
    return _success(client.keys.list(cast(str, args.project)))


def _keys_revoke(args: argparse.Namespace, client: Client) -> int:
    key_id = cast(str, args.key_id)
    client.keys.revoke(cast(str, args.project), key_id)
    return _success({"revoked": key_id})


def _apps_create(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.applications.create(
            cast(str, args.project),
            cast(str, args.name),
            cast(str, args.slug),
        )
    )


def _apps_list(args: argparse.Namespace, client: Client) -> int:
    return _success(client.applications.list(cast(str | None, args.project)))


def _apps_get(args: argparse.Namespace, client: Client) -> int:
    return _success(client.applications.get(cast(str, args.application)))


def _apps_update(args: argparse.Namespace, client: Client) -> int:
    data = _load_application_update(cast(Path, args.file))
    return _success(client.applications.update(cast(str, args.application), **data))


def _apps_delete(args: argparse.Namespace, client: Client) -> int:
    application_id = cast(str, args.application)
    client.applications.delete(application_id)
    return _success({"deleted": application_id})


def _apps_set_endpoint(args: argparse.Namespace, client: Client) -> int:
    endpoint = _load_endpoint(cast(Path, args.file))
    return _success(client.applications.set_endpoint(cast(str, args.application), endpoint))


def _apps_clear_endpoint(args: argparse.Namespace, client: Client) -> int:
    return _success(client.applications.clear_endpoint(cast(str, args.application)))


def _datasets_list(args: argparse.Namespace, client: Client) -> int:
    return _success(client.datasets.list(application_id=cast(str, args.application)))


def _datasets_get(args: argparse.Namespace, client: Client) -> int:
    return _success(client.datasets.get(cast(str, args.dataset)))


def _datasets_update(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.datasets.update(
            cast(str, args.dataset),
            name=cast(str | None, args.name),
            description=cast(str | None, args.description),
            clear_description=cast(bool, args.clear_description),
        )
    )


def _datasets_delete(args: argparse.Namespace, client: Client) -> int:
    dataset_id = cast(str, args.dataset)
    client.datasets.delete(dataset_id)
    return _success({"deleted": dataset_id})


def _datasets_export(args: argparse.Namespace, client: Client) -> int:
    return _json_lines(client.datasets.iter_all_items(cast(str, args.dataset)))


def _dataset_items_get(args: argparse.Namespace, client: Client) -> int:
    return _success(client.datasets.get_item(cast(str, args.dataset), cast(str, args.item_id)))


def _dataset_items_list(args: argparse.Namespace, client: Client) -> int:
    return _success(client.datasets.list_items(cast(str, args.dataset)))


def _dataset_items_add(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.datasets.create_items(
            cast(str, args.dataset),
            (_load_dataset_item(cast(Path, args.file)),),
        )
    )


def _dataset_items_replace(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.datasets.replace_item(
            cast(str, args.dataset),
            cast(str, args.item_id),
            item=_load_dataset_item(cast(Path, args.file)),
        )
    )


def _dataset_items_delete(args: argparse.Namespace, client: Client) -> int:
    dataset_id = cast(str, args.dataset)
    item_id = cast(str, args.item_id)
    client.datasets.delete_item(dataset_id, item_id)
    return _success({"deleted": item_id})


def _datasets_from_trace(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.datasets.from_trace(
            cast(str, args.dataset),
            cast(str, args.trace_id),
            scorer=cast(str, args.scorer),
            expected_output=cast(str | None, args.expected_output),
        )
    )


def _datasets_import(args: argparse.Namespace, client: Client) -> int:
    path = cast(Path, args.file)
    name = cast(str | None, args.name) or path.stem
    dataset = client.datasets.ensure(cast(str, args.application), name)
    result = client.datasets.import_file(
        dataset.id,
        path,
        batch_size=cast(int, args.batch_size),
    )
    return _success(result)


def _scorers_set(args: argparse.Namespace, client: Client) -> int:
    judge_path = cast(Path | None, args.judge_config_file)
    enabled = True if args.enabled else False if args.disabled else None
    return _success(
        client.scorers.set(
            cast(str, args.application),
            cast(str, args.scorer),
            enabled=enabled,
            threshold=cast(float | None, args.threshold),
            judge_config=_load_judge_config(judge_path) if judge_path is not None else None,
            prompt_template_id=cast(str | None, args.prompt_template_id),
        )
    )


def _scorers_list(args: argparse.Namespace, client: Client) -> int:
    return _success(client.scorers.list(cast(str, args.application)))


def _runs_create(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.runs.create(
            cast(str, args.application),
            cast(str, args.dataset),
            cast(str, args.name),
            mode=cast(str, args.mode),
            scorers=cast(tuple[str, ...], args.scorers),
        )
    )


def _runs_list(args: argparse.Namespace, client: Client) -> int:
    return _success(client.runs.list(application_id=cast(str, args.application)))


def _runs_items(args: argparse.Namespace, client: Client) -> int:
    return _success(client.runs.list_items(cast(str, args.run_id)))


def _runs_scores(args: argparse.Namespace, client: Client) -> int:
    return _success(client.runs.list_scores(cast(str, args.run_id)))


def _runs_export(args: argparse.Namespace, client: Client) -> int:
    return _json_lines(client.runs.iter_all_items(cast(str, args.run_id)))


def _runs_get(args: argparse.Namespace, client: Client) -> int:
    return _success(client.runs.get(cast(str, args.run_id)))


def _runs_item(args: argparse.Namespace, client: Client) -> int:
    return _success(client.runs.get_item(cast(str, args.run_id), cast(str, args.item_id)))


def _runs_compare(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.runs.compare(
            cast(str, args.run_id),
            cast(str, args.other_run_id),
            scorer=cast(str, args.scorer),
            limit=cast(int, args.limit),
            cursor=cast(str | None, args.cursor),
        )
    )


def _runs_cancel(args: argparse.Namespace, client: Client) -> int:
    return _success(client.runs.cancel(cast(str, args.run_id)))


def _runs_delete(args: argparse.Namespace, client: Client) -> int:
    run_id = cast(str, args.run_id)
    client.runs.delete(run_id)
    return _success({"deleted": run_id})


def _runs_watch(args: argparse.Namespace, client: Client) -> int:
    previous: object = None

    def report(run: EvalRun) -> None:
        nonlocal previous
        summary = _run_summary(run)
        if summary != previous:
            _write_json(summary, sys.stdout)
            previous = summary

    run = client.runs.wait(
        cast(str, args.run_id),
        timeout=cast(float | None, args.timeout),
        poll_interval=cast(float, args.poll_interval),
        on_update=report,
    )
    if run.status != "succeeded":
        _write_json({"gate": "failed", "run_status": run.status}, sys.stdout)
        return 1
    failures = _gate_failures(run, cast(list[tuple[str, float]], args.gate))
    if failures:
        _write_json({"gate": "failed", "failures": failures}, sys.stdout)
        return 1
    if cast(list[tuple[str, float]], args.gate):
        _write_json({"gate": "passed"}, sys.stdout)
    return 0


def _traces_list(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.traces.list(
            application_id=cast(str, args.application),
            status=cast(str | None, args.status),
            q=cast(str | None, args.query),
            scorer=cast(str | None, args.scorer),
            passed=True if args.passed else False if args.failed else None,
        )
    )


def _traces_get(args: argparse.Namespace, client: Client) -> int:
    return _success(client.traces.get(cast(str, args.trace_id)))


def _traces_score(args: argparse.Namespace, client: Client) -> int:
    return _success(
        client.traces.score(
            tuple(cast(list[str], args.trace_ids)),
            tuple(cast(list[str], args.scorer)),
        )
    )


def _traces_reference(args: argparse.Namespace, client: Client) -> int:
    path = cast(Path, args.file)
    try:
        reference = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as error:
        raise AssayConfigurationError(f"read trace reference {path}: {error}") from error
    return _success(client.traces.set_reference(cast(str, args.trace_id), reference))


def _traces_eligibility(args: argparse.Namespace, client: Client) -> int:
    return _success(client.traces.eligibility(cast(str, args.trace_id)))


def _traces_delete(args: argparse.Namespace, client: Client) -> int:
    trace_id = cast(str, args.trace_id)
    client.traces.delete(trace_id)
    return _success({"deleted": trace_id})


def _success(value: object) -> int:
    _write_json(value, sys.stdout)
    return 0


def _json_lines(values: Iterable[object]) -> int:
    for value in values:
        _write_json(value, sys.stdout)
    return 0


def _load_application_update(path: Path) -> ApplicationUpdate:
    data = _load_json_object(path, "application update")
    unknown = data.keys() - {"name", "slug", "config", "auto_score_scorers"}
    if unknown:
        raise AssayConfigurationError("application update contains an unknown field")
    update = ApplicationUpdate()
    if "name" in data:
        update["name"] = _text(data["name"], "name")
    if "slug" in data:
        update["slug"] = _text(data["slug"], "slug")
    if "config" in data:
        update["config"] = cast(Mapping[str, JSONValue], _object(data["config"], "config"))
    if "auto_score_scorers" in data:
        update["auto_score_scorers"] = _scorer_values(data["auto_score_scorers"])
    return update


def _load_judge_config(path: Path) -> JudgeConfig:
    data = _load_json_object(path, "judge configuration")
    unknown = data.keys() - {"base_url", "model", "api_key"}
    if unknown:
        raise AssayConfigurationError("judge configuration contains an unknown field")
    return JudgeConfig(
        base_url=_text(data.get("base_url"), "base_url"),
        model=_text(data.get("model"), "model"),
        api_key=_optional_dataset_text(data.get("api_key"), "api_key"),
    )


def _load_dataset_item(path: Path) -> DatasetItemInput:
    data = _load_json_object(path, "dataset item")
    input_value = _object(data.get("input"), "input")
    metadata = _object(data.get("metadata", {}), "metadata")
    context_value = data.get("context", [])
    if not isinstance(context_value, list):
        raise AssayConfigurationError("context must be a JSON array")
    chunks = tuple(_dataset_chunk(value) for value in context_value)
    return DatasetItemInput(
        input=cast(Mapping[str, JSONValue], input_value),
        external_id=_optional_dataset_text(data.get("external_id"), "external_id"),
        output=_optional_dataset_text(data.get("output"), "output"),
        expected_output=_optional_dataset_text(data.get("expected_output"), "expected_output"),
        context=chunks,
        metadata=cast(Mapping[str, JSONValue], metadata),
    )


def _optional_dataset_text(value: object, label: str) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str):
        raise AssayConfigurationError(f"{label} must be a string or null")
    return value


def _dataset_chunk(value: object) -> Chunk:
    data = _object(value, "context item")
    return Chunk(
        id=_text(data.get("id"), "context.id"), text=_text(data.get("text"), "context.text")
    )


def _load_endpoint(path: Path) -> TargetEndpoint:
    data = _load_json_object(path, "endpoint configuration")
    response_data = _object(data.get("response_mapping"), "response_mapping")
    timeout_ms = data.get("timeout_ms", 30_000)
    if isinstance(timeout_ms, bool) or not isinstance(timeout_ms, int) or timeout_ms <= 0:
        raise AssayConfigurationError("timeout_ms must be a positive integer")
    return TargetEndpoint(
        url=_text(data.get("url"), "url"),
        method=_text(data.get("method", "POST"), "method"),
        headers=_string_mapping(data.get("headers", {}), "headers"),
        request_template=cast(
            Mapping[str, JSONValue],
            _object(data.get("request_template", {}), "request_template"),
        ),
        response_mapping=ResponseMapping(
            output=_text(response_data.get("output"), "response_mapping.output"),
            context=_optional_text(
                response_data.get("context"),
                "response_mapping.context",
            ),
        ),
        timeout_ms=timeout_ms,
        secret=_optional_text(data.get("secret"), "secret"),
    )


def _load_json_object(path: Path, label: str) -> dict[str, object]:
    try:
        payload = cast(object, json.loads(path.read_text(encoding="utf-8")))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise AssayConfigurationError(f"read {label} {path}: {error}") from error
    return _object(payload, label)


def _object(value: object, label: str) -> dict[str, object]:
    if not isinstance(value, dict) or any(not isinstance(key, str) for key in value):
        raise AssayConfigurationError(f"{label} must be a JSON object")
    return cast(dict[str, object], value)


def _text(value: object, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise AssayConfigurationError(f"{label} must be a non-blank string")
    return value.strip()


def _optional_text(value: object, label: str) -> str | None:
    if value is None:
        return None
    return _text(value, label)


def _string_mapping(value: object, label: str) -> dict[str, str]:
    values = _object(value, label)
    if any(not isinstance(item, str) for item in values.values()):
        raise AssayConfigurationError(f"{label} values must be strings")
    return cast(dict[str, str], values)


def _scorer_values(value: object) -> tuple[str, ...]:
    if not isinstance(value, list) or any(not isinstance(item, str) for item in value):
        raise AssayConfigurationError("auto_score_scorers must be an array of scorer names")
    scorers = tuple(cast(list[str], value))
    if any(scorer not in SCORERS for scorer in scorers) or len(set(scorers)) != len(scorers):
        raise AssayConfigurationError("auto_score_scorers must be known and unique")
    return scorers


def _threshold(value: str) -> float:
    try:
        threshold = float(value)
    except ValueError as error:
        raise argparse.ArgumentTypeError("threshold must be a number between 0 and 1") from error
    if not math.isfinite(threshold) or not 0 <= threshold <= 1:
        raise argparse.ArgumentTypeError("threshold must be a number between 0 and 1")
    return threshold


def _scorer_list(value: str) -> tuple[str, ...]:
    scorers = tuple(part.strip() for part in value.split(",") if part.strip())
    if not scorers or len(set(scorers)) != len(scorers):
        raise argparse.ArgumentTypeError("scorers must be non-empty and unique")
    unknown = tuple(scorer for scorer in scorers if scorer not in SCORERS)
    if unknown:
        raise argparse.ArgumentTypeError(f"unknown scorer: {unknown[0]}")
    return scorers


def _gate(value: str) -> tuple[str, float]:
    scorer, separator, threshold = value.partition(":")
    if not separator or scorer not in SCORERS:
        raise argparse.ArgumentTypeError("gate must be scorer:threshold")
    return scorer, _threshold(threshold)


def _run_summary(run: EvalRun) -> dict[str, object]:
    return {
        "id": run.id,
        "status": run.status,
        "total_items": run.total_items,
        "succeeded_items": run.succeeded_items,
        "failed_items": run.failed_items,
        "canceled_items": run.canceled_items,
        "aggregates": run.aggregates,
    }


def _gate_failures(run: EvalRun, gates: list[tuple[str, float]]) -> list[dict[str, object]]:
    failures: list[dict[str, object]] = []
    for scorer, required in gates:
        aggregate = run.aggregates.get(scorer)
        actual = aggregate.pass_rate if aggregate is not None else None
        if actual is None or actual < required:
            failures.append({"scorer": scorer, "required": required, "actual": actual})
    return failures


def _value(explicit: str | None, environment: str, label: str) -> str:
    value = explicit if explicit is not None else os.getenv(environment)
    if value is None or not value.strip():
        raise AssayConfigurationError(f"{label} is required")
    return value.strip()


def _write_json(value: object, stream: TextIO) -> None:
    json.dump(_json_value(value), stream, sort_keys=True, separators=(",", ":"))
    stream.write("\n")


def _json_value(value: object) -> object:
    if isinstance(value, datetime):
        return value.isoformat()
    if is_dataclass(value) and not isinstance(value, type):
        return {field.name: _json_value(getattr(value, field.name)) for field in fields(value)}
    if isinstance(value, Mapping):
        return {str(key): _json_value(item) for key, item in value.items()}
    if isinstance(value, Sequence) and not isinstance(value, str | bytes | bytearray):
        return [_json_value(item) for item in value]
    return value
