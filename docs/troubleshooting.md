# Troubleshooting

[Documentation](index.md) / Troubleshooting

Start with `docker compose ps`, `docker compose logs --tail=100 assayd`, and `/readyz`.
Review logs locally and redact credentials, prompts and personal data before sharing them.
Do not paste `.env`, project keys or bootstrap workspace JSON into an issue.

## Connection and local mode

### The UI still asks for an admin token

1. Confirm you built a checkout containing local-mode support.
2. Put `ASSAY_LOCAL_MODE=true` in the `.env` used by that Compose project.
3. Run `docker compose up --build -d` to rebuild/recreate the service, then reload the page.
4. Request `http://localhost:8080/v1/server-info`; `local_mode` must be `true`.
5. Confirm the browser port matches `ASSAY_HTTP_PORT`, rather than another older Assay instance.

The UI discovers the server's mode; a browser/client environment variable cannot enable it remotely.
If discovery itself fails, the UI shows **Retry connection**, not an anonymous fallback.

### The CLI says an admin credential is required

The CLI runs on your host and does not read Docker's `.env` automatically. Export
`ASSAY_LOCAL_MODE=true` in that shell, or pass `local_mode=True` to the Python management Client.
Use the checkout SDK; the published 0.3.0 package does not contain this option.

### HTTP 401 in local mode

- Ingestion always needs a project API key. An admin token is not an ingest key.
- Explicit invalid/stale credentials are rejected; they are not promoted to local admin.
- A project key cannot call admin-only operations merely because local mode is enabled.
- Confirm you are reaching the intended local-mode instance.

For normal token mode, verify the admin token separately from `ASSAY_API_KEY`.

### HTTP 403 in local mode

Use `localhost`, a loopback IP, or the Compose service name `assayd`. Custom hostnames and browser
requests from another origin/port are intentionally blocked. Local mode does not trust forwarded
host headers. Use token mode behind a reverse proxy; do not weaken the local safeguards for a tunnel.

A development UI served on another port should use its API proxy so browser requests remain
same-origin. Do not add wildcard CORS to a token-free admin API.

### Port or database connection errors

Another process may use 8080 or source Compose's loopback Postgres port 5432. Choose unused
`ASSAY_HTTP_PORT` / `ASSAY_POSTGRES_PORT` values in `.env` and recreate the services.
Inside Compose, Postgres is `postgres`, and Assay is `assayd`; `localhost` refers to the caller's
own container. Wait for healthchecks before trying bootstrap scripts.

Changing `ASSAY_POSTGRES_PASSWORD` in `.env` does not rotate the password in an existing volume.
Restore the matching configuration or perform an intentional database-password rotation; do not
wipe the volume to resolve a credential mismatch.

## Tracing and sessions

### No traces appear

- Verify the SDK endpoint is Assay's base URL, not your model provider or application target.
- Confirm the project key is active and the application slug belongs to its project.
- Create the application first; auto-creation is off by default.
- End the span, then call `assay.flush()` and check its return value before process exit.
- Inspect OTLP partial-success/export messages: an HTTP success does not guarantee every span was accepted.
- Find the trace in **Traces**, not Sessions unless its root has `session.id`.
- Use JSON OTLP/HTTP. Binary protobuf and OTLP/gRPC are not implemented.

### The trace exists but messages are missing

Timing-only traces are valid. Decorators do not capture content unless enabled. Use explicit
`set_input` / `set_output` / `set_messages` for content you intend to export, and inspect capture
errors or malformed-message diagnostics. Keep a span open while consuming streamed output;
the decorator does not consume streams or intercept provider clients.

### Sessions is empty, or the transcript repeats history

Put `assay.session(session_id)` **outside the root span** for each request. Child-only IDs do not
establish membership. Reuse the same ID for related turns, and create separate traces per turn.
Keep only the current question/reply on each root; put full provider history on child model spans.
Existing untagged traces remain in Traces and are not inferred into sessions.

### `assay.session` or `local_mode` is missing

You are likely running the published SDK instead of the checkout. From your uv application project,
install the editable checkout path. Confirm the interpreter/module location without printing secrets:

```bash
uv run python -c "import assay; print(assay.__file__)"
```

In the Assay repository, use `uv run --project clients/python/assay ...`.

## Evaluations

### A trace is not eligible for scoring

Inspect **scoring eligibility** in the trace UI or `assay traces eligibility TRACE_ID`.
Mark the intended answer span scorable and provide valid captured messages. Groundedness needs
retrieval context; correctness needs a nonblank reference. A trace ID from the SDK is an OTel hex
ID; individual management commands use the Assay UUID returned by trace list/search.

### A scoring task/run failed

Read the task/item error and judge evidence. Typical causes include missing content/reference,
an unreachable judge URL, an invalid provider key/model, or a judge response that is not valid
structured output. Retry only after correcting the cause. Use a model that follows the judge's
JSON response contract reliably; do not silently accept invalid output as a score.

A local judge on the host is normally reached from Docker through `host.docker.internal`, not
`localhost`. Check provider availability separately from Assay readiness.

### Dataset import says conflict or has partial failures

Trace-to-dataset imports reject duplicate trace/scorer pairs with 409. Inspect the existing item
rather than importing it repeatedly. File imports report failures per item; successful rows may
already exist. Review the result before retrying a whole batch.

### A run timed out while watching

Client/CLI polling timeouts do not cancel server work. Inspect the run again, then explicitly cancel
if needed. Existing run snapshots do not change when you edit current dataset items.

## Persistence and recovery

`docker compose down` preserves the named database volume; `down -v` destroys it. Recreating
containers does not require regenerating secrets. If encrypted judge/target credentials can no
longer be decrypted, restore the **matching original encryption key**; generating a new key cannot
recover old ciphertext.

Before upgrades, take and verify a backup. Swapping back to an older image is not a schema rollback.
See [deployment and recovery](deployment.md) for the supported recovery process.
