# M7A Correctness and Safe Lifecycle Implementation Plan

> **For agentic workers:** Use `executing-plans` task-by-task, or the available
> `superpowers:subagent-driven-development` skill. Complete one task's checkboxes before proceeding.

**Goal:** Repair existing state/auth defects and make dataset/trace/run management safe for history.
**Architecture:** Keep domain-owned repositories and real Postgres transactions; add immutable run-item
snapshots before exposing item mutations. Huma remains a thin adapter with generated clients.
**Tech Stack:** Go/Huma/pgx/sqlc/goose; React/TypeScript/Vitest/MSW.
**Spec:** [`../specs/2026-09-08-product-completion-design.md`](../specs/2026-09-08-product-completion-design.md), §§3,5–6.

## Global Constraints

All constraints in the [master plan](2026-09-08-product-completion.md#global-constraints) apply.
Never edit deployed migrations 00001–00005, bypass job leases, or test deletions on the user's data.
This subplan depends on design approval, not on a UI rewrite.

## A1. Fix pending-run behavior and define a typed status contract

**Files**
- Modify: `assayd/internal/api/eval_runs.go` (`evalRunResponse`, `evalRunItemResponse`).
- Create: `web/src/features/runs/run-status.ts`, `run-status.test.ts`.
- Modify: `web/src/features/runs/use-run-polling.ts`, `run-detail.tsx`, `runs-page.tsx`.
- Test: `web/src/features/runs/use-run-polling.test.tsx`, `runs.test.tsx`.
- Generate: `web/openapi.json`, `web/src/api/generated/`.

**Interfaces:** Server wire state is
`pending | running | succeeded | failed | canceled`. UI copy may say “Queued” for `pending`, but the
wire value never becomes `queued`. Produce `isActiveRun(status: EvalRunResponse["status"]): boolean`.

- [x] Add this regression to the existing polling test file, using its actual `server`, `runFixture`,
  `PollingHarness`, `waitForText`, and fake-timer setup:

```tsx
test("continues polling while pending", async () => {
  let requests = 0;
  server.use(http.get(`*/v1/runs/${runID}`, () => {
    requests += 1;
    return HttpResponse.json(runFixture(requests === 1 ? "pending" : "succeeded"));
  }));
  render(<PollingHarness />);
  await waitForText("pending");
  await act(() => vi.advanceTimersByTimeAsync(1000));
  expect(screen.getByText("succeeded")).toBeInTheDocument();
  expect(requests).toBe(2);
});
```

- [x] Run `(cd web && pnpm test src/features/runs/use-run-polling.test.tsx)`; expect the new test to
  fail because no second request occurs. Add a page test that a pending run exposes Cancel, canceled
  runs stop polling, and 409 on cancellation refreshes state without reporting success.
- [x] Add Huma `enum:"pending,running,succeeded,failed,canceled"` tags to run and item response
  status fields, regenerate API, then implement:

```ts
import type { EvalRunResponse } from "@/api/generated/types.gen";

export function isActiveRun(status: EvalRunResponse["status"]): boolean {
  return status === "pending" || status === "running";
}
```

- [x] Replace both `queued` sets, preserving one timer, visibility pause, three-failure stop, and
  explicit retry. Reset pending cancellation state and abort controller on **run ID changes**, not
  just component unmount. Check the response's request generation before closing/refreshing a dialog.
  Clear `run` immediately when run ID changes so the previous run's data/actions cannot appear at the
  new URL while its GET is pending; add a same-application A→B delayed-response regression.
- [x] Run polling/run tests, frontend lint/format/types, and `go test ./internal/api ./cmd/openapi`.
  Temporarily restore `queued` to prove the regression fails; restore the correct code.
- [x] Review/stage only these source/generated files and commit `Fix pending evaluation run lifecycle`.

**Gate:** Pending → running → terminal is observable without a reload. All five states use generated
wire values; the test no longer only covers a server that starts at running.

## A2. Make authentication invalidation and application refresh race-safe

**Files**
- Modify: `web/src/api/client.ts`, `client.test.ts`, `web/src/auth/auth-context.tsx`.
- Create: `web/src/features/applications/application-catalog.tsx`, `application-catalog.test.tsx`.
- Modify: `web/src/app/router.tsx`, `router.test.tsx`, `web/src/components/app-shell.tsx`,
  `web/src/features/applications/applications-page.tsx`.

**Interfaces:** Auth owns credentials and connection state only. Catalog owns application loading.
Produce `useApplicationCatalog(): {applications: ApplicationResponse[]; refresh: () => Promise<void>;
loading: boolean; error: string | null}`. Configure transport as
`configureClient(getToken, onUnauthorized?)`, both functions; do not expose the token to pages.

- [x] Extend existing MSW router tests with these sequences: successful connect → protected GET 401
  → token removed and connection gate; GET 403/503 → remain connected; disconnect while connect GET
  is pending → late success does not reauthenticate; reconnect with a new credential → an old
  request's 401 does not clear the new session. Assert storage and rendered gate, not internal calls.
- [x] Add a catalog behavior test: list initially A, server changes to A+B, invoke refresh, switcher
  contains B without reconnecting. A failed refresh retains last known list and offers Retry; removing
  selected A redirects to Applications after successful deletion, not a stuck not-found workspace.
- [x] Run `(cd web && pnpm test src/app/router.test.tsx src/api/client.test.ts)` and confirm the
  post-connect 401 regression. New catalog test becomes executable once its provider contract exists.
- [x] Implement request-generation fencing. The interceptor compares the failed request's bearer
  value to the current getter **in memory only** before invoking invalidation:

```ts
const current = getToken();
if (response?.status === 401 && current !== null &&
    request?.headers.get("Authorization") === `Bearer ${current}`) {
  onUnauthorized?.();
}
```

  In AuthProvider use a monotonically increasing connection attempt ID and AbortController. Increment
  on connect/disconnect; success must still own that ID before persisting the token. Test StrictMode
  mount/cleanup. Transport callback must remain stable or replace interceptors without accumulating.
- [x] Extract the already duplicated application list ownership into the catalog, without adding a
  query library. Keep connection validation as an admin read; seed the catalog or refresh after
  connection, but do not retain a second authoritative list in auth. Clear catalog on disconnect and
  fence its late responses. Update **all** existing auth-context consumers/tests in this task.
- [x] Run named tests plus `pnpm lint`, `pnpm format:check`, `pnpm typecheck`; inspect no token is
  passed through context values other than the existing private ref; commit
  `Fence authentication requests and refresh application catalog`.

**Gate:** CRUD can refresh navigation safely, and expired credentials consistently return users to
Connect. The localStorage security model is not advertised as a login/session system.

## A3. Persist immutable run-item inputs before allowing case edits

**Files**
- Create: `assayd/db/migrations/00006_eval_run_item_snapshots.sql`.
- Modify: `assayd/db/queries/eval_runs.sql`, `assayd/internal/store/eval_runs.go`,
  `assayd/internal/store/evaluation_execution.go`, `assayd/internal/domain/evaluation_models.go`.
- Modify: `assayd/internal/api/eval_runs.go` (`evalRunItemOutput` and response),
  `assayd/internal/worker/runner.go` only where it assumes live item joins.
- Test: create `assayd/internal/store/run_snapshots_integration_test.go`; extend
  `assayd/internal/migrate/migrate_test.go`, `assayd/internal/worker/runner_test.go`.
- Generate: `assayd/internal/store/sqlc/`, `web/openapi.json`, `web/src/api/generated/`.

**Consumes:** Existing `EvalRunItem.Item domain.DatasetItem`, repeatable-read creation transaction,
score FK `(eval_run_id,dataset_item_id)` and job-first locking.
**Produces:** `EvalRunItem.SnapshotOrigin string`; HTTP `snapshot` containing `datasetItemResponse`
and `snapshot_origin` enum `creation,legacy_backfill`. The internal `Item` now always means snapshot.

- [x] Write `TestRunSnapshotsSurviveSourceMutation` using disposable Postgres: create dataset/case A,
  create run R, update A directly through a test connection, list R items, assert original input and
  output. Delete A via SQL, assert R item and any persisted score still exist. These direct SQL
  mutations deliberately prove the storage invariant before the public item APIs exist.
- [x] Run `go test -race -count=1 ./internal/store -run TestRunSnapshots`; expect old-content
  assertion to fail before the migration. Add migration fixture coverage with existing 00005 data.
- [x] Add typed snapshot columns and backfill; representative SQL below defines the full column set:

```sql
ALTER TABLE eval_run_items
    ADD COLUMN snapshot_dataset_id uuid,
    ADD COLUMN snapshot_external_id text,
    ADD COLUMN snapshot_input jsonb,
    ADD COLUMN snapshot_output text,
    ADD COLUMN snapshot_expected_output text,
    ADD COLUMN snapshot_context jsonb,
    ADD COLUMN snapshot_metadata jsonb,
    ADD COLUMN snapshot_created_at timestamptz,
    ADD COLUMN snapshot_updated_at timestamptz,
    ADD COLUMN snapshot_origin text NOT NULL DEFAULT 'legacy_backfill';

UPDATE eval_run_items ri
SET snapshot_dataset_id = di.dataset_id,
    snapshot_external_id = di.external_id,
    snapshot_input = di.input,
    snapshot_output = di.output,
    snapshot_expected_output = di.expected_output,
    snapshot_context = coalesce(di.context, '[]'::jsonb),
    snapshot_metadata = di.metadata,
    snapshot_created_at = di.created_at,
    snapshot_updated_at = di.updated_at
FROM dataset_items di WHERE di.id = ri.dataset_item_id;

ALTER TABLE eval_run_items
    DROP CONSTRAINT eval_run_items_dataset_item_id_fkey,
    ALTER COLUMN snapshot_dataset_id SET NOT NULL,
    ALTER COLUMN snapshot_input SET NOT NULL,
    ALTER COLUMN snapshot_context SET NOT NULL,
    ALTER COLUMN snapshot_metadata SET NOT NULL,
    ALTER COLUMN snapshot_created_at SET NOT NULL,
    ALTER COLUMN snapshot_updated_at SET NOT NULL,
    ALTER COLUMN snapshot_origin SET DEFAULT 'creation',
    ADD CONSTRAINT eval_run_items_snapshot_origin_check
        CHECK (snapshot_origin IN ('creation', 'legacy_backfill'));
```

  Use Goose Up/Down sections. A down migration must **refuse** if historical item IDs no longer have
  live source rows; raise an actionable exception before removing columns rather than losing history
  or inserting fake dataset items. Otherwise restore the old FK and drop snapshot columns.
- [x] Rewrite `CreateEvalRunItems` to `INSERT ... SELECT` all source fields with origin `creation`.
  Replace live `JOIN dataset_items` in pending/list reads with explicit snapshot column aliases that
  preserve conversion shapes. No permanent fallback to live content after migration.
- [x] Inside `createEvalRunTransaction`, validate copied snapshot count and mode/scorer requirements
  before inserting the job. Add a `CountInvalidEvalRunSnapshots` SQL query (run ID, mode, scorers) and
  return `domain.ErrInvalid` on any missing required output/reference. Reuse domain checks for
  context/question validity; retain existing validation on create. Repeatable read pins membership
  and contents together; a concurrent edit cannot yield half-old/half-new input.
- [x] Expose snapshot/origin in the run item API now; adding per-item scores is task D1. Backfilled
  snapshots are explicitly marked, not called original run inputs.
- [x] Test canceled/failed/succeeded histories, generated outputs independent of source outputs,
  rollback of invalid runs/jobs, exact 00005→00006 backfill, repeated startup, and create/edit/delete
  interleavings with transaction barriers. Include the existing worker retry/lease tests.
- [x] Regenerate, run `go test -race -count=1 ./internal/store ./internal/migrate ./internal/worker
  ./internal/api`, frontend types, review migration lock/disk costs and backup instruction, commit
  `Snapshot evaluation inputs independently of live dataset cases`.

**Gate:** Do not start A4 until the historical-evidence tests pass against real Postgres. The new
schema is the only runtime format; migration support is not a dual-format compatibility layer.

## A4. Add scoped dataset metadata and case mutation APIs

**Files**
- Create: `assayd/internal/domain/dataset_mutations.go`, `assayd/internal/api/dataset_items.go`,
  `assayd/internal/api/dataset_mutations_test.go`.
- Modify: `assayd/internal/domain/evaluation_models.go`, `assayd/internal/domain/evaluations.go`,
  `assayd/internal/store/datasets.go`, `assayd/db/queries/datasets.sql`,
  `assayd/internal/api/datasets.go`, `assayd/internal/api/api_test.go` (security paths).
- Test: extend `assayd/internal/domain/evaluations_test.go`, create
  `assayd/internal/store/dataset_mutations_integration_test.go`; regenerate both clients.

**Interfaces:** Add EvaluationService methods (and matching repository methods using domain types):

```go
type UpdateDatasetInput struct {
    Name *string
    Description *string
    ClearDescription bool
}
type ReplaceDatasetItemInput struct {
    ExternalID *string
    Input map[string]any
    Output *string
    ExpectedOutput *string
    Context []Chunk
    Metadata map[string]any
}
// Methods on *EvaluationService:
// UpdateDataset(ctx context.Context, id uuid.UUID, in UpdateDatasetInput) (Dataset, error)
// GetDatasetItem(ctx context.Context, datasetID, itemID uuid.UUID) (DatasetItem, error)
// ReplaceDatasetItem(ctx context.Context, datasetID, itemID uuid.UUID,
//                    in ReplaceDatasetItemInput) (DatasetItem, error)
// DeleteDatasetItem(ctx context.Context, datasetID, itemID uuid.UUID) error
```

- [x] Add API tests using `newAPIFixture`, `perform(requestSpec)`, `assertStatus`, `decodeResponse`
  already present in `api_test.go`. A concrete metadata regression can bootstrap through existing APIs:

```go
func TestDatasetRename(t *testing.T) {
    f := newAPIFixture(t)
    project := f.createProject("synthetic-judge-secret")
    app := f.createApplication(project.ID)
    created := f.perform(requestSpec{
        method: http.MethodPost, path: "/v1/datasets", token: adminToken,
        body: `{"application_id":"` + app.ID + `","name":"baseline"}`,
    })
    assertStatus(t, created, http.StatusCreated)
    var dataset struct { ID string `json:"id"` }
    decodeResponse(t, created, &dataset)
    renamed := f.perform(requestSpec{
        method: http.MethodPatch, path: "/v1/datasets/" + dataset.ID, token: adminToken,
        body: `{"name":"regression"}`,
    })
    assertStatus(t, renamed, http.StatusOK)
    var result struct { Name string `json:"name"` }
    decodeResponse(t, renamed, &result)
    if result.Name != "regression" { t.Fatalf("name = %q", result.Name) }
}
```

- [x] Run `go test ./internal/api -run TestDatasetRename`; expect unsupported method before code.
  Add table cases: blank name, duplicate name 409, null clearing, set+clear conflict 422, unknown
  dataset 404; item under wrong dataset 404 on GET/PUT/DELETE; no admin 401 on each new route.
- [x] Define PUT required nullable fields using Huma's actual required/null schema support. Check
  generated OpenAPI tests: `output` is both required and nullable. Plain Go pointers alone must not
  accidentally make omission indistinguishable from null. Use a DTO presence type only for these
  fields if needed; no repository-wide PATCH framework. Match the design's full replacement body.
- [x] Extract stateless item normalization from `newDatasetItem` so creation and replacement reuse
  validation without generating a new identity on replacement. Preserve current nonblank
  `input.question` requirement, clone input maps before trimming, reject non-object metadata, blank
  expected output when supplied, and duplicate/blank context IDs. Optional blank output normalizes
  consistently to null; an empty context array is valid.
- [x] Implement parent-scoped SQL, not global item lookup followed by an unscoped mutation:

```sql
-- name: DeleteDatasetItem :one
DELETE FROM dataset_items
WHERE dataset_id = sqlc.arg(dataset_id) AND id = sqlc.arg(item_id)
RETURNING id;
```

  PUT updates editable columns and `updated_at` only; ID, dataset ID, and created_at remain stable.
  Update dataset `updated_at` in the same transaction after case create/replace/delete. Preserve
  atomic bulk create and uniqueness of non-null external ID. Metadata PATCH only sets provided
  fields, uses explicit clear_description, and rejects an empty patch.
- [x] Verify A3 with API-level edit/delete and run execution; duplicates roll back, ID reuse is not
  allowed, pagination still uses created_at+ID. Concurrent full replacements are last-commit-wins;
  document this single-user limitation instead of inventing an optimistic-lock protocol.
- [x] Regenerate and run dataset API/domain/store tests plus frontend types; commit
  `Add safe dataset and case management endpoints`.

**Gate:** Existing dataset import/create clients remain valid. Prior run data does not change when the
live case is edited or deleted; dataset deletion still explicitly cascades all its runs.

## A5. Add explicit admin trace actions and safe destructive lifecycle

**Files**
- Modify: `assayd/internal/api/traces.go`, `eval_runs.go`, `auth.go`, `api_test.go`.
- Create: `assayd/internal/domain/lifecycle.go`, `assayd/internal/store/lifecycle.go`.
- Modify: `assayd/internal/domain/traces.go`, `evaluation_models.go`, `assayd/db/queries/traces.sql`,
  `eval_runs.sql`; use existing `store/projects.go` deletion lock pattern.
- Test: `assayd/internal/api/trace_reads_test.go`, create `trace_mutations_test.go`,
  `assayd/internal/store/lifecycle_integration_test.go`; generate API/sqlc.

**Interfaces:** `TraceService.QueueScoresAdmin(ctx, traceIDs []uuid.UUID, scorers []string)` returns
`[]Job,error`; `AttachReferenceAdmin(ctx, traceID uuid.UUID, reference string)` returns `Trace,error`.
`TraceService.DeleteAdmin(ctx, traceID uuid.UUID) error` and
`EvaluationService.DeleteEvalRun(ctx, runID uuid.UUID) error`. Existing project-scoped methods remain
for their actual distinct auth use case, not as aliases for an unrestricted operation.

- [x] Add tests: admin can attach/score; project A key still cannot mutate project B; mixed-project
  admin score batch returns 422 and creates no jobs; invalid IDs fail atomically; admin token cannot
  ingest; reference write remains nonblank. Run tests and verify current admin mutation rejection.
- [x] Add admin orchestration loading ownership through repository/domain services. Reuse the existing
  queue validation for each selected trace and determine the common project before enqueueing.
  Update only the two mutation OpenAPI security alternatives, never globally bypass project auth.
- [x] Add DELETE tests: unknown resource 404; non-admin 401; terminal runs succeed with 204 and remove
  owned jobs/items/scores; pending/running runs return 409; deleting a trace removes spans, scores,
  scoring tasks, while preserving already imported dataset evidence. A repeated DELETE returns 404.
- [x] Implement store deletions in transactions using the existing job-first global table-lock
  strategy, not SELECT-then-DELETE races. For terminal run deletion lock the job before checking run
  state. Trace deletion uses the cascade-safe lock; a paused worker must not publish a late score.
  Do not implement arbitrary deletion of individual spans or scores.
- [x] Execute `go test -race -count=1 ./internal/api ./internal/store ./internal/worker -run
  'Trace|Lifecycle|Cancel|Lease|Delete'`, regenerate, inspect auth snapshot tests; commit
  `Expose scoped trace actions and safe run deletion`.

**Gate:** No browser project-key prompt is needed for these actions. Revocation, cross-project checks,
worker fencing, and permanent-delete behavior are covered independently.

## A6. Make span-tree presentation complete for malformed parent relationships

**Files**
- Modify: `assayd/internal/api/traces.go`, extracting the tree transform into
  `assayd/internal/api/span_tree.go` if needed to keep the handler focused.
- Create: `assayd/internal/api/span_tree_test.go` in package `api` for the pure transform;
  extend `assayd/internal/api/trace_reads_test.go` for HTTP output.

**Interface:** Preserve `spanTree(spans []domain.Span) []*spanResponse` and the stored parent IDs.
No new ingestion format, JSON shape, or frontend graph dependency.

- [x] Build table-driven fixtures for empty, root/child, orphan, self-parent, two-node cycle,
  three-node cycle with a leaf, equal start times, and deep chains. For every fixture recursively
  collect response identities with a visited set and assert each input span appears exactly once.
  Add an HTTP fixture that currently loses all nodes in a two-node parent cycle.
- [x] Run `go test ./internal/api -run 'SpanTree|TraceReads'`; confirm the cycle regression fails.
- [x] Implement an iterative parent-graph pass using unvisited/visiting/visited state. Order spans
  by start time then stable identity. Break one edge per cycle deterministically (the earliest node
  becomes a presentation root); orphans/self-parent become roots. Preserve original parent fields.
  Avoid O(n²) ancestry scans. Handle duplicate OTel span IDs deterministically without overwriting
  a node: parent resolution chooses the earliest distinct candidate; each stored row still appears.
- [x] Add a 1,000-node fixture and benchmark the transform. Verify deep traversal does not recursively
  construct a cycle, and output order is stable under shuffled database row input.
- [x] Run `go test ./internal/api` and lint; commit `Preserve all spans in trace tree presentation`.

**M7A demo:** Pending run polls/cancels; dataset case edit/delete leaves historical snapshot and scores;
admin trace mutation succeeds while project isolation holds; cyclic spans are visible. Record actual
commands and reviewer approval in the master plan before shipping the UI that depends on these APIs.
