# assay-sdk

The Python package for [Assay](https://github.com/marioweid/assay), a self-hosted LLM
tracing and evaluation platform.

```bash
uv add assay-sdk
```

The distribution is named `assay-sdk` and imported as `assay`. The checkout contains the current
SDK workflow; until its next release, use `uv run --project clients/python/assay ...` for the
structured-message, typed lifecycle, and CLI examples in the Linux and PowerShell quickstarts.

## Tracing

Content capture is off by default. Enable it explicitly when the traced input and output may be
sent to Assay:

```python
import assay

assay.init(
    endpoint="http://localhost:8080",
    api_key="asy_...",
    application="support-bot",
    capture=True,
)


@assay.trace
def answer(question: str) -> str:
    return "Assay evaluates AI systems."
```

Use `assay.span(...)` for explicit spans and call `set_input`, `set_output`, `set_context`, or
`set_reference` on the active span. Structured conversations use the pinned OpenTelemetry GenAI
message shape directly:

```python
with assay.span("chat", scorable=True) as current:
    current.set_messages(
        input=[{"role": "user", "parts": [{"type": "text", "content": "What is Assay?"}]}],
        output=[{"role": "assistant", "parts": [{"type": "text", "content": "An eval tool."}]}],
    )
    trace_id = current.trace_id
```

`set_messages` is explicit even when `capture=False`; redact captured content with its `redact`
callback. It validates complete text/client-tool messages and rejects oversized JSON rather than
truncating it. The decorator captures function arguments and return values, not provider SDK traffic
or generator/stream consumption. Call `assay.flush()` before a short-lived process exits and
`assay.shutdown()` when the tracing lifecycle ends.

## API Client

```python
import assay

with assay.Client("http://localhost:8080", admin_token="...") as client:
    applications = client.applications.list()
```

Management, dataset, scorer, and run operations use an admin token. Trace inspection and scoring
prefer a project API key and fall back to an admin token. Trace deletion and scoring-eligibility
inspection always use the admin token.

Typed resources cover complete dataset-item replacement, run-item evidence and scores, paired run
comparison, trace score summaries, and scoring eligibility. Response parsing is strict: malformed
nested server data raises `AssayProtocolError` instead of returning a partial model.

## Trace-to-evaluation lifecycle

Use a project key to emit and inspect that project's traces; management, dataset, and run calls need
an admin token. Attach a reference before requesting correctness, then import a scored trace into a
dataset. An import retains its selected score evidence and rejects a repeated trace/scorer pair with
409. Replacing a dataset item is a full replacement: explicit `None` clears nullable output/reference
fields. Existing runs retain their immutable creation snapshot; a rerun uses the current dataset.
Comparisons align immutable item IDs and report matched, changed, one-sided, and unscored exclusions.

## CLI

Set `ASSAY_ENDPOINT` and the relevant credential, then use the management and evaluation commands:

```bash
assay projects list
assay projects get PROJECT_ID
assay projects update PROJECT_ID --judge-config-file judge.json
assay keys revoke PROJECT_ID KEY_ID --yes
assay apps list --project PROJECT_ID
assay apps update APP_ID --file application-patch.json
assay apps clear-endpoint APP_ID --yes
assay datasets import APPLICATION_ID --file regression.jsonl
assay datasets list APPLICATION_ID
assay scorers set APP_ID groundedness --disabled --judge-config-file judge.json
assay run create APPLICATION_ID --dataset DATASET_ID --scorers groundedness,correctness
assay run list APPLICATION_ID
assay run export RUN_ID --format jsonl
assay run watch RUN_ID --gate groundedness:0.8
assay run compare BASELINE_RUN_ID CANDIDATE_RUN_ID --scorer groundedness
assay datasets items replace DATASET_ID ITEM_ID --file item.json
assay datasets export DATASET_ID --format jsonl
assay traces list APPLICATION_ID --query answer --scorer correctness --failed
assay traces reference TRACE_ID --file reference.txt
assay traces eligibility TRACE_ID
assay traces score --scorer correctness TRACE_ID
assay scores export APPLICATION_ID --failed --format jsonl
assay datasets from-trace DATASET_ID TRACE_ID --scorer groundedness
assay metrics APPLICATION_ID --scorer groundedness
```

Commands emit JSON. Delete, key-revoke, and endpoint-clear commands require `--yes`; run cancel is
already explicit and does not. `assay run watch` returns exit code 1 when a run fails or a gate is
not met, which makes it suitable
for CI checks.

Commands are available from this checkout via `uv run assay ...`. Metrics and score export
require admin authentication and default to 30 days. `--start` and `--end` accept timezone-bearing
timestamps for ranges up to 366 days. Trace imports preserve the selected scorer's latest evidence
and reject duplicate trace/scorer pairs without overwriting existing items.

`tests/test_product_acceptance.py` and `tests/acceptance/docs-smoke.sh` run through
`tests/acceptance/run.sh` against disposable fake judge/target services; they never call a paid
provider. Set `ASSAY_ACCEPTANCE_KEEP=1` when running that script to retain the synthetic project and
stack for dashboard inspection. The optional `tests/test_live_workflow.py` instead requires
`ASSAY_LIVE_TEST_ENDPOINT` and makes real judge calls. See the runnable
[Linux](../../../docs/quickstart-linux.md) and [PowerShell](../../../docs/quickstart-powershell.md)
guides for setup and credential roles.
