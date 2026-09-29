# Your first evaluation

[Documentation](index.md) / Evaluations

Tracing answers **what happened?** Evaluation asks **was the answer good?** They are separate:
you can capture and inspect traces without running a judge or spending provider tokens.

## 1. Configure a judge

Assay calls an OpenAI-compatible chat-completions endpoint. For global defaults in `.env`:

```dotenv
ASSAY_JUDGE_BASE_URL=http://host.docker.internal:11434/v1
ASSAY_JUDGE_MODEL=your-installed-judge-model
ASSAY_JUDGE_API_KEY=
```

Replace the example URL/model with your provider's actual values. A keyless local endpoint may
leave the API key blank; authenticated providers need their own key. A model that generates good
chat replies is not necessarily reliable at returning the structured JSON an evaluator needs.

After changing `.env`, run `docker compose up -d` so the server receives the new values.
Do not merely restart the old container. Configuration can also be overridden per project and
per scorer; effective scorer configuration takes precedence over project settings and global defaults.

> Inside Docker, `localhost` means the Assay container. `host.docker.internal` reaches a host service
> when configured; the repository Compose file supplies the Linux host-gateway mapping. Your model
> server must listen on an address reachable from Docker. Test reachability before sending a large run.

## 2. Know what each scorer needs

| Scorer | Question | Required evidence |
|---|---|---|
| `groundedness` | Is the answer supported by the retrieved context? | Captured answer and valid context chunks |
| `correctness` | Does the answer match the reference? | Captured answer and a nonblank reference answer |

For online trace scoring, mark exactly one intended answer span `scorable=True`, capture its input
and output, and supply context/reference on that span as appropriate. Eligibility diagnostics in
Trace detail explain missing evidence. A low score is not an execution error; inspect the rationale
and retained evidence rather than treating the numeric value as objective truth.

## 3. Evaluate the included synthetic case

Complete the quickstart first. These Bash commands reuse its `$APP_ID`, local-mode environment,
and `examples/quickstart/regression.jsonl`:

```bash
DATASET_ID=$(uv run --project clients/python/assay assay datasets import "$APP_ID" \
  --file examples/quickstart/regression.jsonl | uv run --project clients/python/assay python -c \
  'import json,sys; print(json.load(sys.stdin)["dataset_id"])')
RUN_ID=$(uv run --project clients/python/assay assay run create "$APP_ID" --dataset "$DATASET_ID" \
  --scorers groundedness,correctness | uv run --project clients/python/assay python -c \
  'import json,sys; print(json.load(sys.stdin)["id"])')
uv run --project clients/python/assay assay run watch "$RUN_ID"
```

PowerShell, reusing `$workspace` from its quickstart:

```powershell
$dataset = uv run --project clients/python/assay assay datasets import $workspace.application_id `
  --file examples/quickstart/regression.jsonl | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'Dataset import failed' }
$run = uv run --project clients/python/assay assay run create $workspace.application_id `
  --dataset $dataset.dataset_id --scorers groundedness,correctness | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'Run creation failed' }
uv run --project clients/python/assay assay run watch $run.id
```

Open the application's **Evaluations** page, select the run, and inspect each item's score,
rationale, judge metadata and evidence. `watch` exits nonzero for a failed run or unmet gate.
The fake judge in the acceptance harness is for tests; it is not part of a normal deployment.

### Dataset JSONL shape

One case per line, using synthetic content or content you have permission to retain:

```json
{"external_id":"storage","input":{"question":"Where are traces stored?"},"output":"In Postgres.","expected_output":"Traces are stored in Postgres.","context":[{"id":"docs","text":"Assay stores traces in Postgres."}]}
```

`output` is the captured/generated answer, `expected_output` is the reference, and `context` is
retrieval evidence. They are not interchangeable. Do not use a model answer as its own reference
unless that is explicitly the evaluation you intend to perform.

## 4. Run evaluations through Python

After configuring a judge, this uses a local-mode client and an existing application UUID:

```python
import os
import assay

with assay.Client(os.environ["ASSAY_ENDPOINT"], local_mode=True) as client:
    app_id = os.environ["ASSAY_APPLICATION_ID"]  # management UUID, not the ingest slug
    dataset = client.datasets.create(app_id, "Storage regression")
    client.datasets.create_items(dataset.id, (
        assay.DatasetItemInput(
            input={"question": "Where are traces stored?"},
            output="In Postgres.",
            expected_output="Traces are stored in Postgres.",
            context=(assay.Chunk(id="docs", text="Assay stores traces in Postgres."),),
        ),
    ))
    run = client.runs.create(app_id, dataset.id, "baseline", scorers=("groundedness", "correctness"))
    completed = client.runs.wait(run.id, timeout=120)
    if completed.status != "succeeded":
        raise RuntimeError(f"Evaluation ended with status {completed.status}")
    for item in client.runs.iter_all_items(run.id):
        print(item.id, item.status)
```

`ASSAY_APPLICATION_ID` above is an example-script variable, not an automatic tracing SDK setting.
In token mode, pass `admin_token` instead of `local_mode=True`.
`wait()` returns terminal runs, including failed/canceled ones; inspect status rather than assuming
that returning means success. A wait timeout does not cancel a server-side run.

## 5. Turn a production failure into a regression case

1. Find the trace and inspect its execution and captured evidence.
2. Attach a correct reference before requesting correctness if none was captured.
3. Request an online score and wait for its scoring task to finish; inspect any failure reason.
4. In the UI, save the trace's selected score evidence to a dataset, or call:

```python
item = client.datasets.from_trace(dataset_id, trace_id, scorer="correctness")
```

Here `trace_id` is the Assay UUID, not the OpenTelemetry ID. The dataset must belong to the same
application. Import retains the selected scorer's evidence. Importing the same trace/scorer twice
returns **409**, not an overwrite.

## Evaluation modes and comparisons

- **`score_existing`** (default): score the outputs already stored in dataset cases.
- **`generate_then_score`**: call the application's configured target endpoint, map its response,
  then score the generated output. Configure and test the target before running this mode;
  it may call external services and incur costs.

Compare a baseline and candidate run in the UI or with
`assay run compare BASELINE_ID CANDIDATE_ID --scorer correctness`.
Comparisons match immutable item IDs and report changed, one-sided and unscored exclusions.
They do not pretend that different or missing cases are comparable.

Runs retain immutable dataset-item snapshots. Editing/replacing/deleting an item does not rewrite
old runs; a new run uses the current dataset. Deleting an entire dataset cascades its runs.

**Next:** [CLI gates and exports](cli.md) · [Troubleshoot scoring](troubleshooting.md) ·
[Back up before changing data](deployment.md)
