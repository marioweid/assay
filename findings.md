# Code Review Findings

Reviewed `origin/main...HEAD` on branch `feat/product-completion`.

## Resolution

Resolved after merge on 2026-09-11. The dark-mode palette issue was already fixed in PR #8. The
follow-up changes replace global job-table deletion locks with target-scoped row locks, seed the
application catalog from authentication, surface refresh failures, protect newly created API keys
from implicit dismissal, normalize blank Python expected outputs, narrow the documentation ignore
rule, and exercise a populated migration 5 → 6 upgrade. Independent review found no blockers or
should-fix items.

Available Go, frontend, and Python checks pass. Docker-backed PostgreSQL tests could not execute
because the local Docker daemon is unavailable, and Go race detection could not run because CGO is
disabled and no C compiler is installed.

## High

### 1. Dark mode breaks existing screens

Locations:

- `web/src/features/runs/run-detail.tsx:159,189`
- `web/src/features/traces/trace-detail.tsx:56,66`
- `web/src/features/datasets/datasets-page.tsx:72,86`

Fixed `bg-white`, `bg-slate-50`, and `text-blue-700` classes conflict with dark-mode text. Core trace, dataset, and completed-run screens can become unreadable or have poor contrast.

**Recommendation:** Replace fixed palette classes with semantic tokens such as `bg-surface`, `text-ink`, `text-accent`, and the existing semantic error tokens. Add a dark-theme rendering test covering an existing feature page.

## Medium

### 2. Deleting one trace or run blocks the entire worker queue

Locations:

- `assayd/internal/store/database.go:23-43`
- `assayd/internal/store/lifecycle.go:16-35`
- `assayd/db/queries/jobs.sql:36-37`
- `assayd/internal/worker/pool.go:18-20`

`LOCK TABLE jobs IN EXCLUSIVE MODE` blocks unrelated job claims, heartbeats, retries, and completions. A sufficiently large cascading deletion can exceed the 30-second worker lease and cause unrelated work to lose its lease or retry.

**Recommendation:** Lock only job rows associated with the target trace or run while preserving job-first lock ordering. Let foreign-key locking serialize new references against deletion. Add a concurrency test proving an unrelated heartbeat proceeds during deletion.

### 3. Application loading performs duplicate requests and misreports failures

Locations:

- `web/src/auth/auth-context.tsx:49-53`
- `web/src/features/applications/application-catalog.tsx:22-52`
- `web/src/features/applications/applications-page.tsx:12,36-42`
- `web/src/components/app-shell.tsx:27,33-47`

Authentication fetches and discards the application list, after which the catalog immediately fetches it again. Catalog failures are stored but ignored by consumers, which can display “No applications” or “Application not found” after a temporary server error. A protected deep link can also briefly flash a not-found state before the catalog effect sets loading.

**Recommendation:** Give one component ownership of the request and seed the catalog from successful authentication, or use a dedicated authentication check. Render catalog errors and initialize a connected catalog as loading.

### 4. A newly created API key can be accidentally lost

Locations:

- `web/src/features/projects/api-keys-panel.tsx:83-115`
- `web/src/components/ui/dialog.tsx:34-37`

After creation, pressing Escape or clicking outside closes the dialog and discards the only plaintext copy of the API key.

**Recommendation:** Once the key exists, disable outside and Escape dismissal until the user chooses the explicit **Done** action, or require confirmation before discarding an unacknowledged key. Test both dismissal paths.

### 5. Backend validation contradicts the Python client

Locations:

- `assayd/internal/domain/dataset_mutations.go:105-107`
- `clients/python/assay/src/assay/importers.py:107-110`
- `clients/python/assay/src/assay/client.py:1181-1184`

The Python importer accepts and sends `expected_output: ""`, while backend dataset creation now rejects a blank expected output with HTTP 422. Existing imports containing blank references therefore stop working.

**Recommendation:** Normalize blank expected outputs to `None` client-side and add a compatibility test. Document the behavior change if it remains externally observable.

### 6. The entire documentation tree is ignored

Location:

- `.gitignore:38`

The unanchored `docs` rule hides newly created documentation files and directories from Git, even though documentation is a tracked project artifact.

**Recommendation:** Ignore only the disposable directory, such as `/docs/plans/`, rather than every `docs` directory.

## Missing coverage

Add a populated migration test for migration 5 → 6 that verifies:

- existing run items receive `snapshot_origin = 'legacy_backfill'`
- workers can process migrated snapshots
- deleting an original dataset item preserves historical run input

Current migration tests install the latest schema from scratch and do not exercise the upgrade path with existing data.

## Validation

Passed:

- `go test ./...`
- `go vet ./...`
- `pnpm test` — 84 tests
- `pnpm typecheck`
- `pnpm lint`
- `pnpm format:check`
- `pnpm build`
- `git diff --check`

Python tests could not start because the downloaded generic Python binary was incompatible with NixOS.

## Overall assessment

The branch generally preserves the API → domain → repository → store → SQL architecture. Evaluation snapshots and scoped dataset mutations look sound. Fix the dark-theme regression and global job-table lock before merging, then address catalog ownership, one-time-key dismissal, the Python contract mismatch, and the overbroad ignore rule.
