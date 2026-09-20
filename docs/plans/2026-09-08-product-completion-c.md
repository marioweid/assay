# M7C Conversation-First Trace Workbench Implementation Plan

> **For agentic workers:** Use `executing-plans` task-by-task, or the available
> `superpowers:subagent-driven-development` skill. Pure normalization/geometry tests precede rendering.

**Goal:** Make conversations and timing understandable, searchable, and directly usable for evaluation.
**Architecture:** Normalize captured OTLP data into a frontend view model without modifying stored
telemetry; use existing detail spans and new set-based list summaries. Domain services own eligibility
and trace-to-case import, shared by UI and Python.
**Tech Stack:** React/TypeScript/SVG/Vitest; Go/Huma/Postgres; existing generated API client.
**Spec:** [`../specs/2026-09-08-product-completion-design.md`](../specs/2026-09-08-product-completion-design.md), §§6.2–6.3,7–8.

## Global Constraints

Apply the [master constraints](2026-09-08-product-completion.md#global-constraints).
No Gantt library, full-trace fetch per list row, global text deduplication, unsafe Markdown, or provider
introspection. C1/C3 can start independently; C4 needs A6 and B1/B2; C6 needs A4/A5 and B5.

## C1. Define one deterministic captured-conversation view model

**Files**
- Create: `web/src/features/traces/conversation-model.ts`, `conversation-model.test.ts`.
- Create: `web/src/features/traces/fixtures/conversation.json` (synthetic shared wire vectors).
- Reference: `assayd/internal/domain/trace_scoring.go`, `docs/semantic-conventions.md`,
  `clients/python/assay/src/assay/conventions.py`, current `captured-content.tsx`.

**Interfaces:** Public pure functions and types below live in `conversation-model.ts`. This is a
presentation model, not a new transport/schema or a scoring implementation.

```ts
export type SpanKey = `${string}@${string}`;
export type CaptureState = "missing" | "empty" | "present" | "malformed";
export type ConversationPart =
  | {kind: "text"; text: string}
  | {kind: "tool-call"; id: string | null; name: string; arguments: unknown}
  | {kind: "tool-result"; id: string | null; result: unknown}
  | {kind: "unsupported"; type: string; raw: unknown};
export type ConversationMessage = {
  key: string;
  role: string;
  direction: "input" | "output";
  parts: ConversationPart[];
  raw: unknown;
};
export type CapturedMessages = {
  state: CaptureState;
  messages: ConversationMessage[];
  diagnostics: string[];
  raw: unknown;
};
export type ConversationCall = {
  spanKey: SpanKey;
  span: SpanResponse;
  input: CapturedMessages;
  output: CapturedMessages;
};
// Imports SpanResponse from generated types.
export function spanKey(span: SpanResponse): SpanKey;
export function normalizeMessages(value: unknown, direction: "input" | "output"): CapturedMessages;
export function conversationCalls(spans: readonly SpanResponse[]): ConversationCall[];
```

- [ ] Pin the wire fixtures to the repository's existing `content` strings and standard text `parts`
  forms. Read current official OTel GenAI input/output message JSON schemas for tool parts, record
  the commit URL in semantic conventions, and add those **exact** fixtures. Do not conflate provider
  `tool_calls` JSON with OTel `parts` without a documented explicit mapping. Unknown forms remain raw.
  Planning verified the moved GenAI repository and commit in D4: text uses `type:text,content`;
  tool requests use `type:tool_call,name,arguments?,id?`; results use
  `type:tool_call_response,response,id?`. Map these explicitly to the view-model kinds below.
- [ ] Add runnable table-driven tests, starting with:

```ts
test("reads ordered standard text parts", () => {
  const result = normalizeMessages(JSON.stringify([
    {role: "user", parts: [{type: "text", content: "first"}, {type: "text", content: "second"}]},
  ]), "input");
  expect(result.state).toBe("present");
  expect(result.messages[0]?.parts).toEqual([
    {kind: "text", text: "first"}, {kind: "text", text: "second"},
  ]);
});
test("distinguishes absent, empty, and malformed capture", () => {
  expect(normalizeMessages(undefined, "input").state).toBe("missing");
  expect(normalizeMessages([], "input").state).toBe("empty");
  expect(normalizeMessages("[broken", "input").state).toBe("malformed");
});
```

- [ ] Test structured-array and JSON-string equivalence; system/developer/user/assistant/tool roles;
  content string, ordered text parts, tool-only messages, unknown role/part; null/non-array values;
  empty content; multiple assistant candidates remain distinct; duplicate identical user messages
  within a call remain distinct; malicious HTML remains plain data. No coercion to `[object Object]`.
- [ ] Implement explicit runtime type guards and bounded, non-recursive message-part parsing. A
  malformed message contributes a diagnostic plus raw fallback without hiding valid siblings. Stable
  keys combine source span key, direction, and original message/part index; content is not identity.
- [ ] Flatten span children once iteratively with visited identities (A6 ensures valid server tree),
  select message-bearing spans, sort by actual start time and stable key. Retrieval-only spans remain
  available as context, not fabricated chat messages. Selecting a span later will filter by **exact**
  spanKey, not recursively include descendants.
- [ ] Run `(cd web && pnpm test src/features/traces/conversation-model.test.ts)` then lint/types.
  Commit `Normalize captured GenAI conversations with source provenance`.

**Gate:** The data model preserves roles, content order, unknown data, duplicates, and source spans.
It never guesses a cross-call chronology for duplicated prompt histories.

## C2. Render readable conversation, context, reference, and tool evidence

**Files**
- Create: `web/src/features/traces/conversation-view.tsx`, `message-content.tsx`,
  `tool-activity.tsx`, `retrieval-context.tsx`, `conversation-view.test.tsx`.
- Modify: `web/src/components/json-view.tsx`, with `json-view.test.tsx` for copy failure.
- Replace/delete after C4 switches consumers: `captured-content.tsx` and its obsolete tests.

**Interfaces:** `ConversationView({calls, selectedSpanKey, onSelectSpan})` consumes C1 call groups.
`onSelectSpan: (key: SpanKey | null) => void`. `RetrievalContext({spans})` reads standard documents and
flattened Assay chunks for display. Backend remains the authority on score eligibility.

- [ ] Render fixtures with user/assistant exchange, multi-call repeated histories, tool call/result,
  context+reference, malformed data, and missing capture. Assert by role/accessible name; selecting
  source links invokes selection using stable span key, never message text. A selected parent must not
  show child's messages.
- [ ] Default group selection: sole scorable message-bearing span; otherwise earliest message-bearing
  span. Show that call expanded, additional model calls collapsed with model/time/token labels. Do
  not concatenate unrelated branches. Within a call show input order then output candidates in their
  recorded order. Context/reference are labeled evidence sections, not conversation turns.
- [ ] Implement text wrapping, code/preformatted blocks, JSON expand/copy, role labels/icons and tool
  arguments/result disclosure. Match tool results only when an explicit call ID identifies a unique
  earlier call in scope; duplicate/missing IDs display unpaired activity rather than a false join.
- [ ] If choosing Markdown, first justify and pin one safe renderer with no raw HTML extension. Test
  script tags, onerror images, javascript/data/file URLs, remote images, SVG payloads, raw HTML,
  and large code fences. Default remote images are attachment placeholders and links use safe
  schemes (`https`, `http`, `mailto`) with no auto-navigation. No Mermaid execution inside captures.
  A text/code/JSON implementation without Markdown is an acceptable initial reviewed implementation.
- [ ] Show capture states exactly: “Content not captured” with setup link, “Captured empty messages,”
  “Could not parse captured messages” with safe raw disclosure, and visible truncation markers when
  present in source. Do not assert `capture=False` caused missing fields: third-party emitters may
  simply not send them. Never invent hidden reasoning or claim tool spans are user messages.
- [ ] Read retrieval documents as string or `{id,text,score?,source?}` objects; flatten chunks only
  when standard documents are absent to avoid duplicate presentation. If both differ, show an
  inconsistency diagnostic with raw values; do not silently hide it. Truncate display, not copied
  source. At 100KiB do not render a full expanded JSON tree on initial paint.
- [ ] Test copy denial/rejection uses an inline status without leaking payload; full original text is
  available by user action. Run conversation/JSON tests and frontend gates; commit
  `Render conversation and tool evidence for GenAI traces`.

**Gate:** A person can read the generated answer and inspect its input/context/tools without opening
Attributes. Raw evidence stays reachable and unknown formats are not discarded.

## C3. Implement accurate and defensive waterfall geometry

**Files**
- Create: `web/src/features/traces/span-timing.ts`, `span-timing.test.ts`,
  `span-waterfall.tsx`, `span-waterfall.test.tsx`.
- Modify: `web/src/features/traces/span-tree.tsx` only to share ordering/selection conventions.

**Interfaces:** Keep absolute nanoseconds as bigint; only convert relative deltas to numbers.

```ts
export type SpanTimingInput = {key: SpanKey; startTime: string; endTime: string};
export type SpanTiming = {
  key: SpanKey; offsetMs: number; durationMs: number;
  leftPercent: number; widthPercent: number; invalid: boolean;
};
export type TimingLayout = {durationMs: number; rows: SpanTiming[]};
export function parseTimestampNs(value: string): bigint | null;
export function layoutSpanTiming(spans: readonly SpanTimingInput[]): TimingLayout;
```

- [ ] Write geometry tests before JSX:

```ts
test("positions overlapping calls by time rather than sequential duration", () => {
  const layout = layoutSpanTiming([
    {key: "root@t", startTime: "2026-09-08T00:00:00Z", endTime: "2026-09-08T00:00:01Z"},
    {key: "a@t", startTime: "2026-09-08T00:00:00.1Z", endTime: "2026-09-08T00:00:00.6Z"},
    {key: "b@t", startTime: "2026-09-08T00:00:00.2Z", endTime: "2026-09-08T00:00:00.7Z"},
  ]);
  expect(layout.rows[1]).toMatchObject({leftPercent: 10, widthPercent: 50});
  expect(layout.rows[2]).toMatchObject({leftPercent: 20, widthPercent: 50});
});
test("keeps fractional milliseconds", () => {
  const start = parseTimestampNs("2026-09-08T00:00:00.000100Z");
  const end = parseTimestampNs("2026-09-08T00:00:00.000400Z");
  expect(start).not.toBeNull();
  expect(end).not.toBeNull();
  if (start !== null && end !== null) expect(Number(end - start) / 1e6).toBeCloseTo(0.3);
});
```

- [ ] Add zero duration, all invalid, reversed time, negative/offset timezone, equal timestamps,
  child beyond parent extent, rootless retained spans, and very long durations. Assert every returned
  percentage is finite in [0,100], `left+width <=100` within floating tolerance, and input order is not
  mutated. A malformed timestamp is invalid, never interpreted as current time.
- [ ] Parse RFC3339 with up to nine fractional digits: validate the calendar/offset, parse whole seconds
  safely, append padded fractional nanoseconds using bigint. Reject overflow and unrecognized dates;
  do not use `Date.parse` on the full fractional value and lose sub-ms precision. Axis origin is min
  valid start across visible **and collapsed** spans; end is max valid end. Collapsing must not rescale.
- [ ] Geometry algorithm: `offset=(start-origin)`, `duration=max(0,end-start)`, relative fractions of
  total. Empty/all-zero span extent uses a finite display domain with zero-duration markers, while
  reported real duration stays zero. Inconsistent timestamps get an “Invalid timing” text label.
- [ ] Render accessible indented rows with names, statuses, duration and an SVG bar/marker plus labeled
  axis. SVG numeric x/width attributes avoid inline CSS under the current CSP. One shared horizontal
  scroller keeps axis and bars aligned; name column remains readable. Keyboard focus/Enter selects,
  expander button exposes aria-expanded, and text describes offset/duration without relying on color.
- [ ] Tests select by role/name, exercise expand/collapse without changing geometry, zero marker, and
  a 1,000-row fixture. Measure before adding windowing. If needed, virtualize only these rows while
  keeping selected row visible and accessible text fallback; do not add general grid infrastructure.
- [ ] Run timing/waterfall tests, frontend gates and embedded-browser CSP check. Commit
  `Add an accurate accessible span waterfall`.

**Gate:** Concurrent child spans overlap visually, sub-ms spans do not become fake zeros, and expanding
rows never alters the timing scale or drops the selected span.

## C4. Integrate the trace workbench with stable URL selection and refresh

**Files**
- Modify: `web/src/features/traces/trace-detail.tsx`, `traces.test.tsx`, `traces-stale.test.tsx`.
- Create: `web/src/features/traces/trace-overview.tsx`, `trace-selection.ts`,
  `trace-selection.test.ts`, `use-trace-detail.ts`, `use-trace-detail.test.tsx`.
- Remove: old `captured-content.tsx` and replace its tests with C1/C2 behavior tests.

**Interfaces:** Trace URL query uses `tab=overview|attributes|events|scores`,
`span=<otel_span_id>`, `span_start=<RFC3339 time>`. Pair resolves to C1 `SpanKey`; no numeric database
ID coercion. Unknown tab falls back to overview; missing span shows a notice and trace summary.
`useTraceDetail(appId, traceId)` returns `{trace, error, loading, refresh, refreshing}`.

- [ ] Write integration tests: default shows conversation without selecting a span, select waterfall
  row → exact span conversation, click message source → same selected row, trace summary clears both,
  refresh/back/forward preserves tab/selection, invalid link cannot render another app's trace.
- [ ] Implement spec layout: compact summary header, conversation primary left pane, waterfall right
  pane, existing raw/scores tabs preserved. At <1024px stack conversation before timing. Reuse B1
  tokens/components; format duration/tokens centrally only if already used in three places.
- [ ] Keep trace response in the data hook and selection as a derived URL lookup, not a second stale
  SpanResponse object in state. Use replace-history for high-frequency span changes and push-history
  for meaningful navigation; Back returns to previous trace/list rather than every row click.
- [ ] Refresh after score/reference actions without flashing an empty page or resetting selection.
  Trace scoring tasks use `pending/running` polling; preserve terminal error text. Use the tested A1
  scheduling behavior, but do not force both domains through a generic polling engine. Pause hidden,
  abort on route change, stop after three transport/5xx failures, expose Retry and manual Refresh.
- [ ] Retention state: `span_count>0` with no retained spans means “Span content is no longer available,”
  with scores/evidence shown if present; avoid claiming retention is definitely the cause when the API
  has no explicit retention marker. A scorable trace with no task/score shows “Not scored,” not pass.
- [ ] Read status/error/model/input-output token values from actual fields. Never substitute total
  wall duration with sum of child durations; unknown cost shows “Not recorded,” not `$0.00`.
  Add explicit Export trace JSON for the current full detail response (retained data only); warn that
  captured content is included, create/revoke the download Blob URL, and never include auth headers.
  CLI equivalent is `assay traces get TRACE_ID > trace.json`, using its existing JSON output.
- [ ] Run trace/conversation/waterfall/stale hook tests, frontend gates; build embedded UI and check
  deep links. Commit `Make conversation the primary trace inspection workspace`.

**Gate:** Raw data is still accessible, but reading a GenAI conversation is the default task.

## C5. Replace loaded-page filtering with server-side trace discovery

**Files**
- Modify: `assayd/internal/api/traces.go`, `assayd/internal/domain/models.go`, `traces.go`,
  `assayd/internal/store/traces.go`, `assayd/db/queries/traces.sql`.
- Create: `assayd/internal/store/trace_search_integration_test.go`; extend API pagination tests.
- Modify: `web/src/features/traces/traces-page.tsx`; create `trace-filters.ts`, `trace-filters.test.ts`.
- Extend: `web/src/features/traces/traces.test.tsx`, `traces-stale.test.tsx`; generate sqlc/API.

**Interfaces:** Add query `q`, `scorer`, `passed`; preserve existing time/status/limit/cursor. Add
`Trace.ScoreSummaries []TraceScoreSummary` and response `score_summaries`, with
`{scorer string, value float64, threshold float64, passed bool, created_at time.Time}`. Queries include
online scores only. Domain query `Passed *bool` must distinguish false from not provided.

- [ ] Test a match only on page 2 is returned by search without first downloading page 1; root-name
  case-insensitivity; exact internal/OTel IDs; literal `%`, `_`, backslash; invalid time range; q over
  200 chars; passed=false; passed without scorer 422; no cross-project rows; no duplicate trace when
  score matches multiple rows. Unscored trace is not failed or passed.
- [ ] Run focused store/API tests and observe unsupported-query/summary behavior before implementing.
- [ ] Implement bound parameters for search. Escape wildcard characters for ILIKE literal substring,
  or use a literal position operation; never interpolate SQL. Parse exact UUID/32-hex ID match only
  when valid. Keep cursor ordering `(start_time,id)` descending. Time bounds stay inclusive/exclusive
  as documented; q/scorer/passed filters are applied before limit.
- [ ] Fetch latest score summaries for **the page's trace IDs in one query**, ordered deterministically
  by `(created_at,id)` per scorer. Use EXISTS for filters to avoid row multiplication. Do not fetch
  trace span/message data. Add an index only after EXPLAIN on the synthetic 10,000-row fixture proves
  a missing access path; place it in the next unused migration, not an old migration.
- [ ] Put filter state in the URL. Default time window is explicit last 24 hours with resolved start/end
  timestamps; user can choose 7/30 days or custom. Debounce q by 250ms, reset cursor on filter change,
  abort/fence previous requests, freeze the time window while paging. Submit/copy URL restores all
  filters; manual refresh updates a relative window and resets pages. No client-only secondary filter.
- [ ] Add score badges to rows from `score_summaries`; no `getTrace` calls for row decoration. Status,
  scorer/pass, time and search fields have labels. Show Reset filters for no matches. Deduplicate
  returned pages by trace ID and guard repeated cursors.
- [ ] Run `go test -race ./internal/store ./internal/api -run 'Trace|Pagination'`, frontend trace
  tests/gates and generation drift. Record EXPLAIN/bounded query evidence; commit
  `Search traces server-side with score summaries`.

**Gate:** Searching is truthful across the application. List request cost is independent of each
trace's span/message count; list row count does not determine detail request count.

## C6. Connect eligibility, references, scoring, deletion, and regression import

**Files**
- Create: `assayd/internal/domain/trace_workflows.go`, `assayd/internal/api/trace_workflows.go`,
  `assayd/internal/domain/trace_workflows_test.go`, `assayd/internal/api/trace_workflows_test.go`.
- Modify: `assayd/internal/domain/trace_scoring.go`, `evaluations.go`, `assayd/internal/app/app.go`,
  `assayd/internal/api/router.go`, `assayd/internal/store/analytics.go`, `assayd/db/queries/analytics.sql`.
- Create: `web/src/features/traces/trace-actions.tsx`, `score-trace-dialog.tsx`,
  `reference-dialog.tsx`, `save-to-dataset-dialog.tsx`, `trace-actions.test.tsx`.
- Modify: `web/src/features/traces/trace-detail.tsx`; generate API/sqlc and update auth fixture deps.

**Interfaces:** Create cohesive `TraceWorkflowService` injected with trace/evaluation collaborators,
process `JudgeDefaults`, and a consumer-owned `TraceWorkflowRepository`. Public methods:
`Eligibility(ctx, traceID uuid.UUID) ([]ScoringEligibility,error)` and
`ImportCase(ctx, datasetID uuid.UUID, input ImportTraceCaseInput) (DatasetItem,error)`.
`ImportTraceCaseInput` fields: `TraceID uuid.UUID`, `Scorer string`, `ExpectedOutput *string`.
`ScoringEligibility` fields: scorer, eligible, reasons (code/message), matching design §6.2.
Repository imports atomically using validated same-app IDs and selected latest score evidence; no
Python/browser reconstruction. It returns conflict on existing external ID, not overwrite.

- [ ] Add eligibility fixtures for every reason code from the spec, including a local judge with no
  key but a valid base/model, disabled scorer, multiple scorable spans, and malformed versus missing
  messages. Refactor existing extraction validation into typed reason errors rather than parsing
  human error strings. Queueing and eligibility call the same validation; do not duplicate scoring
  semantics in a frontend predicate. Eligibility makes zero external judge/target calls.
- [ ] Add import tests: latest selected scorer, missing score, missing evidence, wrong application,
  retained score after span pruning, duplicate 409, expected-reference override, concurrency of
  duplicate imports, score replacement during import. The transaction copies one coherent evidence
  row; it does not require a lock on the entire trace history. Preserve SDK provenance exactly:

```json
{
  "external_id": "trace:TRACE_ID:groundedness",
  "input": {"question": "captured judged input"},
  "output": "captured judged output",
  "metadata": {"trace_id": "TRACE_ID", "score_id": 123, "scorer": "groundedness"}
}
```

  Strings above describe synthetic test values, not defaults; real fields are read from persisted
  evidence, including judged context/reference. Blank explicit expected_output is rejected.
- [ ] Register the exact routes/operation IDs in design §6.2, with admin security and typed errors.
  Update `cmd/openapi` dependency wiring without requiring Postgres/judge during schema generation.
  Add API status tests (201/200/401/404/409/422) before implementing handlers.
- [ ] Build actions: Score dialog loads eligibility and explains disabled options; reference editor
  shows current reference and saves through A5; Save to dataset loads all selectable dataset pages,
  previews evidence, allows corrected reference and then calls the new import endpoint. Explain that
  unscored traces need scoring or manual case entry. Duplicate import offers the dataset link, not a
  pretend success or automatic overwrite. Trace delete uses A5 with typed trace-ID confirmation.
- [ ] Await mutation success, fence/abort on trace change, then C4 refresh. Poll pending scoring tasks
  and show failed task errors without confusing a failed judge with failed answer quality. Reference
  attachment may itself schedule correctness under app settings; do not silently queue a duplicate.
  Delete navigates to trace list and does not retry POST/DELETE blindly after transport uncertainty.
- [ ] Run workflow API/domain/store/retention tests plus trace action UI tests/gates; commit
  `Connect trace inspection to scoring and regression datasets`.

**M7C demo:** Find failure with server filters → read conversation and overlapping spans → attach
reference → score with fake judge → inspect rationale → save evidence to dataset → verify a duplicate
is safe. Show a retained-score trace without spans and malformed/uncaptured content honestly.
