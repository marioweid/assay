# M7: GenAI Product Completion Design

**Status:** Proposed for review; not implemented or an approved API contract.
**Baseline:** `dcf5951` on `main`, inspected 2026-09-08 after `git fetch origin`.
**Request:** [`../../improvements.md`](../../improvements.md), all nine observations.
**Plan:** [`../plans/2026-09-08-product-completion.md`](../plans/2026-09-08-product-completion.md).

## 1. Diagnosis

Assay needs a product-completion milestone, not a replacement architecture. M5.5 intentionally shipped
an inspection/test UI. M6 completed analytics, retention, and agent workflows, not a fully operable
browser product. A later chat-example change added dataset creation and rudimentary captured content.
The original design's opening status paragraph still says M6 is deferred, although its roadmap and
README mark it complete. Treat implementation, not that stale paragraph, as the baseline.

### Evidence ledger

These are source findings, not claims that a browser acceptance suite was run.

| Finding | Evidence | Consequence |
|---|---|---|
| Management UI was deliberately excluded | `docs/specs/2026-09-01-m5-5-web-ui-design.md`, Scope | Add a milestone rather than reopen all of M5.5 |
| Projects/apps already have CRUD APIs and Python methods | `internal/api/{projects,applications}.go`; SDK `client.py` | Build forms over existing endpoints |
| Dataset metadata update and individual item CRUD are absent | `internal/api/datasets.go`, route registration | Backend work is necessary |
| Runs snapshot membership, not source content | `db/queries/eval_runs.sql`, `CreateEvalRunItems`, `ListPendingEvalRunItems` | Editing a case could change queued evaluation inputs |
| Deleting an item currently cascades into run history | migration `00004_offline_scoring.sql:72` and `00005_online_scoring_generation.sql:43-44` | Add immutable run-item snapshots before item delete |
| UI active states differ from server | `web/src/features/runs/{use-run-polling.ts,run-detail.tsx}` use `queued`; domain uses `pending` | Pending runs may stop polling and lack cancellation |
| Run detail only renders aggregates | `web/src/features/runs/run-detail.tsx`, `RunSummary` | Users cannot inspect item outcomes/rationales there |
| Trace content is an attribute renderer | `features/traces/captured-content.tsx`, `ContentValue` | Text `parts`, tool activity, and provenance need a real view model |
| Selected-span capture includes descendants | `trace-detail.tsx` calls recursive `capturedSpans([selected])` | Selection is not an exact span scope |
| No span timing chart | `features/traces/span-tree.tsx` | Add a real relative-time waterfall |
| Trace search only filters loaded pages | `features/traces/traces-page.tsx`, `visibleItems` | Search can hide matching traces on later pages |
| Trace list omits scores server-side | `internal/api/traces.go`, `traceOutput(trace, false)` | Do not add a detail request per row |
| Trace mutations require project keys | `internal/api/traces.go`, `scoreTraces`, `attachTraceReference` | Browser admin token cannot operate these flows yet |
| Post-connect 401 is only normalized | `web/src/api/client.ts` and `auth/auth-context.tsx` | Add centralized credential invalidation with stale-request fencing |
| App catalog is fetched on connection only | `auth/auth-context.tsx` | CRUD requires catalog refresh without reconnecting |
| A production Dockerfile already exists | `assayd/Dockerfile`; `web.yml` builds it | Extend delivery, do not rebuild packaging from scratch |
| No image-publish workflow is checked in | `.github/workflows/`; proposed example in `docs/ci-cd.md` | Publishing is a missing delivery capability, not a verified public image |

Backend paths above are relative to `assayd/`. Further source risks to cover with reproductions:
`spanTree` can lose cyclic parent groups, and request cleanup in mutation dialogs can permit stale
callbacks after route changes. Do not label these runtime-reproduced bugs until their tests fail.

### Research limits

- The running local server answered `GET /readyz` with `ready`; its public OpenAPI was retrieved.
- No existing project data was changed; no real judge calls or image publications were made.
- No authenticated browser session or full application test suite was run during planning.
- Go, Node, and pnpm were not on the planning shell PATH. Install/use the repository's toolchain for
  execution; a successful health check is not build evidence.
- Context7 and browser MCP tools were unavailable. Inspect current official documentation during the
  dependency/API research steps before adding packages. No new dependency versions are guessed here.

## 2. Alternatives and recommendation

1. **CSS/forms only:** fastest visible change, but leaves broken pending-state behavior, mutable run
   evidence, incomplete evaluation drilldown, and publication gaps. Not recommended.
2. **Incremental product completion:** preserve Go/Postgres/OTLP/React, repair correctness, then ship
   coherent vertical workflows with browser evidence. **Recommended.**
3. **Replatform/build an enterprise suite:** adds authentication infrastructure, query frameworks,
   agents, prompt management, and more scorers at once. Too much risk; does not solve these test notes
   sooner. Explicitly excluded.

## 3. Global constraints

- Deployment remains one `assayd` binary with embedded static UI plus Postgres.
- Keep Go 1.27, Node 22 ESM, React/Vite/TypeScript, and Python 3.13 with uv.
- Preserve the SDK's declared Python >=3.10 compatibility and existing supported-version CI matrix.
- Keep current exact dependency pins; verify current stable versions before adding any dependency.
- Use oxlint/oxfmt/tsc/Vitest, Go tests/golangci-lint, and ruff/ty/pytest; no warning suppression by default.
- Edit SQL and Huma source, then regenerate sqlc/OpenAPI/TypeScript; never hand-edit generated output.
- Business logic stays in domain services; handlers and CLI commands delegate.
- Keep content capture opt-in; never capture provider traffic globally as a side effect of initialization.
- Keep project-key isolation; never store project/judge keys in browser persistent storage.
- No credentials, captured messages, or tool arguments in URLs, logs, screenshots, or error telemetry.
- Every browser mutation has an API and a typed Python-client equivalent; CLI exposes the new workflows.
- Treat imported content as untrusted text; no raw HTML execution or automatic external media fetches.
- No generic CRUD engine, new database, runtime Node service, global state framework, or chart framework.
- New work must support keyboard operation, 360px width, 200% zoom, and reduced motion.
- Do not claim published images or new SDK functions in user-facing docs before release verification.

## 4. Target experience

### First session

Run the documented Compose command, connect once with an admin token, create a project/application,
create an ingest key (shown once), and copy a runnable SDK example. Judge credentials are optional for
tracing; explain exactly when evaluation needs them. An empty workspace must not send users to curl
merely to create an application.

### Daily tracing

The app opens on Traces. Search and time/status/scorer filters apply across the application, not just
loaded rows. A trace opens with a concise metadata header, the conversation as the primary overview,
and a synchronized timing waterfall. Model, tools, retrieval, errors, and scoring explain the answer
without requiring users to decode attributes. Raw attributes/events remain available.

### Evaluation loop

Inspect a failed answer → add/edit a reference → score → see eligibility/progress/rationale → save
captured evidence to a dataset → correct a case or generate new outputs → run evaluation → inspect
individual failures → compare two runs on matched cases. Execution failure is not a zero quality score;
missing scores are not passes. The same loop works from Python and the CLI.

## 5. Scope and lifecycle semantics

“CRUD everything” means full management of editable resources, not editable telemetry or fabricated
judge results. Make these distinctions explicit in copy and APIs.

| Resource | Operations in M7 | Deliberate constraints |
|---|---|---|
| Projects | create/list/read/update/delete; judge settings | Delete explicitly warns it removes all child apps/data |
| Applications | create/list/read/update/delete; endpoint and auto-score settings | Project ownership immutable; slug change warns emitters must change |
| API keys | create/list/revoke; copy once at creation | Never recover/edit key material; rotate by create then revoke |
| Datasets | create/list/read/update/delete; JSONL import/export | Application ownership immutable; whole-dataset deletion still removes its runs |
| Dataset cases | create/read/replace/delete | Item edits/deletion preserve prior run snapshots and scores |
| Scorer configs | view/set/enable/disable and judge overrides | Built-in scorers are not user-created resources; no custom scorer registry |
| Traces/spans | ingest/read/export; attach reference, score, delete whole trace | Raw telemetry immutable; no fake span editor or individual span delete |
| Runs | create/list/read/cancel/delete terminal run; rerun with current dataset | No rewriting outcomes; running/pending delete returns conflict |
| Scores | read/filter/export/compare | No create/edit score form; rerun evaluator to change a result |

Whole-project/application/dataset/trace/run deletion is permanent, uses existing lease fencing, and
requires a named confirmation in the UI and `--yes` in the CLI. Individual case deletion requires a
confirmation explaining historical evidence remains. Do not promise that a cascade has an exact
record count unless the backend actually supplies it.

## 6. Data and API decisions proposed for approval

### 6.1 Historical evaluation inputs

Add immutable snapshot fields to `eval_run_items`: source dataset ID, external ID, input, output,
expected output, context, metadata, and original item timestamps. Keep `dataset_item_id` as the stable
historical case identity, but remove its FK to live `dataset_items`; retain the composite score FK to
`eval_run_items`. Keep the run-to-dataset cascade for explicit whole-dataset deletion.

Backfill existing snapshots from live rows at migration time. Label them `legacy_backfill`, not exact
creation-time evidence. New snapshots are `creation`. Never infer historical inputs that no longer
exist. Existing score evidence is shown when actually available; it does not justify guessing missing
run inputs. Workers and item APIs read only snapshots after migration.

Run creation already uses a repeatable-read transaction. Validate output/reference eligibility against
the newly copied snapshot in that same transaction before inserting its job, and roll back an invalid
run. Item mutation must not require a global jobs-table lock now that history is independent. Retain
existing job-first locking for cascades and worker transitions. Test races using real Postgres barriers,
not sleeps.

No separate dataset-version subsystem is needed in this release. A snapshot makes input history
honest; it does not make judge output deterministic or freeze external target behavior.

### 6.2 New REST surface

All routes below are proposals, implemented only after design approval. JSON names are canonical.
Existing routes and auth remain unless explicitly changed.

| Method/path | Operation ID | Semantics |
|---|---|---|
| `PATCH /v1/datasets/{id}` | `update-dataset` | `{name?, description?, clear_description?}`; clearing and setting conflict |
| `GET /v1/datasets/{id}/items/{itemId}` | `get-dataset-item` | scoped read; wrong parent returns 404 |
| `PUT /v1/datasets/{id}/items/{itemId}` | `replace-dataset-item` | full replacement of editable fields, preserving ID/created_at |
| `DELETE /v1/datasets/{id}/items/{itemId}` | `delete-dataset-item` | 204; historical run snapshots survive |
| `POST /v1/datasets/{id}/from-trace` | `create-dataset-item-from-trace` | `{trace_id, scorer, expected_output?}`; score-evidence import, 201 |
| `GET /v1/traces/{id}/scoring-eligibility` | `get-trace-scoring-eligibility` | per-scorer prerequisites without making judge calls |
| `DELETE /v1/traces/{id}` | `delete-trace` | admin-only whole-trace deletion, 204 |
| `DELETE /v1/runs/{id}` | `delete-eval-run` | admin-only terminal run deletion, 204 or 409 |
| `GET /v1/runs/{id}/items/{itemId}` | `get-eval-run-item` | one scoped snapshot, outcome and scores; admin-only |
| `GET /v1/runs/{id}/comparison` | `compare-eval-runs` | `other_run_id`, `scorer`, limit/cursor; matched item deltas |

Dataset mutations/import, run comparison/deletion, and eligibility read use admin auth. Existing
`POST /v1/traces/score` and `PATCH /v1/traces/{id}/reference` additionally accept admin auth. A project
key retains exactly its previous project scope. Admin multi-trace scoring may cross applications
within one project, but a mixed-project batch is rejected atomically with 422; the UI selects one app.
Ingestion remains project-key-only. Do not infer a project solely from user-supplied IDs without
loading and validating ownership.

Item replacement body requires all editable fields: `external_id`, `input`, `output`,
`expected_output`, `context`, `metadata`. Nullable strings use JSON null to clear. `input` and
`metadata` must be objects, `context` an array of `{id,text}`; existing nonblank/unique chunk validation
applies. Preserve the current required nonblank `input.question`; arbitrary input schemas are not
introduced by this milestone. Empty context is allowed for reference-only evaluation. An omitted required field is 422,
not “preserve old value.” POST create retains its current contract.

Eligibility returns `{items:[{scorer, eligible, reasons:[{code,message}]}]}`. Stable reason codes:
`scorer_disabled`, `missing_judge`, `missing_scorable_span`, `multiple_scorable_spans`,
`missing_input`, `missing_output`, `missing_context`, `missing_reference`, `malformed_content`.
Use the same backend validators as actual queueing. Process judge defaults must be injected into the
eligibility service; never expose API keys. Missing API key is not automatically invalid for local
OpenAI-compatible endpoints. `eligible` describes the current check, not a reservation; queueing
revalidates.

Trace-to-dataset import uses the latest persisted selected scorer evidence, including after span
retention. Enforce same application in the domain transaction. Preserve current external ID
`trace:<trace_id>:<scorer>` and provenance metadata from the SDK; duplicate import is 409, never an
upsert. Replace duplicated SDK orchestration with this endpoint; keep the public Python method name.
Unscored traces can be entered through the ordinary case form; do not silently invent score evidence.

### 6.3 Trace list and detail

Extend trace list query with `q` (trimmed, max 200 chars; literal case-insensitive root-name match or
exact Assay/OTel trace ID), `scorer`, and `passed`. `passed` requires `scorer`. Keep existing
`application_id`, `start`, `end`, `status`, `limit`, `cursor`. Filter online scores only. Use EXISTS
filters and a set-based latest-score summary query; never fetch full spans/messages for each row.
A zero score and an absent score remain distinct. Add `score_summaries` with
`{scorer,value,threshold,passed,created_at}` to list rows, sorted by scorer.

Detail keeps the existing nested spans contract. Make backend tree construction cycle-safe before
building richer views. Every retained valid span is reachable exactly once; orphan/cyclic links are
broken deterministically for presentation, with parent IDs preserved in attributes/response. Do not
silently change stored telemetry to produce the tree.

### 6.4 Run item inspection and comparison

Extend run-item response with `snapshot: DatasetItemResponse`,
`snapshot_origin: "creation" | "legacy_backfill"`, and `scores: ScoreResponse[]` loaded in a bounded
page query. This avoids joining two independently paginated lists in the browser or fetching the
live dataset item to reconstruct history.

Comparison requires distinct terminal runs in the same application and same dataset, and a scorer
requested by both. Return 422 for incompatible selections, 409 for active runs. Match by historical
`dataset_item_id`; do not fuzzy-match text or external IDs. Per item show each outcome, output,
score, threshold, model, prompt version, and `delta = candidate.value - baseline.value` only when
both scores exist and input/reference snapshots match. Changed input/reference becomes
`changed_case`; absent case is `baseline_only`/`candidate_only`; absent score is `unscored`.
Zero remains a score. Warn when judge/prompt/threshold differs; do not call such a comparison a
statistical experiment. Aggregate deltas only over matched, scored, unchanged cases and report `n`.

## 7. Visual design

Reference inspected: Odysseus default `dev` at
[`934d23c0be29c9721385f34565c0ae2cbd60da04`](https://github.com/odysseus-dev/odysseus/tree/934d23c0be29c9721385f34565c0ae2cbd60da04),
`static/style.css:18-105` and `assets/branding/odysseus-browser.jpg`. Its current default is dark,
not the light gray interpretation in the older Assay design. Odysseus is AGPL-3.0-or-later: use visual
inspiration only; do not copy CSS, components, fonts, or branding into Apache-2.0 Assay.

Original Assay direction:

- Slate canvas, slightly elevated blue-gray panels, restrained cyan/blue accent, readable off-white
  text; muted red/amber/green for semantic state, never neon as the only status signal.
- Light and dark themes, with system preference by default and an explicit three-way selector.
  Retain self-hosted IBM Plex Sans for readable conversations and Mono for timing/IDs/code.
- One 224px navigation rail, 56px header, 36px controls, 44px table rows; 8px panel/control radius;
  4/8/12/16/24/32px spacing scale; subtle separators rather than boxes inside every box.
- Workspace navigation: Applications, Projects; application navigation: Traces, Evaluations,
  Datasets, Score trends, Settings. Keep existing `/runs` and `/metrics` route paths; improve labels.
- Conversation content width about 80 characters; technical inspectors use remaining width. No huge
  mascot, empty hero section, decorative chart, or marketing heading in the working area.
- Shared accessible buttons/fields/dialogs/tabs/status/empty/error/loading primitives using installed
  Radix; avoid a generic form renderer. Account for the production Content-Security-Policy.

### Trace layout

```text
Breadcrumb / root operation                          Score · Save to dataset · More
Status · duration · model(s) · input/output tokens · trace ID copy
Overview | Attributes | Events | Scores
┌───────────────────────────────────────────────┬─────────────────────────────┐
│ Conversation / selected model call            │ Span hierarchy + waterfall  │
│ user → assistant → tool request/result        │ relative axis, expand/select│
│ context and reference are separate sections   │ duration, status, operation │
│ score summary links to full evidence          │                             │
└───────────────────────────────────────────────┴─────────────────────────────┘
```

At narrow widths conversation precedes the timeline; controls remain available. Default overview
chooses the sole scorable model call when present, otherwise the earliest call with messages. Other
calls are explicit groups, not concatenated into a fictional conversation. Repeated prompt history
across calls is collapsed at the call-group level, not globally deduplicated by text. Each group
retains its source span, model, and token usage. Selecting a span shows exactly that span, not its
subtree. A trace-summary action resets the selection. URL stores tab/span selection, never content.

Messages support role, ordered text parts, structured JSON, tool calls/results, and unsupported parts
with an honest raw fallback. Missing, malformed, empty, and truncated captures have distinct labels.
Multimodal references are labeled attachments; no remote loading. Render safe Markdown only if the
small renderer dependency is justified and security-tested; code/text/JSON support is mandatory even
without Markdown. Do not display uncaptured/private reasoning as if it existed.

The waterfall uses relative actual span start/end times, not sibling order or summed durations.
Concurrent spans overlap. Keep sub-millisecond precision until relative conversion, clamp invalid
geometry, and show a zero-duration marker. Include an equivalent textual table for accessibility.
Expansion, selected span, and call-group links synchronize without replacing the conversation tab.

## 8. SDK ergonomics

Retain the three environment values `ASSAY_ENDPOINT`, `ASSAY_API_KEY`, `ASSAY_APPLICATION`, then
`assay.init(capture=True)` and `@assay.trace` as the minimal automatic-capture path. Keep explicit
`set_input`/`set_output` unchanged. Add `AssaySpan.set_messages(*, input=None, output=None, redact=None)`
for structured conversation capture, and read-only `AssaySpan.trace_id` / `span_id` as hex identifiers
inside an active context. `set_messages` is explicit capture opt-in; it never activates global capture.

Typed messages cover system/developer/user/assistant/tool roles and ordered text/tool parts using the
pinned OTel contract; unknown provider objects are not automatically introspected. The official GenAI
schemas have moved to `open-telemetry/semantic-conventions-genai`. Planning inspected commit
`b5d8440f6f126738fd50f927752cd669772c517b`, files `model/gen-ai/gen-ai-input-messages.json` and
`model/gen-ai/gen-ai-output-messages.json`. They define ordered `parts`, `tool_call` with name/arguments
and optional ID, and `tool_call_response` with response and optional ID; extension roles are allowed.
See the pinned links in subplan D4. New helpers emit this shape; existing content-string helpers
remain their separate simple-input API, not a deprecated alternative transport. Serialize valid
JSON under the byte budget; redact before serialization and preserve tool IDs. If a structured payload
cannot fit, fail the explicit setter with a content-free validation error instead of emitting corrupt
JSON or silently truncating a tool ID. Existing decorator truncation behavior remains unchanged and
is documented. Tests ensure redactor/serialization failures do not leak payloads.

The current decorator wraps functions, not provider SDK streaming integrations. Document that boundary;
do not imply it captures generator consumption or all OpenAI/Pydantic AI calls automatically. Dedicated
provider instrumentation and streaming integrations are a follow-up, not hidden prerequisites for M7.

## 9. Delivery and documentation

Extend the existing Docker build with a SHA-pinned publishing workflow for
`ghcr.io/marioweid/assay`, immutable release/SHA tags, OCI source/revision/version labels, SBOM and
provenance. Default target is linux/amd64; linux/arm64 is part of acceptance if both builds and smoke
tests are available. Never advertise an architecture solely because build metadata lists it.
Publishing and package visibility are external writes: ask the maintainer before the first push.

Keep root `docker-compose.yml` for source development. Add standalone `compose.published.yaml` with
no build context, plus the same two-service health/dependency/volume wiring. Make README source-start
command exactly:

```bash
docker compose up --build --force-recreate -d
```

A user copying published Compose content into `compose.yaml` can run that same command; explain that
`--build` has no build target there. For the checked-in filename use explicit `-f` and otherwise the
same flags. Require `ASSAY_IMAGE` containing a verified released tag/digest rather than inventing one
in the plan. No registry availability claim until anonymous pull and smoke tests succeed.

Linux quickstart is the primary walkthrough. Keep a complete linked PowerShell equivalent. Explain:
admin vs ingest credentials, encryption key generation/persistence, Compose-only Postgres variables,
host `localhost` vs container `postgres`/`assayd` URLs, optional judge configuration, Linux
`host.docker.internal:host-gateway` for host-local models, database backups, safe upgrades, and why
`down -v` destroys data. Bind development ports to loopback; do not expose Postgres in the published
example. A fresh install must not require real judge credentials merely to view traces.

## 10. Release bar and exclusions

M7 is complete only after a real embedded-build browser test demonstrates setup → trace → conversation
and waterfall → score with fake judge → dataset edit → run → per-item result → comparison, and Python
and CLI exercises verify the same outcomes. Include dirty-form confirmation, duplicate/conflict,
expired credentials, retention, provider failures, large payloads, keyboard-only and narrow viewport.
Use synthetic content only in screenshots and a local fake judge in CI.

Performance fixture: 1,000-span trace, 50 message-bearing calls, 100KiB message payload, 1,000-case
dataset, and 10,000 trace summaries. Measure a baseline on a recorded machine before setting release
numbers. Initial browser targets: selected-span update p95 below 100ms; no task above 200ms during
selection after initial parsing; do not load all datasets/runs to render one page. If profiling shows
row rendering exceeds the budget, add windowing in that task, not a universal virtualization framework.
These are proposed acceptance budgets, not measured current performance or throughput claims.

Excluded: OIDC/RBAC/SSO, custom scorer authoring, prompt playground, sessions spanning multiple traces,
provider auto-instrumentation, arbitrary multimodal replay, binary protobuf/gRPC, `trace_selection`,
model price catalogs, live streaming transport, dashboards/saved-view engines, and agent execution.
Do not fake cost totals when no actual cost has been recorded. These can be separately designed after
this workflow is genuinely usable.
