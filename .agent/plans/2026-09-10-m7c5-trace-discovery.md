---
gate: yes
milestone: M7C5
status: ready-for-approval
---

# M7C5 Server-Side Trace Discovery

**Goal.** Make trace search and score filtering truthful across the full application, with stable
cursor pagination, list-row score summaries, generated API types, and URL-backed frontend controls.
Scope is only M7C5; M7C1-C4 remain the presentation baseline.

**Current facts.** HEAD is `c3f6a7a` on `feat/product-completion`; the four preceding M7C commits
implemented conversation normalization/rendering, waterfall timing, and workbench integration. The
server currently filters traces by application/time/status before `(start_time,id)` keyset pagination,
but list responses omit scores. `traces-page.tsx` searches only downloaded rows and does not carry
filters through “Load more.” Online score replacement deletes the prior trace/scorer row, while the
existing score index supports bounded trace-ID lookups. Focused domain/API and trace UI tests pass in
the current checkout (the frontend shell reports that local Node 24 differs from the required Node 22).

**API contract.** Extend `GET /v1/traces` without changing auth, ordering, cursor encoding, or existing
parameters. Add `q` (trimmed, maximum 200 characters; literal case-insensitive root-name substring or
exact Assay UUID/32-hex OTel trace ID), `scorer` (`groundedness|correctness`), and `passed`
(`true|false`, valid only with `scorer`). Start is inclusive and end exclusive. All filters run before
limit/cursor and scores are online-only. Every list row adds non-null `score_summaries`, sorted by
scorer, containing the latest `{scorer,value,threshold,passed,created_at}` per scorer; zero and absent
scores remain distinct.

**Approach.** Extend the existing trace query/domain/store path rather than add a search service. Use
bound SQL, `position(lower(q) in lower(root_name))` for literal matching, and one `EXISTS` predicate so
scorer/pass filtering cannot duplicate traces. Fetch summaries for the bounded page trace IDs in one
second query; any database/conversion failure aborts the request with context rather than returning
partial rows. Keep frontend filter serialization in one small pure module, then make the page call the
generated client with the exact URL filters on every page. Validation failures are fatal 422s; initial
request failures replace results with an error, while load-more failures retain prior rows and report
the failure; aborted/stale requests are ignored intentionally. No design pattern or new layer is
needed.

**Not doing.**
- No full-text engine, fuzzy search, saved views, new dependency, or trace-detail fetch per row.
- No schema migration/index now: existing trace/score indexes bound paging and the 10,000-row EXPLAIN
  check decides whether a separately reviewed index is justified.
- No snapshot cursor or compatibility path; membership can naturally change when a trace is rescored.
- No SDK/CLI changes (those belong to M7D), trace actions (M7C6), or broad documentation rewrite;
  Huma/OpenAPI field descriptions are the public documentation for this approved API change.

## Steps

1. Extend `assayd/internal/domain/{models.go,traces.go,traces_test.go}` with `TraceScoreSummary`,
   `Trace.ScoreSummaries`, and `TraceQuery.Q/Scorer/Passed`; trim/validate q, validate scorer, and reject
   `passed` without scorer — verify: `cd assayd && go test ./internal/domain -run 'TestListTraces'`.
2. Update `assayd/db/queries/traces.sql` and `assayd/internal/store/traces.go` to apply q/scorer/pass
   before keyset/limit and load latest page summaries set-wise; extend
   `assayd/internal/store/traces_integration_test.go`, regenerate `internal/store/sqlc/`, and cover
   page-2 matches, literal `%/_/\\`, both IDs, passed=false, unscored exclusion, no duplicates,
   project isolation, deterministic summaries, and unchanged cursors — verify:
   `cd assayd && go test -race -count=1 ./internal/store -run 'Trace.*(Search|Filter|Pagination|Summary)'`.
3. Modify `assayd/internal/api/traces.go` and focused trace read/pagination tests to expose and parse the
   three query parameters and map summaries on list rows only; assert 422 for overlong q and
   passed-without-scorer, plus auth/isolation and opaque cursor round trips. Regenerate
   `web/openapi.json` and `web/src/api/generated/` — verify: `cd web && pnpm generate:api`, then after
   committing generated output rerun it and require
   `git diff --exit-code -- openapi.json src/api/generated`.
4. Create `web/src/features/traces/trace-filters.ts` and its test, then replace client-only filtering in
   `traces-page.tsx`. Canonical URL state includes q/status/scorer/pass, exact start/end, and UI-only
   `range=24h|7d|30d|custom`; default 24h and presets resolve to fixed timestamps, refresh advances
   relative ranges, custom values use native date/time controls, q debounces 250ms, and every filter
   change resets pages/cursors. Render summaries as score/pass badges, “Not scored” for none, dedupe
   trace IDs, and retain the repeated-cursor guard — verify:
   `cd web && pnpm test src/features/traces/trace-filters.test.ts src/features/traces/traces.test.tsx src/features/traces/traces-stale.test.tsx`.
5. Run generation drift and repository gates; update only Huma field descriptions if generated docs do
   not state the contract — verify: backend and frontend commands under “Verification evidence” all
   pass with zero warnings on Go 1.27/Node 22.

## Verification evidence

- Truthful production query/pagination — evidence:
  `cd assayd && go test -race -count=1 ./internal/store ./internal/api -run 'Trace|Pagination'`; tests
  call the Huma route against disposable Postgres and prove filters precede limit/cursor.
- Bounded list cost — evidence: recorded `EXPLAIN (ANALYZE, BUFFERS)` for 10,000 trace summaries shows
  one bounded trace page plus one summary query, with no span/message scan or per-row query.
- Public/generated contract — evidence: `cd web && pnpm generate:api && git diff --exit-code -- openapi.json src/api/generated`;
  generated `ListTracesData` exposes q/scorer/passed and `TraceListResponse.score_summaries`.
- Browser behavior through the generated client — evidence: the focused Vitest command in step 4 shows
  page-2 search, frozen filters on load-more, URL restoration, pass=false, score badges, stale-response
  fencing, deduplication, and repeated-cursor reporting.
- Repository gates — evidence: `cd assayd && go build ./... && golangci-lint run && go test -race -count=1 ./...`;
  `cd web && pnpm lint && pnpm format:check && pnpm typecheck && pnpm test && pnpm build`; then
  `cd assayd && go test ./internal/ui ./internal/app && go build ./cmd/assayd` verifies the embedded
  production path.

## Risks

- Score changes between pages can alter membership; test stable ordering/cursor behavior and document
  that cursors are positions, not snapshots, rather than adding snapshot infrastructure.
- Literal root-name search may scan the 10,000-row fixture; inspect EXPLAIN early and replan an index
  only if measured performance fails.
- Debounced URL updates can race navigation/load-more; existing abort plus request-number fencing and
  stale-response tests are the cheapest proof.

## Open questions

- None; the M7C5 public trace-query contract is explicitly approved.
