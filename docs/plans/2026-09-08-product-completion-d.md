# M7D Evaluation Review and SDK Parity Implementation Plan

> **For agentic workers:** Use `executing-plans` task-by-task, or the available
> `superpowers:subagent-driven-development` skill. No live judge calls in ordinary tests.

**Goal:** Explain each evaluation outcome, compare like-for-like cases, and expose the same workflows
through a small typed Python API and CLI.
**Architecture:** Run-item pages return immutable source snapshots with bounded score enrichment.
Comparison is a domain/store read, not a browser join. Structured capture stays an opt-in OTel helper.
**Tech Stack:** Go/Postgres/Huma; React/TypeScript; Python/OTel/httpx/uv/pytest/Hypothesis.
**Spec:** [`../specs/2026-09-08-product-completion-design.md`](../specs/2026-09-08-product-completion-design.md), §§6.4,8,10.

## Global Constraints

Apply the [master constraints](2026-09-08-product-completion.md#global-constraints).
A3 is mandatory before D1/D3; B1/B6 before D2; C6 before D5; D4 shares C1 wire fixtures.
Use Python 3.13 for development, but **preserve the SDK's declared Python >=3.10 support and CI matrix**.
Use 3.10-compatible syntax/types unless a separately approved support-floor change is made. Do not
quietly switch ruff/ty target versions or packaging metadata while adding helpers.

## D1. Return inspectable run-item pages and classify failures correctly

**Files**
- Modify: `assayd/internal/api/eval_runs.go`, `assayd/internal/domain/evaluation_models.go`,
  `assayd/internal/store/eval_runs.go`, `assayd/db/queries/eval_runs.sql`.
- Test: `assayd/internal/api/eval_run_output_test.go`; create
  `assayd/internal/store/run_item_results_integration_test.go`.
- Generate: sqlc, OpenAPI, TypeScript.

**Consumes:** A3 `snapshot`/`snapshot_origin`, existing item generated fields and `Score` provenance.
**Produces:** `EvalRunItem.Scores []Score`; run-item response adds `scores: ScoreResponse[]` (empty array
when absent). Page envelope/cursor remains unchanged. No request per item or scorer for a table page.
Add `EvaluationService.GetEvalRunItem(ctx, runID, itemID uuid.UUID) (EvalRunItem,error)` and a matching
repository read; expose admin-only `GET /v1/runs/{id}/items/{itemId}`, operation `get-eval-run-item`.
This direct scoped read uses the same snapshot/score conversion and returns 404 for wrong run/item.

- [ ] Test a run with three outcomes: scored pass, scored fail at value 0, execution failure with no
  score. Assert each GET `/v1/runs/{id}/items` item includes its original snapshot, current outcome,
  generated output if any, and correct score provenance; failed execution must return `scores:[]`.
  Cover direct item GET, wrong-parent 404, missing item 404, and missing admin 401.
- [ ] Test pagination boundary with scores crossing old score-list page boundaries; delete source case
  and reread result; two scorers per item; canceled pending case; legacy-backfill indicator; Unicode
  snapshot context; no leaking encrypted/clear judge keys. Run tests before implementing enrichment.
- [ ] Fetch the requested run-item page first, then all scores for that page's item IDs with one SQL
  query scoped by run ID. Group in a local map and attach in stable scorer/time/ID order. Do not
  load the entire run, and do not depend on a live dataset item join.
- [ ] Keep source output distinct from generated output and judged evidence. If persisted judged fields
  are absent for old offline scores, show snapshot/generated values as such, not “exact judge input.”
  Score details/model/prompt/threshold are actual persisted values, never current scorer settings.
- [ ] Add/verify API enum response for item status, per-item errors without captured content, and
  origin enum from A3. Regenerate and run `go test -race ./internal/api ./internal/store -run
  'RunItem|EvalRun|Snapshot'`; lint/types; commit `Expose evaluation case snapshots and score results`.

**Gate:** The UI/SDK can render one result page with one API operation without reconstructing history.

## D2. Build evaluation item review, rerun, cancel, delete, and export

**Files**
- Modify: `web/src/features/runs/run-detail.tsx`, `runs-page.tsx`, `create-run-dialog.tsx`.
- Create: `web/src/features/runs/run-items.tsx`, `run-item-detail.tsx`, `run-items.test.tsx`,
  `run-actions.tsx`, `run-actions.test.tsx`.
- Modify: `web/src/components/score-result.tsx`, create `score-result.test.tsx`.

**Consumes:** A1 active-state helper, A5 run delete, B6 paged dataset selection, D1 enriched items.
**Produces:** Run review links `/apps/:appId/runs/:runId?item=<case-id>`; no captured content in query.

- [ ] Add MSW tests for aggregate plus item table, failed quality score versus execution error, empty
  partial aggregates while running, null generated output, source case deleted, 0 score, missing
  requested scorer, and long rationale/details. Score display must satisfy:

```tsx
// Add to score-result.test.tsx with a full generated ScoreResponse fixture.
expect(screen.getByText("0.00")).toBeInTheDocument();
expect(screen.getByText(/Fail/)).toBeInTheDocument();
expect(screen.queryByText("Not scored")).not.toBeInTheDocument();
```

  Build the fixture with `satisfies ScoreResponse`, including scorer, threshold, passed, details,
  prompt ID, judge model/provider/tokens, ID, created_at; do not cast incomplete objects to the type.
- [ ] Render run summary plus Cases section: external ID/question, execution status, scorer values,
  tokens and expand/detail link. Detail shows original snapshot input/reference/context, recorded or
  generated output, errors, score rationale, groundedness claim verdicts with chunk references,
  correctness reference facts, and raw fallback for unfamiliar details. Preserve provenance labels.
- [ ] Poll current run and currently visible item page while active. Pause hidden, abort on run/page
  change, stop terminal. Do not append the same items each poll, reload all pages per second, or
  combine stale items from another run. On terminal refresh the visible page once and update progress.
  Paginate normally after completion. An item deep link uses D1's direct scoped item GET to open its
  detail independently of the current table page; do not search through pages to locate a known ID.
- [ ] Add “Run again” opening the existing creation form prefilled with dataset, mode, and scorers,
  with editable name. Copy reads **current dataset/current configuration**, clearly stated. It is
  not replay of the old snapshot or retry of only failed cases. Backend still validates requirements.
- [ ] Cancellation uses A1 state and confirmation; terminal Delete uses A5, with run name confirmation
  and warning that scores are deleted. A 409 refreshes and explains the state; failed DELETE leaves
  current data intact. Do not immediately recreate or delete a run after timeout uncertainty.
- [ ] Export complete item results as JSONL following cursors with abort/repeated-cursor guards and
  explicit max 10,000 items/50MiB browser cap; larger results use CLI. Include actual snapshot/outcome/
  score evidence, not just aggregates. Export uses synthetic fixtures in tests; no spreadsheet formula
  risk because no CSV feature is added.
- [ ] Run item/actions/run polling/score tests, frontend gates; commit
  `Review individual evaluation outcomes and manage run lifecycle`.

**Gate:** A run mean is no longer a dead end. A user can identify which case failed and why, without
mistaking judge/network failure for model-quality failure.

## D3. Compare two terminal runs on matched historical cases

**Files**
- Create: `assayd/internal/domain/run_comparison.go`, `run_comparison_test.go`,
  `assayd/internal/store/run_comparison.go`, `assayd/db/queries/run_comparison.sql`,
  `assayd/internal/api/run_comparison.go`, `run_comparison_test.go`.
- Modify: `assayd/internal/api/router.go`, `assayd/internal/app/app.go`, and OpenAPI fixture wiring.
- Create: `web/src/features/runs/run-comparison.tsx`, `run-comparison.test.tsx`.
- Modify: `web/src/features/runs/runs-page.tsx`, `web/src/app/router.tsx`; generate sqlc/API.

**Interfaces:** Domain `RunComparisonService` receives `RunComparisonRepository`; method
`Compare(ctx context.Context, input RunComparisonQuery) (RunComparisonPage,error)`.
`RunComparisonQuery` contains `BaselineID`, `CandidateID` uuid.UUID, `Scorer string`, `Limit int`,
`Cursor *uuid.UUID` (ascending case-ID pagination of a fixed terminal pair). Huma path `{id}` is
baseline; query `other_run_id` is candidate. Reply contract:

```ts
type ComparisonKind = "matched" | "changed_case" | "baseline_only" |
  "candidate_only" | "unscored";
type ComparisonRow = {
  dataset_item_id: string;
  kind: ComparisonKind;
  baseline: EvalRunItemResponse | null;
  candidate: EvalRunItemResponse | null;
  delta: number | null;
};
type ComparisonPage = {
  baseline_run_id: string; candidate_run_id: string; scorer: string;
  items: ComparisonRow[]; next_cursor?: string;
  summary: {
    n: number; mean_delta: number | null;
    matched: number; changed_cases: number; baseline_only: number;
    candidate_only: number; unscored: number;
  };
  warnings: string[];
};
```

  Types above are API output contracts to generate, not handwritten frontend substitutes.
  Extend repo boundary with `GetEvalRun` and one read for comparison rows/summary; service owns
  same-app/dataset/distinct/terminal/scorer validation. Cursor is base64 of validated case UUID,
  bound to baseline/candidate/scorer in the encoded envelope; mismatched cursor is 422.
  Default limit is 100, maximum 500; fetch limit+1 and return a cursor only when another row exists.

- [ ] Write pure classification/aggregation tests with these exact expected outcomes:

| Baseline | Candidate | Case input/reference | Kind / delta |
|---|---|---|---|
| score .4 | score .9 | equal | matched / +.5 |
| score .8 | score .2 | equal | matched / -.6 |
| score 0 | score 0 | equal | matched / 0 |
| no case | score .9 | absent | candidate_only / null |
| score .4 | no case | absent | baseline_only / null |
| score .4 | execution failure | equal | unscored / null |
| score .4 | score .9 | different | changed_case / null |

- [ ] Implement integration tests for identical case IDs surviving deletion/reimport, changed external
  IDs not used as join keys, incompatible apps/datasets 422, same run 422, nonterminal 409, absent
  requested scorer 422, zero scores, missing scores, invalid/repeated cursor, and pagination. No
  delta treats null as zero. `summary.n` includes only matched non-null deltas across **all pages**.
- [ ] SQL full-outer-joins each run's immutable items by dataset_item_id, then left joins the selected
  scorer score rows (one deterministic latest row if more than one exists). Compare JSONB
  snapshot_input and nullable expected_output using `IS NOT DISTINCT FROM`; recorded/generated
  output changes are the thing under evaluation, not a reason to exclude a case. Context changes
  remain shown as a warning; context is not silently discarded from result details.
- [ ] Aggregate in Postgres over the same classification predicate, not by averaging each page's mean.
  Keep response rows paginated and set-based; do not return all run evidence at once. Warn when score
  judge model/provider/prompt/threshold differs. No confidence interval, win probability, or causal
  “improved model” claim is justified by these two runs alone.
- [ ] Register/read the API as admin-only. UI compare selection accepts exactly two terminal runs,
  assigns baseline/candidate explicitly, and opens `/apps/:appId/runs/compare?baseline=...&candidate=...`
  (register static route before dynamic ID route). Select shared scorer; show paired means/deltas,
  matched n and excluded buckets, per-case diff and evidence. Display mismatched judge warning.
- [ ] Add UI tests for +/−/zero/missing deltas, no matched samples, swapped pair sign, retained filters
  on refresh, same-app validation, mobile side-by-side stacking, and per-case keyboard expansion.
- [ ] Run comparison domain/store/API tests + frontend tests/gates; regenerate; commit
  `Compare evaluation runs using matched snapshot cases`.

**Gate:** Comparison explains its denominator and excludes changed/unscored cases honestly. It never
compares two page-level averages or silently pairs by row position.

## D4. Add minimal structured-message capture and trace identifiers to the SDK

**Files**
- Create: `clients/python/assay/src/assay/messages.py`,
  `clients/python/assay/tests/test_messages.py`.
- Modify: `clients/python/assay/src/assay/tracing.py`, `conventions.py`, `__init__.py`.
- Extend: `clients/python/assay/tests/test_tracing.py`, `test_conventions.py`, `test_exporter.py`.
- Modify: `docs/semantic-conventions.md`, `clients/python/assay/README.md`.

**Consumes:** Current private provider and explicit AssaySpan capture helpers, C1's JSON wire fixtures.
**Produces:** Public `Message`, `TextPart`, `ToolCallPart`, `ToolResultPart` TypedDicts exported from
`assay`; `AssaySpan.set_messages(*, input: Sequence[Message] | None = None,
output: Sequence[Message] | None = None, redact: Callable[[object],object] | None = None) -> None`;
read-only active-context `trace_id: str`, `span_id: str` (32/16 lowercase hex).

- [ ] Use the official schemas inspected during planning, not an invented provider format:
  [input](https://github.com/open-telemetry/semantic-conventions-genai/blob/b5d8440f6f126738fd50f927752cd669772c517b/model/gen-ai/gen-ai-input-messages.json),
  [output](https://github.com/open-telemetry/semantic-conventions-genai/blob/b5d8440f6f126738fd50f927752cd669772c517b/model/gen-ai/gen-ai-output-messages.json).
  The old semantic-conventions repository now redirects GenAI documentation to this repository.
  Record this commit as the message mapping reference; review any newer changes rather than blindly
  changing the entire SDK semconv contract. Input/output require role and ordered parts. Tool request
  is `type:tool_call,name,arguments?,id?`; response is `type:tool_call_response,response,id?`.
- [ ] Add `messages.py` TypedDicts with required fields and optional ID/arguments using 3.10-compatible
  TypedDict inheritance. `Message` requires role (str, preserving extension roles such as developer)
  and parts; optional name. Start with text and client tool parts only; UI can show other emitters'
  generic/attachment/server-tool parts raw without the SDK promising constructors for all of them.
- [ ] Add behavior tests through the existing in-memory-exporter setup. Main example:

```python
with assay.span("chat", scorable=True) as current:
    current.set_messages(
        input=[{"role": "user", "parts": [{"type": "text", "content": "What is Assay?"}]}],
        output=[{"role": "assistant", "parts": [{"type": "text", "content": "An eval tool."}]}],
    )
    trace_id = current.trace_id
    span_id = current.span_id
assert len(trace_id) == 32
assert len(span_id) == 16
```

  Assert exported `gen_ai.input.messages`/`output.messages` decode to the supplied structure, not an
  escaped message array inside one user content string. Test `capture=False` leaves decorator capture
  off while this explicit setter captures only supplied values; no global provider replacement.
- [ ] Implement validation/serialization as free functions in messages.py. Both omitted is ValueError;
  one omitted leaves that side unchanged; explicit empty list stores `[]`. Validate role/parts/type,
  JSON-safe tool values (finite numbers, string-keyed maps), recursion/cycles and UTF-8 size. Redact
  the entire supplied message collection before serializing, then validate the redacted result.
  Serialize complete JSON, apply the existing byte budget per input/output attribute including JSON
  envelope, and reject overflow without content in the exception. Never truncate a JSON token/tool ID.
- [ ] Serialize/validate **both** supplied sides before setting any span attribute so output failure
  cannot leave a newly updated input with an old output. Explicit setters raise on bad user input;
  the existing decorator remains fail-safe for capture errors. Redactor exceptions become a
  content-free error with `raise ... from None`; do not include exception repr containing secrets.
- [ ] Add Hypothesis tests for Unicode and JSON-safe text/tool payloads: decode(encode(messages)) ==
  messages under cap, encoded bytes <= cap, mutations do not change input objects, invalid/over-limit
  calls leave attributes unchanged. Test secret-bearing redactor failure, NaN/infinity, empty roles,
  unknown part types, inactive context IDs, nested async spans sharing trace ID but distinct span IDs,
  flush/shutdown, and two concurrent tasks without context leakage.
- [ ] Verify backend scoring on a final assistant text-parts output. Tool-only outputs remain
  ineligible for text scorers; do not change the “one scorable final span, first output candidate”
  rule just to make a fixture pass. Document decorator arguments are function inputs, not inferred
  provider messages; streaming/generator consumption is not automatic.
- [ ] Run messages/tracing/conventions/exporter pytest files, ruff/ty, and existing supported Python
  CI matrix; commit `Capture structured GenAI messages with minimal SDK helpers`.

**Gate:** Existing three-env-var initialization remains sufficient. Users do not need raw OTel
attribute names for a normal conversation or a trace ID, and no new runtime dependency is required.

## D5. Extend typed Python APIs and CLI to match browser workflows

**Files**
- Modify: `clients/python/assay/src/assay/{client,models,_parsing,cli}.py`.
- Test: `test_client_management.py`, `test_client_evaluations.py`, `test_client_traces.py`,
  `test_client_workflows.py`, `test_models.py`, `test_cli.py`, `test_analytics.py`.
- Create: `clients/python/assay/tests/test_product_workflows.py`.

**Interfaces:** Keep existing resource names/methods. Add these typed methods:

```text
datasets.update(dataset_id, *, name=None, description=None, clear_description=False) -> Dataset
datasets.get_item(dataset_id, item_id) -> DatasetItem
datasets.replace_item(dataset_id, item_id, *, item: DatasetItemInput) -> DatasetItem
datasets.delete_item(dataset_id, item_id) -> None
runs.delete(run_id) -> None
runs.get_item(run_id, item_id) -> EvalRunItem
runs.compare(run_id, other_run_id, *, scorer, limit=100, cursor=None) -> RunComparisonPage
traces.delete(trace_id) -> None
traces.eligibility(trace_id) -> tuple[ScoringEligibility, ...]
traces.list(... existing keyword params ..., q=None, scorer=None, passed=None) -> Page[Trace]
```

  Existing `datasets.from_trace` keeps its public signature but delegates to C6's one endpoint.
  Remove `_regression_item` if it becomes unused; no permanent old/new orchestration switch.
  Extend `EvalRunItem` parsing with required snapshot/origin/scores for the new contract; introduce
  typed immutable Comparison/Eligibility dataclasses. Extend Trace with score_summaries.

- [ ] Write HTTP-boundary tests for exact method/path/auth/JSON/query values, null clear vs omission,
  URL quoting, wrong parent 404, duplicate 409, disabled eligibility, comparison pagination, and
  content-free errors. Replace `from_trace` tests expecting its old three-call choreography with
  public returned-case/error behavior. Do not mock the new service logic in backend tests.
- [ ] Decide trace request auth from available credentials: prefer explicitly supplied project key
  for existing list/get/score/reference methods; otherwise use admin for allowed routes. If both are
  supplied, project scope remains authoritative for those calls. Admin-only delete/eligibility use
  admin even when a project key exists. Never retry an unauthorized project call with admin silently.
- [ ] Implement full PUT using all seven fields; `DatasetItemInput` output/reference/external ID None
  becomes JSON null, context defaults [], metadata defaults {}, input.question required. PATCH fields
  omitted by default, explicit clear flag supported. Validate flags without echoing secrets/payloads.
- [ ] Expand thin CLI groups (existing singular `run` remains canonical):

```text
assay projects get|update|delete PROJECT_ID
assay keys list PROJECT_ID
assay keys revoke PROJECT_ID KEY_ID --yes
assay apps get|update|delete APP_ID
assay apps clear-endpoint APP_ID --yes
assay datasets list APP_ID
assay datasets get|update|delete DATASET_ID
assay datasets items list DATASET_ID
assay datasets items get DATASET_ID ITEM_ID
assay datasets items add DATASET_ID --file case.json
assay datasets items replace DATASET_ID ITEM_ID --file case.json
assay datasets items delete DATASET_ID ITEM_ID --yes
assay datasets export DATASET_ID --format jsonl
assay scorers list APP_ID
assay run list APP_ID
assay run get|items|scores|cancel|delete RUN_ID
assay run item RUN_ID ITEM_ID
assay run compare BASELINE_ID CANDIDATE_ID --scorer groundedness
assay run export RUN_ID --format jsonl
assay traces reference TRACE_ID --file reference.txt
assay traces eligibility TRACE_ID
assay traces delete TRACE_ID --yes
```

  Keep existing create/import/from-trace/score/export/metrics/watch commands. `projects update` accepts
  `--name` and `--judge-config-file`/`--clear-judge-config`; `apps update` accepts `--file` for the exact
  application PATCH body; dataset update accepts `--name`, `--description`, `--clear-description`.
  Scorer set extends `--enabled/--disabled`, `--judge-config-file`, `--prompt-template-id` so UI settings
  are achievable by agents; `apps set-endpoint` already exists. No secrets in CLI flags or argv.
- [ ] Every DELETE/revoke/clear-endpoint command requires `--yes` in noninteractive mode. `run cancel`
  is explicit but does not delete completed results. stdout remains JSON/JSONL, errors/progress stderr,
  nonzero errors/gate failures. Export follows all pages with fixed query windows and repeated-cursor
  guards. Commands delegate to typed resource methods; no business decisions duplicated in argparse.
- [ ] Add CLI tests for no --yes causes zero requests; file errors; JSONL multi-page outputs; auth
  fallback choice; comparison no samples; zero-valued score preserved; cancellation 409. Capture
  outputs and assert no secret file values appear in errors. Large client.py/cli.py may be split by
  resource/command responsibility if touched functions exceed limits; do not change public imports.
- [ ] Run named SDK tests and ruff/ty; rebuild package and verify exports via test_package.py; commit
  `Expose product workflows through the typed client and CLI`.

**Gate:** There is no browser-only mutation, and the SDK does not force admin credentials into trace
emitters. CLI docs describe actual commands, not planned ones.

## D6. Verify the SDK-to-trace-to-evaluation loop without paid services

**Files**
- Create: `clients/python/assay/tests/test_product_acceptance.py`.
- Modify: `clients/python/assay/README.md`, `examples/python-qa/README.md`,
  `examples/python-qa/` actual tracing call sites after reading them.
- Modify: `.claude/skills/assay/SKILL.md` using the skill-maintainer skill at execution time.
- Reference: `clients/python/assay/tests/test_live_workflow.py` and existing fake Go judge/target fixtures.

**Interface:** Acceptance receives an explicitly disposable endpoint and admin credential from the
E1 harness; creates only a synthetic project whose ID is tracked and cleaned. Never auto-target
`http://localhost:8080` for destructive acceptance. Existing opt-in paid live test remains separate.

- [ ] Write a deterministic test: Client creates project/app/key/scorers → SDK init from env → explicit
  message+retrieval spans → flush → find trace by OTel ID via C5 q → eligibility → score → poll →
  from_trace import → replace case → create run → read snapshots/items → second run → comparison.
  Validate all numbers against fixed fake-judge fixtures, not any particular real model.
- [ ] Include capture-off trace (metadata but no messages), missing-reference eligibility, fake judge
  503/retry/permanent failure, duplicate import 409, changed dataset source with old run preserved,
  cancel/terminal delete, revoke key then ingest denied. Failed quality score and execution failure
  must yield different result objects and CLI exit behavior.
- [ ] Update example to use structured helper **only where it replaces manual message serialization**;
  do not add provider auto-instrumentation, unrelated framework changes, or redundant decorators that
  double count spans. Use public trace_id helper instead of private provider internals if applicable.
- [ ] Update SDK docs/agent skill with actual lifecycle commands, nullable item replacement semantics,
  admin/project distinction, original versus backfilled evidence, current-dataset rerun warning, and
  comparison exclusions. Keep minimum quickstart under 15 executable Python lines excluding env setup.
- [ ] Run acceptance against E1 disposable stack and the named existing SDK tests; run ruff/ty and
  skill-maintainer checks for changed skill. Commit `Verify and document the full evaluation workflow`.

**M7D demo:** Reviewer can explain a failed case, compare two runs without hidden denominator changes,
and reproduce the workflow through Python/CLI with synthetic judge responses and no manual SQL.
