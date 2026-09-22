## Now

- E2 delivery implementation is complete locally: reproducible amd64/arm64 builds, separate published Compose, guarded smoke coverage, and a protected GHCR workflow. Actual publication and anonymous-pull evidence remain an approval gate.
- E1 is complete on `feat/e1-acceptance-harness`: an isolated Compose stack, deterministic fake judge/target, and embedded Playwright/axe suite run without paid services and clean up only their prefixed project.
- M7D6 is complete on `feat/m7d6-sdk-acceptance`: its disposable SDK acceptance covers trace capture, scoring, retained evidence, evaluation lifecycle, and failure paths without paid services.
- E3 implementation is complete locally: Linux and PowerShell guides, deployment/recovery guidance, current-checkout SDK examples, and E1-only docs smoke are in place. The dynamic E1 docs workflow needs a runnable local Python/Playwright environment, and PowerShell still needs Windows-runner evidence.

## Next

- Record E3's disposable Linux workflow and Windows PowerShell verification when their environments are available.
- Then redo the UI with the user and lock the approved state through E4 browser/accessibility/visual/performance acceptance.

## Done

- 2026-09-22 Backend CI flake fixed: app integration-test HTTP clients now close idle keep-alive connections before asserting graceful shutdown; the failing SHA reproduced the timeout under load, and five repeated race runs pass after the fix.
- 2026-09-22 E1 shipped: disposable embedded-build acceptance with deterministic judge/target fixtures, guarded cleanup, CSP/browser-error enforcement, keyboard coverage, and both-theme 360/1440px axe scans.
- 2026-09-20 M7D1 shipped: admin-scoped run-item lookup, non-null score arrays, set-based page enrichment, repeatable-read consistency, generated OpenAPI/TypeScript, and regressions for source deletion, 101-score boundaries, and concurrent completion.

- 2026-09-11 PR #9 passed Docker-backed PostgreSQL integration tests, Go race detection, frontend production checks, and Python checks in CI.
- 2026-09-11 closed `findings.md`: scoped deletion locking, single-source application loading with error presentation, protected one-time keys, blank expected-output normalization, narrowed docs ignore, and populated migration upgrade coverage.

- 2026-09-11 M7C5 and M7C6 merged to `main` in PR #8 (`41e3a6d`). M7C6 shipped eligibility, reference/score/save-to-dataset trace-detail actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.

- Frontend trace filters now drop `passed` without a valid scorer before serializing or requesting.
- Trace pagination tests use distinct IDs, assert filters on both load-more requests, and verify reset results.
