## Now

- E2 delivery implementation is complete locally: reproducible amd64/arm64 builds, separate published Compose, guarded smoke coverage, and a protected GHCR workflow. Actual publication and anonymous-pull evidence remain an approval gate.
- E1 is complete on `feat/e1-acceptance-harness`: an isolated Compose stack, deterministic fake judge/target, and embedded Playwright/axe suite run without paid services and clean up only their prefixed project.
- M7D6 is complete on `feat/m7d6-sdk-acceptance`: its disposable SDK acceptance covers trace capture, scoring, retained evidence, evaluation lifecycle, and failure paths without paid services.
- E3 local workflows are now exercised: Linux Docker docs smoke and Windows PowerShell 7.6.6 credential/bootstrap/trace/import checks pass. Optional documented real-judge evaluation and GitHub-run evidence remain unverified.
- 2026-09-23 persistent validation of `aa30031`: Assay at `http://127.0.0.1:18080`, interactive SDK dummy at `http://127.0.0.1:18090`, Compose project `assay-acceptance-persistent`. Workspace/report: `C:/Users/mario/sources/assay-local-validation/REPORT.md`; the admin credential is in its private `.env`. Original port-8080 stack untouched. Projects, agent runs, screenshots, backup, and restored check database are retained; forced recreation preserved all data.
- Core checks pass: 323 Linux Go race tests, 158 web tests, 239 Python tests (one real-judge skip), 9 E1 browser tests after the follow-up, dummy HTTP/SDK workflows, and a separate skill agent's 37 CLI outcomes. The three populated-page accessibility defects were fixed on 2026-09-24; 48/48 pages now pass axe and browser errors remain zero. The persistent stack now runs rebuilt `assay-local-validation:a11y-fix` without replacing its Postgres volume. Follow-up: `C:/Users/mario/sources/assay-local-validation/FOLLOWUP-2026-09-24.md`.

## Next

- Keep the persistent validation stack/data for user inspection; do not remove its named volume. See the workspace report for start/stop commands and exact evidence.
- Keep the populated-page accessibility regression; remaining UI redesign and E4 cross-browser/visual/performance acceptance require user approval. The Assay skill guidance was refreshed and its live CLI workflow was rechecked read-only on 2026-09-24.
- Obtain remaining GitHub/release evidence, then lock the user-approved UI through E4 browser/accessibility/visual/performance acceptance. Local validation is not an E4 sign-off.

## Done

- 2026-09-22 Backend CI flake fixed: app integration-test HTTP clients now close idle keep-alive connections before asserting graceful shutdown; the failing SHA reproduced the timeout under load, and five repeated race runs pass after the fix.
- 2026-09-22 E1 shipped: disposable embedded-build acceptance with deterministic judge/target fixtures, guarded cleanup, CSP/browser-error enforcement, keyboard coverage, and both-theme 360/1440px axe scans.
- 2026-09-20 M7D1 shipped: admin-scoped run-item lookup, non-null score arrays, set-based page enrichment, repeatable-read consistency, generated OpenAPI/TypeScript, and regressions for source deletion, 101-score boundaries, and concurrent completion.

- 2026-09-11 PR #9 passed Docker-backed PostgreSQL integration tests, Go race detection, frontend production checks, and Python checks in CI.
- 2026-09-11 closed `findings.md`: scoped deletion locking, single-source application loading with error presentation, protected one-time keys, blank expected-output normalization, narrowed docs ignore, and populated migration upgrade coverage.

- 2026-09-11 M7C5 and M7C6 merged to `main` in PR #8 (`41e3a6d`). M7C6 shipped eligibility, reference/score/save-to-dataset trace-detail actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.

- Frontend trace filters now drop `passed` without a valid scorer before serializing or requesting.
- Trace pagination tests use distinct IDs, assert filters on both load-more requests, and verify reset results.
