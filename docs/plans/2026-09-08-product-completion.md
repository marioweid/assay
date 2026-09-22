# Assay Product Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: If available, use the
> `superpowers:subagent-driven-development` skill; otherwise use `executing-plans` to implement this
> plan task-by-task. Steps use checkbox syntax for tracking. Read the design before writing code.

**Goal:** Turn Assay's test UI into a polished conversation-first tracing and evaluation workspace,
with safe resource management, ergonomic SDK/API workflows, and a verified container quickstart.

**Architecture:** Preserve the embedded React SPA, Go domain/store/worker layers, OTLP JSON ingestion,
and Postgres. Deliver five independently reviewable subplans; establish immutable evaluation inputs
before item mutation and use generated contracts across the backend, browser, and SDK.

**Tech Stack:** Existing Go 1.27/Huma/pgx/sqlc/goose; React/Vite/TypeScript/Tailwind/Radix; Node 22/pnpm;
Python 3.13/uv/OTel/httpx; Postgres; Docker/GitHub Actions.

**Spec:** [`../specs/2026-09-08-product-completion-design.md`](../specs/2026-09-08-product-completion-design.md).

**Status:** Implementation in progress on branch `feat/product-completion`. Milestones M7A (A1–A6) and
M7B tasks B1–B3 are complete and committed with their tests; see §6 for the verification record.
B4–B6 and the C/D/E subplans remain; milestone demo gates await human review in the browser.

## Global Constraints

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

## 1. Start here: smaller-model execution protocol

1. Obtain approval for the proposed design, especially snapshot migration, permanent deletion,
   admin trace mutation, item PUT semantics, and theme direction. Publication needs separate approval.
2. Work on a feature branch, never directly on `main`. Preserve the user's `improvements.md`.
3. Read this file, the design, and **one** subplan. Do not load the entire codebase or implement all
   tasks in one response. A task is a review unit; its checkboxes are individual actions.
4. Inspect only that task's named source files and their direct types/test fixtures. Reconcile paths
   against the checkout. Source has precedence over line-number hints, not over agreed requirements.
5. Write the regression/acceptance test first. Run it and record its specific failure. Implement the
   narrow change, rerun the test, lint/types, inspect the diff, and commit that task only.
6. A missing tool, skipped database test, package-install failure, and failed assertion are different
   outcomes. Record them accurately. Do not mark a task done because it compiles or because a test skips.
7. Report: task ID, changed files, API/schema changes, commands/results, unresolved issue, next task.
8. Stop at each milestone's demo gate for human review. Do not push a release or run destructive
   acceptance tests against the user's working database.
9. If a contract needs changing, stop and update/review the design and dependent tasks together; do
   not introduce aliases, compatibility fallback fields, or private browser-only APIs.
10. For parallel workers, each uses its own worktree. Only parallelize the independent lanes below.

Recommended handoff prompt:

```text
Read docs/specs/2026-09-08-product-completion-design.md and the master plan.
Implement only task A1 from subplan A after design approval.
Read its named files, add the stated failing behavior test, and verify the failure.
Make the smallest fix; run its test/lint/type commands and inspect the diff.
Do not implement later tasks, edit generated code by hand, touch .env, publish, or use real judges.
Report evidence and stop for review before continuing.
```

## 2. Milestones and task dependency map

| Milestone | Subplan | Tasks | Demo gate |
|---|---|---|---|
| M7A: correctness and safe lifecycle | [A: backend/data safety](2026-09-08-product-completion-a.md) | A1–A6 | Pending runs poll; editing/deleting cases preserves run evidence; auth boundaries tested |
| M7B: operable visual workspace | [B: design and management](2026-09-08-product-completion-b.md) | B1–B6 | Empty install → project → app → key; settings and dataset CRUD usable in both themes |
| M7C: GenAI trace workbench | [C: trace experience](2026-09-08-product-completion-c.md) | C1–C6 | Conversation + waterfall + server filters + score/reference/regression flow |
| M7D: evaluation and SDK parity | [D: evaluations and SDK](2026-09-08-product-completion-d.md) | D1–D6 | Inspect/compare case outcomes; same workflow in Python/CLI; structured capture round-trip |
| M7E: usable distribution | [E: delivery and release](2026-09-08-product-completion-e.md) | E1–E4 | Published image boot + Linux docs + embedded browser acceptance |

Safe default execution order:

```text
A1 → A2 → A3 → A4 → A5 → A6
B1 → E1 (embedded-browser guardrails) → B2 → B3 → B4 → B5 → B6
C1 → C2 → C3 → C4 → C5 → C6
D1 → D2 → D3 → D4 → D5 → D6
E2 → E3 → E4
```

**Execution update (2026-09-22):** Finish the remaining functional/delivery work (E2–E3) before
expanding browser workflow coverage. E4's visual redesign, full browser scenarios, accessibility,
and performance gates begin only after the user approves the finished UI direction; E1's existing
smoke/security guardrail remains in place meanwhile.

Independent work, if desired:

- B1–B2 can follow A1 while A2–A6 are developed elsewhere; merge the auth/catalog changes first.
- C1 normalization and C3 geometry can be tested independently of B, but final UI integration needs B1.
- D4 structured SDK capture is independent of UI implementation but must honor C1's wire fixtures.
- E1 acceptance harness runs early after B1 and is a prerequisite for D6, avoiding a circular
  test-fixture dependency. E2 publishing workflow/Compose can be prepared independently; do not
  publish/advertise M7 before E4.
- A3/A4/A5/C5/D1 all affect generated OpenAPI and possibly migrations: serialize these merges and
  regenerate after merging. Do not resolve generated-file conflicts manually.

## 3. File ownership map

| Area | Existing entry points | New units |
|---|---|---|
| Run lifecycle | `web/src/features/runs/use-run-polling.ts`; Go eval model/API | `run-status.ts`, immutable run-item migration, lifecycle tests |
| Auth/catalog | `web/src/auth/auth-context.tsx`, `api/client.ts` | application catalog context; session-generation fencing |
| Dataset mutation | `internal/domain/evaluations.go`, store/API datasets, SQL | focused dataset mutation domain/API files |
| Shared UI | `styles.css`, shell, modal, JSON/score components | small UI primitives, theme provider, shared problem states |
| Management | applications/datasets pages, generated client | project/key screens, app settings, case editor/import/export |
| Trace interpretation | captured-content, trace-detail, span-tree | message normalization, call groups, conversation, timing geometry/waterfall |
| Trace queries/actions | trace domain/API/store/SQL | query summaries, scoring eligibility, import service |
| Evaluation review | run detail, run items/scores APIs | item snapshots response, result table, comparison service/page |
| SDK | `tracing.py`, `conventions.py`, `client.py`, `models.py`, `cli.py` | messages module; lifecycle/analytics method extensions |
| Delivery | Dockerfile, Compose, README, workflows | published Compose, publish workflow, Linux guide, browser acceptance |

Backend entries in the table are under `assayd/`; SDK entries under
`clients/python/assay/src/assay/`. Subplans give repository-relative exact paths.

## 4. Shared verification commands

Run the relevant subset for every task; run cross-layer gates when a contract changes.

```bash
# From repository root; execution shell/scripts use set -euo pipefail.
uv sync --project clients/python/assay --frozen
(cd web && corepack enable && pnpm install --frozen-lockfile)
prek install

# Frontend task: append the task's actual test file names.
(cd web && pnpm test src/features/runs/use-run-polling.test.tsx)
(cd web && pnpm lint && pnpm format:check && pnpm typecheck)

# SQL or API changes.
(cd assayd && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate)
(cd web && pnpm generate:api)
# Commit regenerated output with source. Rerun generation before the drift check.
git diff --exit-code -- assayd/internal/store/sqlc web/openapi.json web/src/api/generated

# Backend: use task-specific -run selectors during development.
(cd assayd && go test -race -count=1 ./internal/domain ./internal/store ./internal/api)
(cd assayd && golangci-lint run)

# SDK: substitute the task's named pytest files.
uv run --project clients/python/assay pytest -q clients/python/assay/tests/test_conventions.py
uv run --project clients/python/assay ruff check clients/python/assay
uv run --project clients/python/assay ruff format --check clients/python/assay
uv run --project clients/python/assay ty check --project clients/python/assay

# Before a task commit, after reviewing changes.
git diff --check
prek run
```

The drift check compares to the current index/commit, so expected newly generated changes are not a
failure to hide: review and stage/commit them first, rerun generation, then expect an empty diff.
Database tests use `internal/testutil.Postgres(t)` disposable containers, never root `.env` credentials.
If Docker is unavailable the repository may skip tests locally; CI must actually execute them.
Do not lower runtime pins merely because the shell lacks tools. Follow the checked-in workflow versions.

Before adding Playwright/axe or an optional Markdown renderer: read current official docs, resolve and
record exact stable package versions, run dependency audit, enforce the existing minimum release age
and disabled lifecycle scripts. Do not install `latest` as an unreviewed permanent dependency. Use
existing packages for all other planned UI behavior. Pin new Actions to full verified SHAs.

## 5. Requirements coverage

| User observation | Delivery |
|---|---|
| 1: CRUD across UI | A2–A5, B3–B6, C6, D2/D5; lifecycle exceptions explicit in design §5 |
| 2: Gantt per span | A6 and C3–C4; actual relative timing and concurrency |
| 3: conversation primary overview | C1–C2/C4, D4 capture helper |
| 4: Odysseus-inspired polish | B1–B2, C4, E4 visual/accessibility gates |
| 5: tracing/evaluation focus; backend/SDK changes allowed | A/C/D vertical workflows, comparison, source evidence |
| 6: minimal SDK configuration/code | D4–D6, B4 onboarding, E3 runnable examples |
| 7: Linux documentation | E3, with PowerShell parity |
| 8: published image and wired Compose/env explanation | E2–E3; publishing permission gate |
| 9: exact Docker command | E2–E3 |

Additional proposed improvements are evidence-led: pending-state/auth fixes, immutable run evidence,
server-side trace search, item-level evaluation review, same-case comparison, safe capture fallbacks,
accessible states, secret-safe onboarding, and verified release delivery. They are not a new platform
roadmap disguised as polish.

## 6. Completion record

Keep task checkboxes in their subplans. At each milestone append actual verification evidence here:
commit, commands, tests executed versus skipped, synthetic demo artifacts, reviewer approval. Leave
this section without a completion claim until those checks run.

Implementation is intentionally not started by the planning session. The next action is design review,
then task A1, not a mass rewrite or an image push.

### Planning verification (2026-09-08)

- Checked seven documents, 18 local links, 28 task IDs, and balanced fenced examples.
- Checked Python example syntax and JSON examples; validated the proposed published Compose using
  `docker compose config --quiet` with synthetic values and no root `.env` file.
- Confirmed plans are not Git-ignored; this repository excludes `docs/superpowers/`, so these durable
  handoff documents use `docs/plans/` instead.
- No application code changed, no application test suite executed, no containers started or images
  published by planning. Runtime acceptance remains required during implementation.

### Implementation verification (2026-09-08)

Branch `feat/product-completion`; subplan checkboxes A1–A6 and B1–B3 are ticked. Untracked plan,
spect, and `improvements.md` files are preserved.

**M7A commits (backend/data safety):**

| Commit | Task | Evidence |
|---|---|---|
| `3188b5e` | A1–A2 | Typed eval-status enum + generated clients; run polling/cancel lifecycle; 401 credential fencing; `ApplicationCatalogProvider` refresh |
| `e9181f1` | A3 | Migration `00006` snapshot columns + backfill; run-item reads switch to immutable `snapshot_*`; FK to live cases dropped |
| `7b54391` | A4 | Dataset PATCH + scoped item GET/PUT/DELETE endpoints; parent-scoped SQL; duplicate external-id 409 |
| `7ac0090` | A3 completion | Copied-snapshot count/validity checks on run create; `snapshot_origin` in run-item API; rollback regression |
| `2ff0259` | A5 | Admin queue/attach/delete trace ops with project isolation; terminal-only eval-run deletion (409 active); lifecycle store/domain units |
| `de85e78` | A6 | Cycle-safe span-tree transform (duplicate IDs to earliest row; functional-graph cycle breaking) + stability tests |

**M7B commits (B1–B3):**

| Commit | Task | Evidence |
|---|---|---|
| `bea4904` | B1 | Light/dark theme tokens + `data-theme` resolution, `ThemeProvider`, Radix Dialog/Button/Field/StatusBadge, shared problem/empty/loading states; old Modal removed |
| `a85bc43` | B2 | 224px rail + 56px header, global Applications/Projects nav, Evaluations/Score trends labels, theme selector, Dialog drawer, new routes, retryable metrics |
| `9970195` | B3 | Projects list/detail CRUD with typed-name delete, API-key panel: one-time secret, copy feedback, revoke confirm, no secret persistence |

**Verification commands executed (all passing):**

```text
(cd assayd && go test -race -count=1 ./...)
(cd assayd && golangci-lint run)
(cd web && pnpm test)              # 84 tests, 17 files
(cd web && pnpm lint)
(cd web && pnpm typecheck)
(cd web && pnpm format:check)
(cd web && pnpm build)             # production bundle into assayd/internal/ui/dist
(cd assayd && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate)
(cd web && pnpm generate:api)
```

Generated `assayd/internal/store/sqlc`, `web/openapi.json`, and `web/src/api/generated` are committed
with their sources; drift check clean after regeneration. API/schema changes: migrations now at
version 6; run-item responses carry `snapshot`/`snapshot_origin`; dataset PATCH and item-level
routes; admin DELETE trace/run; OpenAPI regenerated for every contract change.

**Not yet done:** B4–B6, C1–C6, D1–D6, E1–E4. Milestone demo gates (browser review of pending-run
poll/cancel, snapshot-preserving edits, admin trace mutations, cyclic-span visibility, both-theme
usability, project/key workflows) still require human approval before release work.
