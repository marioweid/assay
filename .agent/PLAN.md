## Now

- New feature plan proposed in `docs/plans/2026-09-25-session-explorer-and-ui.md`: OTel `session.id`-tagged cross-trace Sessions view, scoped SDK helper, backend-issued persistent demo-chat session plus pseudonymous browser ID, conversation-first trace/session/Gantt UI, and midnight-slate/teal dark-first redesign. The user chose persistence until New Chat, backend-issued IDs, Traces retaining all data, and this palette. The user approved the small desktop/mobile visual preview, then requested its folder be removed; the design direction is recorded in the plan. Future working mockups may live in a hidden, locally gitignored folder during implementation, but should not be committed. No session feature implementation or migration applied. Start feature work separately after this chat branch is pushed.
- The existing `python-qa-example` project and chat now run locally at `127.0.0.1:8080` and `127.0.0.1:8090` on `feat/local-general-chat`. General and Assay-specific replies are traced; only explicit Assay questions are eligible for local Mistral judging (one repeat score failed strict JSON validation). Private Ollama generates with Qwen3 on CPU, containers are healthy, and the project/Postgres/model volumes persist. The chat code and tests are committed as `8ccb3f6`; see `../assay-local-validation/CHAT-2026-09-25.md` for local validation.
- Previously pushed accessibility PR #19 has a failing `test` check: GitHub runner lacks `rg` for container smoke. This separate follow-up is not part of the chat branch.
- E2 delivery implementation is complete locally: reproducible amd64/arm64 builds, separate published Compose, guarded smoke coverage, and a protected GHCR workflow. Actual publication and anonymous-pull evidence remain an approval gate.
- E1 is complete on `feat/e1-acceptance-harness`: an isolated Compose stack, deterministic fake judge/target, and embedded Playwright/axe suite run without paid services and clean up only their prefixed project.
- M7D6 is complete on `feat/m7d6-sdk-acceptance`: its disposable SDK acceptance covers trace capture, scoring, retained evidence, evaluation lifecycle, and failure paths without paid services.
- E3 implementation is complete locally: Linux and PowerShell guides, deployment/recovery guidance, current-checkout SDK examples, and E1-only docs smoke are in place. Web CI now provisions the pinned `uv` required by E1's SDK/doc checks; GitHub-run evidence is pending. The dynamic E1 docs workflow needs a runnable local Python/Playwright environment, and PowerShell still needs Windows-runner evidence.

## Next

- Review/approve the remaining session explorer contract and signed anonymous history cookie; implement in small verified phases on a separate branch after this chat branch is pushed. Check empty/error and light-theme states during implementation.
- Ask before revoking seven historical active `temporary-example-run` project keys from ungraceful shutdowns; improve the local judge's intermittent structured-output reliability, and repair PR #19's runner prerequisite in its own branch.
- Record E3's disposable Linux workflow and Windows PowerShell verification when their environments are available.
- Complete the approved UI direction and lock the final state through E4 browser/accessibility/visual/performance acceptance.

## Done

- 2026-09-22 Backend CI flake fixed: app integration-test HTTP clients now close idle keep-alive connections before asserting graceful shutdown; the failing SHA reproduced the timeout under load, and five repeated race runs pass after the fix.
- 2026-09-22 E1 shipped: disposable embedded-build acceptance with deterministic judge/target fixtures, guarded cleanup, CSP/browser-error enforcement, keyboard coverage, and both-theme 360/1440px axe scans.
- 2026-09-20 M7D1 shipped: admin-scoped run-item lookup, non-null score arrays, set-based page enrichment, repeatable-read consistency, generated OpenAPI/TypeScript, and regressions for source deletion, 101-score boundaries, and concurrent completion.

- 2026-09-11 PR #9 passed Docker-backed PostgreSQL integration tests, Go race detection, frontend production checks, and Python checks in CI.
- 2026-09-11 closed `findings.md`: scoped deletion locking, single-source application loading with error presentation, protected one-time keys, blank expected-output normalization, narrowed docs ignore, and populated migration upgrade coverage.

- 2026-09-11 M7C5 and M7C6 merged to `main` in PR #8 (`41e3a6d`). M7C6 shipped eligibility, reference/score/save-to-dataset trace-detail actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.

- Frontend trace filters now drop `passed` without a valid scorer before serializing or requesting.
- Trace pagination tests use distinct IDs, assert filters on both load-more requests, and verify reset results.
