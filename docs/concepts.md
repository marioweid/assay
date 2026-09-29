# Concepts and UI tour

[Documentation](index.md) / Understand the workspace

## The data model, in one picture

```text
Project ── project API keys
  └─ Application (identified by a slug when ingesting)
      ├─ Traces ── spans ── captured messages, tools, context, scores
      ├─ Sessions ── explicitly tagged traces from multiple turns
      └─ Datasets ── test cases ── evaluation runs ── item evidence and scores
```

| Term | What it means | Common mistake |
|---|---|---|
| Project | A group of applications and scoped API keys | Using an application's UUID as a project ID |
| Application | The system you are observing, with a stable slug | Passing its display name to the SDK |
| Trace | One request/execution, made up of spans | Treating a session as one giant trace |
| Span | One operation: answer, model call, retrieval or tool | Marking every nested span scorable |
| Session | Separate traces sharing a root `session.id` | Tagging only a child span |
| Dataset | Versioned-by-snapshot evaluation inputs/evidence | Expecting edits to rewrite old runs |
| Run | One evaluation of a dataset snapshot | Treating a queued run as completed |
| Score | A judge result and retained evidence | Confusing a low score with an execution error |

## Two kinds of credential

- **Admin token:** management/UI access in normal mode. It is not the SDK ingest key.
- **Project API key:** returned once when created; scopes trace ingestion and project trace reads.
  Use separate keys per application environment/automation where useful, and revoke unused keys.

`ASSAY_LOCAL_MODE=true` replaces only the admin-token requirement. It gives anonymous callers
management access; it does not make a project key safe to publish or turn a session ID into a
permission token. Do not expose a local-mode server to untrusted clients.

## Projects and applications

Create a project first. On its detail page, create an API key and save the one-time value privately.
Then create an application in that project. Your instrumented program uses:

```dotenv
ASSAY_ENDPOINT=http://localhost:8080
ASSAY_API_KEY=asy_your_project_key
ASSAY_APPLICATION=your-application-slug
```

The endpoint is **Assay**, not your model provider or evaluation target. Application target endpoints
are separate: evaluation workers call them only for generation-based evaluation workflows.

## Traces: start with the answer

Select an application → **Traces**. Search by root operation or trace ID; filter by execution status
or recorded scorer result. Open a trace to inspect its captured conversation, then reveal spans,
timing, retrieval context, score evidence and raw attributes.

Capture is optional. A trace with no captured messages is still useful for timing and status.
An error trace and a trace with a failing correctness score mean different things.

### IDs you will encounter

- `AssaySpan.trace_id`: the OpenTelemetry trace ID (32 hexadecimal characters).
- Trace API `id`: Assay's database UUID, used by `client.traces.get(id)` and UI deep links.
- `session.id`: an opaque correlation value you choose; not either trace ID and not authentication.

Use the trace list search to map an OpenTelemetry ID to its Assay record.

## Sessions: follow the conversation

Sessions contains only traces whose **root span** has `session.id`. Untagged traces remain in
Traces. In a session:

1. Choose a recent session in the desktop sidebar, or use **All sessions** on mobile.
2. Read the user/assistant turns. Select either message to inspect its source trace.
3. Inspect duration, tokens, execution steps, scores, tools and retrieved context.
4. Use the timing below the conversation for the selected turn, or switch to **Timeline** for
   timestamps across loaded turns. Keyboard navigation and zoom are supported.
5. Use **Load more turns** for older/longer paginated histories; summary counts say *loaded* until
   the entire available page sequence is present.

The conversation displays root-turn messages, not the repeated model history from every child span.
For a chat application, keep the root capture to the current question/reply; put full provider
context on child model spans. See [SDK sessions](python-sdk.md#sessions-across-multiple-requests).

## Datasets, evaluations and trends

Save a scored trace to a dataset to preserve a regression example and selected score evidence.
Import JSON/JSONL cases or create them through the client. Run groundedness/correctness evaluators,
inspect per-item evidence, compare runs, and use **Score trends** for recorded application metrics.

Dataset-item edits/deletion preserve old run snapshots. Deleting a **whole dataset** cascades its
runs. Read the confirmation dialog and [recovery guidance](deployment.md) before deleting data.

**Next:** [Python SDK](python-sdk.md) · [Your first evaluation](evaluations.md)
