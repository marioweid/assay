# Local mode and onboarding

## Decision

The user selected admin-token-free local mode, **not** keyless ingestion.
`ASSAY_LOCAL_MODE=true` explicitly enables unauthenticated management and UI access.
The default remains token authentication. Project keys remain required for ingestion,
and explicitly supplied project credentials retain their scope rather than becoming admin.
Database passwords and the stable encryption key are still required.

Local mode is for a trusted machine/Docker network, not a multi-user or public deployment.
Compose stays bound to loopback. Host validation (loopback names/IPs plus the checked-in
Compose service name `assayd`) and same-origin browser checks mitigate DNS rebinding and
cross-site requests; they are not authentication against a client that can reach the server.
Forwarded headers do not grant access. Do not use local mode behind a public reverse proxy.

The UI discovers the server mode from a public, non-secret server-info endpoint, opens
without storing a dummy token, and visibly labels local mode. Discovery errors fail closed
with retry. The Python management client/CLI supports explicit local mode, including the
same environment flag; tracing still requires a project key. Server-side configuration is
always authoritative: a client cannot enable local mode on a token-protected server.

## Acceptance

- Default/off/malformed flag and missing-token configuration tests.
- Local admin reads/writes without a token; token mode remains protected.
- Host/origin/fetch-metadata rejection, including DNS rebinding and cross-origin reads.
- Ingestion and project-key scope remain enforced in local mode.
- UI discovery, no-token boot, visible mode indicator, failures/retry and token-mode regressions.
- SDK/CLI local management works without credentials; trace ingestion still rejects missing keys.
- Source/published Compose configuration and current generated API client remain consistent.
- Clear source-first docs hub, Linux/PowerShell local setup, SDK tracing/session/evaluation
  examples, CLI/configuration references, troubleshooting and recovery. No claimed image/SDK release.
- Run focused Go/web/Python checks, browser checks and docs checks. Independent security review
  is required; preserve any unavailable evidence rather than treating a skipped check as a pass.

## Safety and rollout

Existing Sessions/UI work and persistent volumes are retained. Docker was stopped at task start.
No persistent startup/migration or publication is authorized by this implementation task.
Database-backed acceptance needs an isolated disposable stack; request approval before any
Docker startup that might auto-resume existing persistent containers. Ask separately before
starting the user's persistent application after validation.

## Local implementation and evidence

Implemented the server flag/guard/discovery, scope-preserving request authentication, token-free UI
with retry and mode indicator, Python client/CLI support, Compose wiring and source-first docs hub.
SDK setup snippets now use the Assay origin rather than the application's evaluation target URL.
A rejected-token regression was reproduced and fixed so login failures show an actionable error.

Checks completed without Docker:

- 191 Vitest tests; 10 synthetic Playwright/axe checks, including local mode at 320/1440px.
- Frontend typecheck, lint, format and production build (output outside the repository).
- 125 focused SDK/client/CLI/tracing/session tests; Ruff and ty.
- Go config/auth/HTTP-server suites, local/default-auth/OpenAPI API checks and focused `go vet`.
- Generated OpenAPI parity; source/published Compose parse in both auth modes, no services started.
- Docs links/embedded Compose parity and 46 Python/JSON/CLI examples validated statically.
- Six SDK guide tracing examples executed against an in-memory exporter, with session assertions.
- PowerShell blocks parsed; documented secret-generation/ACL block executed in a disposable folder.

Independent read-only security review found no additional concrete auth, ingestion, scope, guard or
UI lifecycle regressions. Its one low-severity documentation finding is fixed: project keys remain
attached to trace/session operations, while a local-mode Client's management operations use its
admin token or anonymous access, not its project key. A regression test records that distinction.

## Disposable live acceptance (user approved Docker startup)

- `CI=1 go test -p 1 ./... -count=1` passed with testcontainer PostgreSQL; the full-app local-mode
  test exercised startup, anonymous project creation/read, malicious Host/Origin rejection,
  explicit project-key rejection on management and keyless ingestion rejection.
- Normal token-mode `tests/acceptance/run.sh` passed 10 embedded real-browser checks, docs bootstrap
  and trace workflow, SDK product acceptance and demo chat live TestClient flow. Its initial run
  found an ambiguous Sessions locator matching hidden raw JSON rather than visible chat evidence;
  the selector was narrowed, then the whole guarded stack passed on rerun.
- New `tests/acceptance/run-local.sh` passed on an isolated override stack: two live Python tests
  covering anonymous management and CLI, keyed SDK session/trace export, cross-project isolation,
  keyless/bad-key ingestion rejection, anonymous deletion, and Host/Origin/fetch-metadata guards.
  One real embedded browser/axe test passed with no stored admin token and anonymous project writes.
- 191 Vitest, 10 synthetic Playwright/axe, 248 Python tests (four opt-in skips), all Go tests,
  frontend lint/format/typecheck/build, Python Ruff/ty, static docs and Compose mode parsing pass.

Both acceptance stacks and only their prefixed containers/volumes/images/temp credentials were
removed; Docker volumes before/after match exactly, and no Assay containers remain. Rancher Desktop
was started for approved tests but its daemon does not remain available across tool commands here.
No persistent services/data/migrations were modified or deployed. Existing Sessions deployed-chat,
manual real-data accessibility/visual, representative migration runtime/WAL and release gates remain.
Bash syntax checks pass; shellcheck/shfmt were not available on this host.
