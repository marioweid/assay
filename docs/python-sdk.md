# Python SDK: from your first span to regression evaluation

[Documentation](index.md) / Python SDK

**The distribution is `assay-sdk`; the import is `assay`.** The SDK exports JSON OTLP/HTTP traces
and provides a synchronous typed management client and CLI. Python 3.10+ is supported by the package.

## Install the right version

Local-mode management and `assay.session(...)` are **checkout features**, not published
`assay-sdk==0.3.0`. From your own uv project, replace the path below with your actual Assay checkout:

```bash
uv add --editable /absolute/path/to/assay/clients/python/assay
```

For repository examples, use `uv run --project clients/python/assay ...` from the repository root.
For released functionality only, `uv add assay-sdk` installs the published package. Do not assume
that a matching `0.3.0` source version means unreleased checkout additions are on PyPI.

## Configure your application

Set these in the **instrumented program's environment**, not only Docker's `.env`:

```dotenv
ASSAY_ENDPOINT=http://localhost:8080
ASSAY_API_KEY=asy_your_project_key
ASSAY_APPLICATION=your-application-slug
```

Create the project/key/application first using the [quickstart](quickstart-linux.md).
The slug must belong to the key's project. From a container sharing Assay's Compose network, use
`http://assayd:8080`; `localhost` inside that container refers to itself.

`assay.init(endpoint=..., api_key=..., application=...)` overrides the corresponding environment
values. **Tracing requires a project key even when the server runs in local mode.** It does not
accept an admin token as an ingest credential. It never uses your application's evaluation target
or model-provider URL as the Assay endpoint.

## Lifecycle: initialize once, finish spans, flush, shut down

Save as `first_trace.py` and run it in the environment configured above:

```python
import assay

assay.init(service_name="support-worker")
try:
    with assay.span("answer", scorable=True) as current:
        current.set_input("Where does Assay store traces?")
        current.set_output("Assay stores traces in Postgres.")
        current.set_context((assay.Chunk(id="storage-doc", text="Assay stores traces in Postgres."),))
        current.set_reference("Assay stores traces in Postgres.")
        otel_trace_id = current.trace_id
    if not assay.flush(timeout_millis=30_000):
        raise RuntimeError("Assay export did not complete before the timeout")
finally:
    assay.shutdown()

print(f"Search Traces for {otel_trace_id}")
```

For a long-running service, initialize at startup and shut down during graceful termination,
not after each request. Repeating `init` with the same effective configuration is allowed;
changing it requires `shutdown` first. `flush` covers completed spans; do not flush while the
span you want to export is still open. Check the returned boolean in short-lived programs.

### Privacy comes before instrumentation

- Decorator capture defaults to **off**; opt in with `capture=True` when appropriate.
- Explicit `set_input`, `set_output`, `set_messages`, `set_context` and `set_reference` calls
  intentionally export supplied content even when decorator capture is off.
- `max_capture_bytes` defaults to 65,536 bytes per captured attribute. Structured messages/context
  must fit as complete payloads; simple capture helpers have different truncation behavior.
- Redact prompts, tool arguments/results and context before export. Never put credentials into
  attributes or session/user IDs. Local mode does not change these privacy requirements.

## Decorators: synchronous and asynchronous functions

After `assay.init()`, decorate the function whose execution you want to trace:

```python
import assay

@assay.trace(name="answer", capture=True)
def answer(question: str) -> str:
    return f"You asked: {question}"

@assay.trace(name="async-answer", capture=True)
async def async_answer(question: str) -> str:
    return f"You asked: {question}"
```

The decorator captures bound arguments and the return value when enabled. Exceptions remain
exceptions; the span records the failure. It does **not** intercept provider SDK traffic or
consume generators/streams for you. For streaming, keep an explicit span open while consuming
the stream and set the final output after completion.

A `redact` callback transforms captured content before serialization. For example, this deliberately
removes all function inputs/outputs rather than trying to guess which fields are sensitive:

```python
import assay

@assay.trace(capture=True, redact=lambda _: "[redacted]")
def sensitive_answer(question: str) -> str:
    return "A response that must not enter telemetry"
```

Use explicit spans when you need fine-grained context, token counts or scoring controls.

## Explicit spans: model calls, tools and retrieval

Nested spans form one trace. Mark the **one answer you intend to evaluate** as scorable, not every
nested operation. Call `set_context` for groundedness and `set_reference` for correctness.

```python
import assay

with assay.span("request") as request_span:
    request_span.set_input("Where are my traces?")
    with assay.span("generate", scorable=True, attributes={"gen_ai.operation.name": "chat"}) as model:
        model.set_attribute("gen_ai.request.model", "your-actual-model")
        model.set_input("Where are my traces?")
        model.set_output("In Postgres.")
        model.set_context((assay.Chunk(id="storage", text="Assay stores traces in Postgres."),))
        model.set_reference("Traces are stored in Postgres.")
        # Set usage from your provider's actual response, not estimated/example counters.
    request_span.set_output("In Postgres.")
```

`set_attribute` accepts OTel primitive values and supported arrays, not arbitrary nested objects.
Set `gen_ai.usage.input_tokens` / `gen_ai.usage.output_tokens` only when you have the actual counts.
`current.trace_id` and `current.span_id` are available only while that span context is active.
The former is an OTel hex ID, not the Assay UUID used by management endpoints.

## Structured conversations and tools

`set_messages` accepts the SDK's pinned OTel GenAI message shape. Text uses `content`; tool-call
responses use `response`. It replaces only the supplied side(s), validates both before writing,
and rejects malformed or oversized collections instead of exporting half a message.

```python
import assay

with assay.span("model", attributes={"gen_ai.operation.name": "chat"}) as current:
    current.set_messages(
        input=[
            {"role": "user", "parts": [{"type": "text", "content": "Find the storage docs"}]},
        ],
        output=[
            {"role": "assistant", "parts": [
                {"type": "tool_call", "id": "lookup-1", "name": "lookup", "arguments": {"query": "storage"}},
            ]},
        ],
    )
```

For a subsequent provider call, put the tool response in its input messages using
`{"role": "tool", "parts": [{"type": "tool_call_response", "id": "lookup-1", "response": "..."}]}`.
Use `set_messages(..., redact=your_callback)` to sanitize complete supplied collections before
validation. Exported types include `Message`, `TextPart`, `ToolCallPart` and `ToolResultPart`.
See [semantic conventions](semantic-conventions.md) for the full transport contract.

## Sessions across multiple requests

A session groups **separate traces**, one per turn. Reuse an opaque ID while the conversation
continues; rotate it when the user starts a new conversation. Put the session scope **outside**
the root span so the root receives `session.id`:

```python
from uuid import uuid4
import assay

session_id = str(uuid4())  # persist this in your application's trusted session state

for question, answer in [
    ("Where are traces stored?", "In Postgres."),
    ("Can I inspect the evidence?", "Yes, select the answer's source trace."),
]:
    with assay.session(session_id, conversation_id=session_id):
        with assay.span("chat-turn") as turn:
            turn.set_input(question)
            turn.set_output(answer)
```

Initialize once before the loop and flush/shut down afterward as in the lifecycle example.
The root should capture only the **current** question/reply. Full provider history belongs on a
child model span; otherwise your session transcript repeats previous turns.

The scope is task-local and safe across concurrent async requests. Nested scopes restore their
previous value, including on exceptions. It tags new spans; it does not retroactively tag a root
created earlier. `conversation_id` applies to spans identified with `gen_ai.operation.name`.
Optional `pseudonymous_user_id` sets `enduser.pseudo.id`, not authenticated `user.id`.
IDs are bounded to 128 characters and must not contain leading/trailing ASCII spaces or controls.
Use random opaque identifiers, not emails, access tokens or personal data.

This helper **does not** implement browser authentication, cookies, transcript authorization,
model context or persistence for your app. The separate [chat example](../examples/python-qa/README.md)
implements those with a signed browser capability. A session ID alone is never a read permission.

## Management client: local and token modes

Local mode explicitly permits credential-free management requests:

```python
import assay

with assay.Client("http://localhost:8080", local_mode=True) as client:
    client.ready()
    applications = client.applications.list()
    for application in applications:
        print(application.id, application.slug)
```

The server must independently set `ASSAY_LOCAL_MODE=true`. `local_mode=True` cannot bypass a
secure server. If the parameter is omitted, the client reads `ASSAY_LOCAL_MODE` (default false).
`local_mode=False` overrides the environment. No dummy token is generated or persisted.

Normal mode uses `assay.Client(endpoint, admin_token=...)`. For project-scoped reads, pass
`api_key=...`; trace/session operations send that key and retain its scope in local mode. When both
credentials are supplied, trace operations prefer the project key, while management uses admin.

Management operations do not use the project key. With `local_mode=True` and no admin token,
`client.projects.list()` sends an anonymous management request **even if `api_key` is supplied**.
An API key therefore does not restrict the management capabilities of a local-mode Client.
Credentials actually sent on a request are validated: a stale admin token or an invalid key on a
trace read is rejected, not silently replaced with anonymous access.

The client is synchronous. In an async server, do blocking management work outside the event loop
(for example using `asyncio.to_thread`), and close owned clients with a context manager.

### Resource map

| Resource | Common operations |
|---|---|
| `client.projects` | `create`, `list`, `get`, `update`, `delete` |
| `client.keys` | `create`, `list`, `revoke` (raw key returned once) |
| `client.applications` | `create`, `list`, `get`, `update`, `set_endpoint`, `clear_endpoint`, `delete` |
| `client.traces` | `list`, `get`, `score`, `set_reference`, `eligibility`, `delete`, `iter_all_traces` |
| `client.sessions` | `list(application_id)`, `turns(application_id, session_id)`, `recent(...)` |
| `client.datasets` | `create`, `import_file`, `from_trace`, item CRUD and page iterators |
| `client.scorers` | `list`, `set` |
| `client.runs` | `create`, `get`, `wait`, `cancel`, `compare`, item/score reads and iterators |
| `client.scores` / `client.metrics` | Application score exports and trends |

Trace deletion and eligibility are management operations. Projects, applications, datasets,
scorers, runs, metrics and score export also require management access. Trace/session reads,
reference attachment and online scoring can use scoped project keys.

### Read traces and paginate sessions

The examples below assume an open `client`, an application UUID `app_id`, and a session ID:

```python
page = client.traces.list(application_id=app_id, limit=50)
for trace in page.items:
    print(trace.id, trace.root_name)

cursor = None
while True:
    turns = client.sessions.turns(app_id, session_id, cursor=cursor)
    for turn in turns.items:
        print(turn.id, turn.root_name)
    cursor = turns.next_cursor
    if cursor is None:
        break
```

`recent(app_id, session_id, limit=19)` returns a bounded chronological tuple of the latest turns,
not the entire history. Use opaque cursors unchanged; do not parse or manufacture them. Other
resources expose `iter_all_*` helpers when available. List APIs return typed `Page` objects;
non-paginated resources commonly return tuples.

## Errors and safe operations

Catch `assay.exceptions.AssayError` at an application boundary, or handle its specific subclasses:
`AssayConfigurationError`, `AssayAPIError`, `AssayTransportError`, `AssayTimeoutError`,
`AssayProtocolError` and `AssayImportError`. Malformed nested server responses fail validation;
the client does not silently return partial models. Do not automatically retry destructive writes.

`datasets.replace_item` is a **full replacement**; explicit `None` clears nullable output/reference
fields. Old runs retain their creation snapshot. Re-importing the same trace/scorer evidence gives
409 rather than overwriting an existing regression item.

**Next:** [Evaluation workflow](evaluations.md) · [CLI cookbook](cli.md) ·
[Configuration](configuration.md) · [Troubleshooting](troubleshooting.md)
