## Now

- New feature plan proposed in `docs/plans/2026-09-25-session-explorer-and-ui.md`: OTel `session.id`-tagged cross-trace Sessions view, scoped SDK helper, backend-issued persistent demo-chat session plus pseudonymous browser ID, conversation-first trace/session/Gantt UI, and midnight-slate/teal dark-first redesign. The user chose persistence until New Chat, backend-issued IDs, Traces retaining all data, and this palette. The user approved the small desktop/mobile visual preview, then requested its folder be removed; the design direction is recorded in the plan. Future working mockups may live in a hidden, locally gitignored folder during implementation, but should not be committed. No session feature implementation or migration applied. Start feature work separately after this chat branch is pushed.
- The existing `python-qa-example` project and chat now run locally at `127.0.0.1:8080` and `127.0.0.1:8090` on `feat/local-general-chat`. General and Assay-specific replies are traced; only explicit Assay questions are eligible for local Mistral judging (one repeat score failed strict JSON validation). Private Ollama generates with Qwen3 on CPU, containers are healthy, and the project/Postgres/model volumes persist. The chat code and tests are committed as `8ccb3f6`; see `../assay-local-validation/CHAT-2026-09-25.md` for local validation.
- Accessibility PR #19's `test` failure came from requiring `rg` on the GitHub runner. On its branch, current `main` was merged to resolve note conflicts and the container smoke now uses portable `grep`; the Linux CI rerun must pass before merging.
- E2 delivery implementation is complete locally: reproducible amd64/arm64 builds, separate published Compose, guarded smoke coverage, and a protected GHCR workflow. Actual publication and anonymous-pull evidence remain an approval gate.
- E1 is complete on `feat/e1-acceptance-harness`: an isolated Compose stack, deterministic fake judge/target, and embedded Playwright/axe suite run without paid services and clean up only their prefixed project.
- M7D6 is complete on `feat/m7d6-sdk-acceptance`: its disposable SDK acceptance covers trace capture, scoring, retained evidence, evaluation lifecycle, and failure paths without paid services.
- E3 local workflows are now exercised: Linux Docker docs smoke and Windows PowerShell 7.6.6 credential/bootstrap/trace/import checks pass. Optional documented real-judge evaluation and GitHub-run evidence remain unverified.
- 2026-09-23 persistent validation of `aa30031`: Assay at `http://127.0.0.1:18080`, interactive SDK dummy at `http://127.0.0.1:18090`, Compose project `assay-acceptance-persistent`. Workspace/report: `C:/Users/mario/sources/assay-local-validation/REPORT.md`; the admin credential is in its private `.env`. Original port-8080 stack untouched. Projects, agent runs, screenshots, backup, and restored check database are retained; forced recreation preserved all data.
- Core checks pass: 323 Linux Go race tests, 158 web tests, 239 Python tests (one real-judge skip), 9 E1 browser tests after the follow-up, dummy HTTP/SDK workflows, and a separate skill agent's 37 CLI outcomes. The three populated-page accessibility defects were fixed on 2026-09-24; 48/48 pages now pass axe and browser errors remain zero. The persistent stack now runs rebuilt `assay-local-validation:a11y-fix` without replacing its Postgres volume. Follow-up: `C:/Users/mario/sources/assay-local-validation/FOLLOWUP-2026-09-24.md`.

## Next

- Keep the persistent validation stack/data for user inspection; do not remove its named volume. See the workspace report for start/stop commands and exact evidence.
- Confirm PR #19's updated Linux CI and mergeability; do not claim success from the Windows-local partial container smoke.
- Review/approve the remaining session explorer contract and signed anonymous history cookie; implement in small verified phases on a separate branch. Check empty/error and light-theme states during implementation.
- Ask before revoking seven historical active `temporary-example-run` project keys from ungraceful shutdowns; improve the local judge's intermittent structured-output reliability.
- Retain the populated-page accessibility regression and Assay skill guidance. Obtain remaining GitHub/release evidence, then lock the approved UI through E4 browser/accessibility/visual/performance acceptance. Local validation is not E4 sign-off.

## Done

- 2026-09-22 Backend CI flake fixed: app integration-test HTTP clients now close idle keep-alive connections before asserting graceful shutdown; the failing SHA reproduced the timeout under load, and five repeated race runs pass after the fix.
- 2026-09-22 E1 shipped: disposable embedded-build acceptance with deterministic judge/target fixtures, guarded cleanup, CSP/browser-error enforcement, keyboard coverage, and both-theme 360/1440px axe scans.
- 2026-09-20 M7D1 shipped: admin-scoped run-item lookup, non-null score arrays, set-based page enrichment, repeatable-read consistency, generated OpenAPI/TypeScript, and regressions for source deletion, 101-score boundaries, and concurrent completion.

- 2026-09-11 PR #9 passed Docker-backed PostgreSQL integration tests, Go race detection, frontend production checks, and Python checks in CI.
- 2026-09-11 closed `findings.md`: scoped deletion locking, single-source application loading with error presentation, protected one-time keys, blank expected-output normalization, narrowed docs ignore, and populated migration upgrade coverage.

- 2026-09-11 M7C5 and M7C6 merged to `main` in PR #8 (`41e3a6d`). M7C6 shipped eligibility, reference/score/save-to-dataset trace-detail actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.

- Frontend trace filters now drop `passed` without a valid scorer before serializing or requesting.
- Trace pagination tests use distinct IDs, assert filters on both load-more requests, and verify reset results.
