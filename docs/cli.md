# CLI cookbook

[Documentation](index.md) / CLI

The CLI ships with the Python SDK. From the repository root, run:

```bash
uv run --project clients/python/assay assay --help
```

The examples below use the short `assay` entrypoint. In this checkout, prefix each with
`uv run --project clients/python/assay`. Uppercase IDs such as `APP_ID` are placeholders to replace,
not names the CLI resolves automatically. Commands emit JSON unless showing help/progress.

## Connect

For local mode, enable it independently on the server and in the CLI environment:

```bash
export ASSAY_ENDPOINT=http://localhost:8080
export ASSAY_LOCAL_MODE=true
uv run --project clients/python/assay assay projects list
```

PowerShell:

```powershell
$env:ASSAY_ENDPOINT = 'http://localhost:8080'
$env:ASSAY_LOCAL_MODE = 'true'
uv run --project clients/python/assay assay projects list
```

For token mode, set `ASSAY_LOCAL_MODE=false` and provide `ASSAY_ADMIN_TOKEN` privately.
Set `ASSAY_API_KEY` for project-scoped trace operations. Do not paste keys into command history.
An explicit `--endpoint` can override `ASSAY_ENDPOINT`.

## Create a workspace

The [quickstart bootstrap](../examples/quickstart/bootstrap.py) securely saves the one-time key and
IDs to a private file. Prefer it to copying secrets from terminal scrollback. The equivalent commands:

```text
assay projects create "Support"
assay keys create --project PROJECT_ID --name developer
assay apps create --project PROJECT_ID --name "Support Bot" --slug support-bot
assay projects list
assay apps list --project PROJECT_ID
```

**`keys create` prints the plaintext key once.** Protect/redact its output and do not include it in
screenshots or logs. To rotate, create a replacement, update your sender, verify ingestion, then:

```text
assay keys revoke PROJECT_ID KEY_ID --yes
```

## Find and inspect traces

```text
assay traces list APP_ID --query answer
assay traces list APP_ID --scorer correctness --failed
assay traces get TRACE_ID
assay traces eligibility TRACE_ID
```

The query can be an operation name, an OTel hex trace ID, or an Assay UUID. Individual trace commands
use the **Assay UUID** returned by list/get. `--failed` means a failed score when paired with a scorer,
not necessarily an execution failure. Eligibility is a management operation.

Attach a reference from a UTF-8 text file and request scoring:

```text
assay traces reference TRACE_ID --file reference.txt
assay traces score --scorer correctness TRACE_ID
```

Scoring is asynchronous. Inspect the trace's scoring tasks and resulting evidence in the UI or
`traces get`; a successful queue request does not mean a score already exists.

## Build a regression dataset

```text
assay datasets import APP_ID --file examples/quickstart/regression.jsonl --name regression
assay datasets list APP_ID
assay datasets from-trace DATASET_ID TRACE_ID --scorer groundedness
assay datasets export DATASET_ID --format jsonl
assay datasets items replace DATASET_ID ITEM_ID --file item.json
```

Imports report per-item failures; read the result rather than assuming every row succeeded.
Trace imports require persisted score evidence and reject duplicate trace/scorer pairs with 409.
Item replacement is a complete replacement, not a patch. See the [SDK guide](python-sdk.md).

## Evaluate, compare and export

```text
assay run create APP_ID --dataset DATASET_ID --name baseline --scorers groundedness,correctness
assay run list APP_ID
assay run watch RUN_ID --timeout 120
assay run export RUN_ID --format jsonl
assay run compare BASELINE_RUN_ID CANDIDATE_RUN_ID --scorer correctness
assay scores export APP_ID --failed --format jsonl
assay metrics APP_ID --scorer groundedness
```

`run create` defaults to `score_existing`. `--mode generate_then_score` calls the application's
configured target; make that choice explicitly because it can invoke external services.
Score exports/metrics require management access. Their default window is 30 days; `--start` and
`--end` accept timezone-bearing timestamps with ranges up to 366 days.

### CI gates

```text
assay run watch RUN_ID --gate groundedness:0.8 --timeout 120
```

| Exit code | Meaning |
|---|---|
| `0` | Command succeeded; for watch, the run and requested gate succeeded |
| `1` | Watched run failed or a requested gate was not met |
| `2` | CLI/configuration/API/transport error |

Do not hide nonzero exit codes in shell pipelines. In Bash automation use `set -euo pipefail`;
in PowerShell inspect `$LASTEXITCODE` after native commands. A watch timeout does not cancel the
run. Keep failed run evidence for diagnosis rather than deleting it automatically.

## Configure scorers and applications

```text
assay projects update PROJECT_ID --judge-config-file judge.json
assay scorers set APP_ID groundedness --threshold 0.8
assay apps update APP_ID --file application-patch.json
assay apps set-endpoint APP_ID --file target-endpoint.json
```

Judge files can contain credentials: keep them private and outside Git. Target endpoints generate
answers and are distinct from both the Assay endpoint and the judge. Validate the response mapping
using a small synthetic case before starting a large run.

## Destructive commands

Delete, key-revoke and endpoint-clear commands require `--yes`. Run cancellation is already an
explicit action and does not. Examples:

```text
assay run cancel RUN_ID
assay apps clear-endpoint APP_ID --yes
assay datasets delete DATASET_ID --yes
```

Deleting a dataset cascades its runs. Deleting a dataset item preserves existing run snapshots.
Back up first; see [deployment and recovery](deployment.md).

For the exact arguments supported by your installed version, use `assay COMMAND --help`.
Session reads are currently available in the Python client/API/UI, not a separate CLI sessions command.
