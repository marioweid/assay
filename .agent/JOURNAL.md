## 2026-09-10T15:20Z — M7C5 review blocked

[OUTCOME] M7C5 remains uncommitted after verification largely passed. Final review is blocked because independent pool statements for the filtered trace page and score summaries can disagree if a rescore occurs between them.
[DISCOVERY] The frontend URL canonicalization request still needs rerunning, and trace pagination tests are weak.

## 2026-09-10T19:35Z — M7C5 snapshot consistency fixed

[OUTCOME] `ListTraces` now reads its filtered page and score summaries through one read-only repeatable-read transaction. A deterministic pgx-traced integration test pauses before the summary query, commits a newer passing score, and verifies the response still returns the originally filtered failing summary.
[VERIFY] The regression test first failed before the transaction change; focused test, `go build ./...`, `golangci-lint run`, and `go test -race -count=1 ./...` pass. `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` completed without new generated output.

## 2026-09-10T21:43Z — M7C5 frontend filter regressions fixed

[OUTCOME] Orphaned `passed` URL filters are removed before a trace request, preventing the backend validation error. Trace pagination coverage now uses distinct trace IDs, checks both cursor requests retain score filters, and proves a cleared search replaces prior results.
[VERIFY] Focused trace Vitest, frontend typecheck/lint/format/test/build, and `git diff --check` pass. pnpm reported its Node 24 runtime is outside the project’s Node 22 engine range.

## 2026-09-10 — M7C5 10,000-row EXPLAIN evidence

[VERIFY] Using the running local Compose Postgres, a rolled-back transaction seeded 10,000 traces and 10,000 online scores. `EXPLAIN (ANALYZE, BUFFERS)` returned the bounded 51-row page in 0.207 ms via `traces_application_start_idx`; its 51 latest-score lookups used `scores_*_trace_id_created_at_id_idx`. The 51-ID score-summary query returned in 0.166 ms via the same score index. Neither query scanned spans or issued per-row summary queries; empty score partitions were scanned only because the partitioned table has no matching partition constraint.

## 2026-09-10 — M7C5 review cleanups

[OUTCOME] The trace load-more regression now asserts start, end, status, q, scorer, and passed on both cursor requests. Test-only score insert helpers now use input structs, staying within the positional-parameter limit.
[VERIFY] Focused and full store race tests, all backend race tests/build/lint, and focused and full frontend tests/lint/format/typecheck/build passed. Local pnpm continues to warn that Node 24 is outside the required Node 22 range.

## 2026-09-10T20:08Z — M7C5 final gate

[OUTCOME] Final review approved with no blockers or should-fix items. M7C5 server-side trace discovery is fully verified and remains uncommitted, ready for user review and commit.

## 2026-09-11T20:06Z — M7C6 Trace Actions implementation review

[OUTCOME] M7C6 Trace Actions shipped and passed independent review plus focused recheck repairs. The approved scope includes trace scoring eligibility, reference/score/save-to-dataset actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.
[VERIFY] Go build, lint, race tests, UI/app production checks, web lint/format/typecheck, 139 Vitest tests, build, OpenAPI/TypeScript generation comparisons, sqlc regeneration, and `git diff --check` passed. pnpm emitted known Node 24 engine warnings while shell Node reports 22. Normal `uv run` Python gates could not start because the project `.venv` and fresh uv build executables hit NixOS's dynamic-linker stub; source lines were manually line-length checked. M7C5 remains uncommitted alongside M7C6.

## 2026-09-11T20:41Z — Post-merge findings closed

[OUTCOME] Replaced global jobs-table deletion locks with deterministic target-scoped row locks across trace, run, dataset, application, and project deletion. Authentication now seeds the application catalog without a duplicate request and catalog errors render truthfully. One-time API keys resist Escape/outside dismissal, blank Python expected outputs normalize to `None`, the docs ignore is scoped, and migration 5 → 6 now has a populated historical-run test. Independent concurrency-focused review passed with no findings.
[VERIFY] Go build, non-race tests, golangci-lint v2.13.1, and sqlc v1.31.1 generation pass. Frontend 140 tests, lint, typecheck, format, and production build pass. Python 158 tests pass with one opt-in live test skipped; ruff and ty pass. Docker-backed PostgreSQL cases were skipped because Docker is unavailable. Go race detection could not run because CGO is disabled and no C compiler is installed.

## 2026-09-11T20:51Z — Post-merge findings landed

[OUTCOME] PR #9 merged to `main` as `132cba7` through the repository PR workflow.
[VERIFY] Backend CI passed in 3m08s, including Docker-backed PostgreSQL integration tests and Go race detection. Web CI passed in 2m07s, and Python CI passed in 10s.

## 2026-09-11T20:56Z — Reserved-name artifact removed

[OUTCOME] Identified root `nul` as redirected `where.exe` stderr created by Windows-style `2>NUL` under Git Bash. Renamed and moved `nul` plus the empty `nul.txt` fallback artifact to a temporary directory, then sent both safely to the Windows Recycle Bin. Future Git Bash commands use `/dev/null`.
[VERIFY] `git status` no longer reports either reserved-name artifact.

## 2026-09-20T07:42Z — M7D1 evaluation item evidence complete

[OUTCOME] Paged evaluation items now include complete persisted score evidence, and admin-only scoped item lookup returns the same immutable snapshot after source deletion. Both reads use one repeatable-read snapshot, preventing impossible running-item/new-score combinations during worker completion. Independent review passed after the consistency repair.
[VERIFY] The deterministic interleaving test failed under read committed and passed under repeatable read for list and direct reads. Focused race tests, full Go tests, build, golangci-lint, 140 web tests, web lint/format/typecheck/build, generation stability, and `git diff --check` passed. pnpm reported the known Node 24 versus required Node 22 engine warning.

## 2026-09-20T07:50Z — M7D2 case outcomes started

[OUTCOME] Run detail now loads cursor-paginated case outcomes, labels execution failures separately from absent quality scores, and preserves zero-valued failed scores. Shared score cards consistently format score values to two decimals.
[VERIFY] The score regression failed before formatting was fixed. All 142 web tests, lint, format, typecheck, production build, and `git diff --check` passed; pnpm reported the known Node 24 versus required Node 22 engine warning.

## 2026-09-20T07:54Z — M7D2 direct case evidence

[OUTCOME] Run cases now open URL-backed direct detail through the scoped item endpoint, showing original input/context/reference, recorded versus generated output, execution errors, score rationale, provenance, and raw details.
[VERIFY] Focused run tests and all 142 web tests, lint, format, typecheck, production build, and `git diff --check` passed.

## 2026-09-20T14:07Z — M7D2 bounded item polling

[OUTCOME] Run detail pagination now replaces the visible cursor page instead of accumulating every prior page. Active run updates refresh that page, including one final terminal refresh, while page navigation retains back/forward cursor state.
[VERIFY] Focused polling and pagination regressions plus all 144 web tests, lint, format, typecheck, production build, and `git diff --check` passed. pnpm reported the known Node 24 versus required Node 22 range.

## 2026-09-20T14:14Z — M7D2 run lifecycle and export

[OUTCOME] Terminal runs now support typed-name deletion with 409 state refresh, current-state reruns through the existing prefilled creation form, and complete JSONL evidence export. Browser export follows cursors and stops on repeated cursors, 10,000 cases, or 50 MiB; larger runs direct users to the CLI.
[OUTCOME] Direct case detail now labels snapshot provenance and includes generated context alongside original and generated evidence.
[VERIFY] Focused lifecycle, rerun, pagination, and export tests passed with lint and typecheck. Independent review found item polling tied to a non-advancing parent timestamp, unsafe retries after uncertain create/delete outcomes, and non-exact whitespace confirmation. Polling now uses every successful parent poll as a page refresh generation; uncertain mutations remain disabled pending list reconciliation; confirmation is byte-for-byte exact. Focused recheck found create uncertainty could be erased by dialog dismissal, so in-flight/uncertain dialogs now block dismissal and provide an explicit parent-owned run-list reconciliation action. Regressions cover each repair.

## 2026-09-21T07:46Z — M7D3 paired run comparison

[OUTCOME] Terminal runs can be compared by immutable dataset item ID and a shared scorer. One set-based Postgres read selects deterministic latest scores, classifies matched/changed/one-sided/unscored cases, computes all-page paired delta aggregates, and reports context or judge-configuration mismatches. Bound cursors prevent reuse across another pair/scorer and retain full per-case evidence.
[OUTCOME] The runs UI selects explicit terminal baseline/candidate/scorer values, preserves them in the URL, supports swapping, explains the denominator and exclusions, and exposes responsive keyboard-expandable side-by-side evidence.
[VERIFY] Domain classification tests, Docker-backed store/API integration tests, sequential race tests, backend vet/golangci-lint, 158 web tests, frontend lint/typecheck/format/build, generated client stability, and `git diff --check` pass. Independent review found optional generated query parameters and unbounded pre-page evidence JSON construction. The parameters are now required end to end, and SQL materializes scalar classification data before joining and encoding only the bounded evidence page. Focused recheck approved both repairs with no remaining findings.

## 2026-09-21T08:22Z — M7D4 structured Python capture

[OUTCOME] The SDK exports typed OpenTelemetry GenAI text/tool messages and an explicit `AssaySpan.set_messages` helper. Complete input/output arrays are redacted, recursively JSON-validated, size-checked including their envelopes, and serialized before any attribute is changed. Active spans expose lowercase 32/16-character trace/span IDs without replacing the global provider or enabling decorator capture.
[VERIFY] Structured-message, tracing, conventions, exporter, and package regressions pass on Python 3.13 and the declared Python 3.10 floor. The installed-package suite passes 182 tests with one opt-in live test skipped; ruff, ty, the backend text-parts/tool-only scorer regression, domain race tests, and `git diff --check` pass. Independent review identified a secret-bearing custom-collection exception path; normalization now converts every ordinary exception to a content-free error. Focused recheck confirmed the repair and verified that explicit null participant names/tool IDs match the pinned official schema.

## 2026-09-22T05:45Z — M7D5 typed client and CLI parity

[OUTCOME] The Python SDK now strictly parses immutable run-item snapshots and scores, paired run comparisons, trace score summaries, and scoring eligibility. Typed resources cover dataset mutation/item lifecycle, run inspection/comparison/deletion, and trace filtering/eligibility/deletion with project-key-preferred trace auth and explicit admin fallback. The CLI now exposes the complete project, key, application, dataset/item, scorer, run, and trace workflow matrix; file-backed settings stay out of argv, exports follow guarded cursors as JSONL, and every delete/revoke/endpoint-clear requires `--yes`.
[VERIFY] The installed Python 3.13 package suite passes 238 tests with one opt-in live test skipped. Python 3.10 passes the 70 focused D5/CLI tests; ruff and ty pass against the declared 3.10 target, console-script help smoke passes, and `git diff --check` is clean. Independent review found required-nullable comparison fields were treated as optional; dedicated nullable parsers now reject omission while preserving explicit JSON null, with regressions for row evidence and summary delta.

## 2026-09-22T05:53Z — M7D6 prerequisite confirmed missing

[DISCOVERY] M7D6 cannot be executed honestly yet: its required E1 disposable acceptance harness, isolated Compose project, and deterministic fake judge/target have not been implemented. The repository has no `tests/acceptance`, acceptance fixture command, or Playwright harness. The paid opt-in M6 live test is intentionally not a substitute and no destructive test will default to localhost.
[NEXT] Implement E1 before creating or claiming the D6 end-to-end acceptance result.

## 2026-09-22T06:24Z — E1 disposable acceptance harness complete

[OUTCOME] E1 now builds an isolated `assay-acceptance-*` Compose stack with fresh Postgres storage, generated 0600 credentials, deterministic stdlib Go judge/target responses, guarded teardown, and no default to the ordinary localhost service. Playwright seeds workspaces and traces through public APIs, checks embedded CSP/browser errors, keyboard modal/drawer behavior, and axe at 360/1440px in both themes. Browser execution exposed Radix's fixed scroll-lock style; CSP now permits only its exact hash rather than `unsafe-inline`.
[VERIFY] Exact pins `@playwright/test==1.63.0` and `@axe-core/playwright==4.13.0` were resolved from npm; `pnpm audit --audit-level=moderate` reports no findings after updating the existing `js-yaml` override to 4.3.2. All 8 embedded browser tests, 158 Vitest tests, Go fixture/UI tests, focused golangci-lint, frontend lint/format/typecheck/build, shellcheck, shfmt, actionlint, zizmor, Compose validation, and endpoint-refusal checks pass. Teardown preserved the three pre-existing Compose containers with unchanged IDs and left no acceptance images or volumes. Independent review's cleanup-error blocker was repaired and the focused recheck approved it.
[NEXT] M7D6 is unblocked and can consume this harness for the no-paid-service SDK workflow.

## 2026-09-22T16:08Z — Backend shutdown-test flake fixed

[OUTCOME] Reproduced the failed `main` SHA under repeated race runs. `TestAppMigratesBeforeServingAndStops` timed out while `http.Server.Shutdown` waited on test-owned keep-alive connections; a goroutine dump ruled out the worker pool and database. The M2, M3, and readiness helpers now close idle client connections before the five-second shutdown assertion.
[VERIFY] The unmodified failing SHA failed in both five-run and instrumented three-run stress executions. After the fix, `go test -race -count=5 ./...`, the exact CI command `go test -race -count=1 ./...`, `go build ./...`, golangci-lint, gofmt, and `git diff --check` pass.

## 2026-09-22T14:27Z — M7D6 SDK lifecycle acceptance complete

[OUTCOME] Added a disposable E1-backed SDK acceptance test covering structured/capture-off traces, eligibility, scoring, retained-evidence import conflicts, snapshot-preserving replacement, transient retry, terminal execution failure, cancellation/deletion, CLI failures, and revoked-key ingestion denial. `ASSAY_ACCEPTANCE_KEEP=1` now preserves a successful disposable stack and its synthetic dashboard data on explicit request.
[VERIFY] The new test failed first when its CLI lacked admin credentials and when its target context mapping selected objects rather than strings; both corrections were verified by a passing Docker-backed run. Docker-run package checks pass: 238 tests, 2 skips, ruff, format, and ty. The Python Q&A example passes 8 tests plus ruff/format/ty. Browser E1 could not launch locally because NixOS rejects Playwright's dynamically linked Chromium; the existing browser suite was not bypassed. `shellcheck` and `shfmt` are unavailable locally; `bash -n`, explicit invalid-keep rejection, and Compose validation pass. A preserved `assay-acceptance-d6-*` stack is healthy at port 18080 for dashboard inspection.

## 2026-09-22T14:36Z — Local persistent D6 demonstration

[OUTCOME] With explicit approval, rebuilt only the source-Compose `assayd` container against current `main`; the existing Postgres volume was preserved. A deterministic fake fixture container joined the Compose network as `fixtures`, and the complete synthetic D6 project was retained in the localhost dashboard.
[VERIFY] The first localhost attempt exposed the old container's pre-D5 eligibility response shape; the incomplete project was removed. The rebuilt service passed `tests/test_product_acceptance.py` against `http://127.0.0.1:8080` with `ASSAY_ACCEPTANCE_KEEP=1`, using a project-scoped fake judge and target rather than configured paid providers.

## 2026-09-22 — UI sequencing updated

[DECISION] Finish the remaining functional/delivery milestones before redesigning the UI with the user. Defer broad frontend E2E workflows, visual baselines, accessibility expansion, and performance acceptance until that redesign is approved; retain E1 smoke/security coverage until then.
[NEXT] E2 and E3 proceed before the final E4 UI lock and acceptance gates.

## 2026-09-22 — E2 container delivery implementation complete

[OUTCOME] Added OCI-labelled cross-platform Docker builds, loopback-only source Compose, a no-build published Compose, a protected GHCR release workflow, and a disposable image smoke test. Semantic version validation, serialized publication, overwrite refusal, SBOM/provenance-aware manifest verification, and anonymous digest pull are enforced in the release path.
[VERIFY] Local amd64 labelled image smoke passed: embedded hashed assets/icon/font, readiness, SPA deep link, protected API, distroless healthcheck, tracing-only boot, and Linux host-gateway mapping. Source/published Compose validate with synthetic values; missing published variables fail; YAML parsing, Bash syntax, and `git diff --check` pass. An arm64 image cross-build passed, but local runtime smoke is blocked because this Docker daemon lacks arm64 binfmt; the protected workflow installs QEMU and runs that smoke.
[BLOCKED] No image was published or anonymously pulled: maintainer approval, GHCR public visibility, and a release tag are required.
