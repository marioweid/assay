"""Command-line interface for Assay management and evaluation workflows."""

from __future__ import annotations

import argparse
import json
import math
import os
import sys
from collections.abc import Callable, Mapping, Sequence
from dataclasses import fields, is_dataclass
from datetime import datetime
from pathlib import Path
from typing import TextIO, cast

import httpx

from assay.client import Client
from assay.exceptions import AssayConfigurationError, AssayError
from assay.models import EvalRun, JSONValue, ResponseMapping, TargetEndpoint

Command = Callable[[argparse.Namespace, Client], int]
SCORERS = ("groundedness", "correctness")


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
    return parser


def _add_project_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    create = commands.add_parser("create")
    create.add_argument("name")
    create.set_defaults(execute=_projects_create)
    commands.add_parser("list").set_defaults(execute=_projects_list)


def _add_key_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    create = commands.add_parser("create")
    create.add_argument("--project", required=True)
    create.add_argument("--name", default="cli")
    create.set_defaults(execute=_keys_create)


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
    endpoint = commands.add_parser("set-endpoint")
    endpoint.add_argument("application")
    endpoint.add_argument("--file", required=True, type=Path)
    endpoint.set_defaults(execute=_apps_set_endpoint)


def _add_dataset_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    import_command = commands.add_parser("import")
    import_command.add_argument("application")
    import_command.add_argument("--file", required=True, type=Path)
    import_command.add_argument("--name")
    import_command.add_argument("--batch-size", type=int, default=1000)
    import_command.set_defaults(execute=_datasets_import)


def _add_scorer_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    set_command = commands.add_parser("set")
    set_command.add_argument("application")
    set_command.add_argument("scorer", choices=SCORERS)
    set_command.add_argument("--threshold", required=True, type=_threshold)
    set_command.set_defaults(execute=_scorers_set)


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
    watch = commands.add_parser("watch")
    watch.add_argument("run_id")
    watch.add_argument("--gate", action="append", default=[], type=_gate)
    watch.add_argument("--timeout", type=float)
    watch.add_argument("--poll-interval", type=float, default=1.0)
    watch.set_defaults(execute=_runs_watch)


def _add_trace_commands(parser: argparse.ArgumentParser) -> None:
    commands = parser.add_subparsers(required=True)
    list_command = commands.add_parser("list")
    list_command.add_argument("application")
    list_command.add_argument("--status")
    list_command.set_defaults(execute=_traces_list)
    get = commands.add_parser("get")
    get.add_argument("trace_id")
    get.set_defaults(execute=_traces_get)
    score = commands.add_parser("score")
    score.add_argument("--scorer", action="append", choices=SCORERS, required=True)
    score.add_argument("trace_ids", nargs="+")
    score.set_defaults(execute=_traces_score)


def _projects_create(args: argparse.Namespace, client: Client) -> int:
    return _success(client.projects.create(cast(str, args.name)))


def _projects_list(_: argparse.Namespace, client: Client) -> int:
    return _success(client.projects.list())


def _keys_create(args: argparse.Namespace, client: Client) -> int:
    return _success(client.keys.create(cast(str, args.project), cast(str, args.name)))


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


def _apps_set_endpoint(args: argparse.Namespace, client: Client) -> int:
    endpoint = _load_endpoint(cast(Path, args.file))
    return _success(client.applications.set_endpoint(cast(str, args.application), endpoint))


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
    return _success(
        client.scorers.set(
            cast(str, args.application),
            cast(str, args.scorer),
            threshold=cast(float, args.threshold),
        )
    )


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


def _success(value: object) -> int:
    _write_json(value, sys.stdout)
    return 0


def _load_endpoint(path: Path) -> TargetEndpoint:
    try:
        payload = cast(object, json.loads(path.read_text(encoding="utf-8")))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise AssayConfigurationError(f"read endpoint configuration {path}: {error}") from error
    data = _object(payload, "endpoint configuration")
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
