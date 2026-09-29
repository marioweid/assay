# assay-sdk

The typed Python SDK and CLI for [Assay](https://github.com/marioweid/assay), a self-hosted LLM
tracing and evaluation workspace. The distribution is `assay-sdk`; the import is `assay`.

[Full SDK guide](../../../docs/python-sdk.md) · [Getting started](../../../docs/index.md) ·
[CLI cookbook](../../../docs/cli.md) · [Evaluations](../../../docs/evaluations.md)

## Installation and release status

This source tree is version **0.4.0**. For its local-mode management, session context/reads and
structured conversation features, use a verified PyPI release of at least 0.4.0:

```bash
uv add 'assay-sdk==0.4.0'
```

Published `assay-sdk==0.3.0` does **not** include these features. If 0.4.0 is not yet available on
PyPI, use the checkout in your application project until the release is verified:

```bash
uv add --editable /absolute/path/to/assay/clients/python/assay
```

From the repository root, examples and the CLI use `uv run --project clients/python/assay ...`.

## Trace an answer

Set `ASSAY_ENDPOINT` to Assay's base URL, `ASSAY_API_KEY` to a project key, and
`ASSAY_APPLICATION` to the application's slug. Project keys remain required in local mode.

```python
import assay

assay.init()
try:
    with assay.span("answer", scorable=True) as current:
        current.set_input("Where are traces stored?")
        current.set_output("In Postgres.")
        current.set_context((assay.Chunk(id="docs", text="Assay stores traces in Postgres."),))
        current.set_reference("Traces are stored in Postgres.")
    if not assay.flush():
        raise RuntimeError("Assay export did not complete")
finally:
    assay.shutdown()
```

Initialize once per process; shut down during graceful termination. Decorators support synchronous
and asynchronous functions. `@assay.trace(capture=True)` captures arguments and returns; provider
traffic and stream consumption are not automatically instrumented.

**Privacy:** decorator capture is off by default. Explicit message/input/output/context/reference
setters intentionally export their values even with capture off. Redact before export. Structured
messages validate complete collections and reject oversized content instead of partially writing it.
See the [capture and redaction guide](../../../docs/python-sdk.md#privacy-comes-before-instrumentation).

## Sessions and structured conversations

Wrap each request's root span with `with assay.session(session_id):` to group separate turn traces.
The scope is task-local, supports async concurrency and restores nested values on exceptions.
Use opaque IDs, not personal information. Capture only the current question/reply on the root;
full model history belongs on child model spans.

`AssaySpan.set_messages(input=..., output=..., redact=...)` supports the pinned OTel GenAI text and
client-tool shapes. Optional `conversation_id` tags GenAI spans, and `pseudonymous_user_id` identifies
an anonymous browser, not an authenticated user. Session IDs are correlation metadata, not transcript
permissions. See [complete session examples](../../../docs/python-sdk.md#sessions-across-multiple-requests).

## Management client

For a server explicitly configured with `ASSAY_LOCAL_MODE=true`:

```python
import assay

with assay.Client("http://localhost:8080", local_mode=True) as client:
    client.ready()
    applications = client.applications.list()
```

For normal authentication, pass `admin_token=...`. For scoped trace/session reads, pass `api_key=...`.
An explicit project key keeps its scope for trace/session operations even in local mode.
Management operations instead use the admin token, or anonymous access in local mode when no admin
token is supplied; a project key does not restrict those management calls.
If `local_mode` is omitted, the client reads `ASSAY_LOCAL_MODE`, defaulting to false; explicit false
overrides it. Endpoint and credentials
are constructor arguments, not automatically loaded from environment. A client flag never disables
authentication on a token-protected server.

The context-managed client is synchronous and exposes projects, keys, applications, traces,
sessions, datasets, scorers, runs, scores and metrics. Models are typed; malformed server data raises
`AssayProtocolError` rather than returning a partial object. Use the
[resource and pagination guide](../../../docs/python-sdk.md#resource-map) for details.

## CLI

The CLI reads `ASSAY_ENDPOINT`, `ASSAY_ADMIN_TOKEN`, `ASSAY_API_KEY` and `ASSAY_LOCAL_MODE` from its
environment. It does not automatically load Docker's `.env`.

```bash
uv run --project clients/python/assay assay projects list
uv run --project clients/python/assay assay --help
```

Commands cover bootstrap, trace inspection/scoring, dataset import/export, evaluation runs,
comparisons, metrics and CI gates. Delete/revoke/clear actions require `--yes`. Read the
[CLI cookbook](../../../docs/cli.md) before destructive operations.

## Verification

The repository has offline SDK/CLI tests and a disposable fake-judge acceptance harness. The live
product test runs through `tests/acceptance/run.sh`; it does not call a paid provider. The optional
real-judge workflow is separate and requires explicit endpoint/credential configuration.

[Linux quickstart](../../../docs/quickstart-linux.md) ·
[PowerShell quickstart](../../../docs/quickstart-powershell.md) ·
[Semantic conventions](../../../docs/semantic-conventions.md)
