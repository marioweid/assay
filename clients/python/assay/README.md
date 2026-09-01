# assay-sdk

The Python package for [Assay](https://github.com/marioweid/assay), a self-hosted LLM
tracing and evaluation platform.

```bash
uv add assay-sdk
```

The distribution is named `assay-sdk` and imported as `assay`.

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
`set_reference` on the active span. Call `assay.flush()` before a short-lived process exits and
`assay.shutdown()` when the tracing lifecycle ends.

## API Client

```python
import assay

with assay.Client("http://localhost:8080", admin_token="...") as client:
    applications = client.applications.list()
```

Management, dataset, scorer, and run operations use an admin token. Trace inspection and scoring
use a project API key.

## CLI

Set `ASSAY_ENDPOINT` and the relevant credential, then use the management and evaluation commands:

```bash
assay projects list
assay apps list --project PROJECT_ID
assay datasets import APPLICATION_ID --file regression.jsonl
assay run create APPLICATION_ID --dataset DATASET_ID --scorers groundedness,correctness
assay run watch RUN_ID --gate groundedness:0.8
assay traces score --scorer correctness TRACE_ID
```

Commands emit JSON. `assay run watch` returns exit code 1 when a run fails or a gate is not met,
which makes it suitable for CI checks.
