# M7E Delivery, Documentation, and Release Acceptance Implementation Plan

> **For agentic workers:** Use `executing-plans` task-by-task, or the available
> `superpowers:subagent-driven-development` skill. Image publication is a separate approval gate.

**Goal:** Make a verified release easy to install, configure, test, and upgrade on Linux and Windows.
**Architecture:** Extend the existing multi-stage Dockerfile and two-service Compose deployment.
Test-only fake services are isolated from release images; browser acceptance targets embedded assets.
**Tech Stack:** Docker/Compose/BuildKit/GitHub Actions; Playwright/axe; Bash; existing Go/Python tools.
**Spec:** [`../specs/2026-09-08-product-completion-design.md`](../specs/2026-09-08-product-completion-design.md), §§9–10.

## Global Constraints

Apply the [master constraints](2026-09-08-product-completion.md#global-constraints).
Do not publish packages, change GHCR visibility, overwrite root `.env`, expose real captured data,
or delete existing Compose volumes during execution without explicit permission.
**Run E1 early**, after A1/B1, so the browser/security guardrail exists before completing UI milestones
and D6. E2 can run independently; E3 requires working APIs/SDK and E2; final E4 waits for all milestones.

## E1. Add a disposable embedded-build acceptance harness before UI rollout

**Files**
- Create: `web/playwright.config.ts`, `web/e2e/fixtures.ts`, `web/e2e/embedded-smoke.spec.ts`,
  `web/e2e/accessibility.spec.ts`.
- Modify: `web/package.json`, `web/pnpm-lock.yaml`, lint config/script to include new test/config files.
- Create: `tests/acceptance/compose.yaml`, `tests/acceptance/run.sh`,
  `assayd/cmd/acceptance-fixtures/main.go`, `main_test.go`, `tests/acceptance/Dockerfile.fixtures`.
- Modify: `.github/workflows/web.yml`, `.pre-commit-config.yaml` (frontend checks), `.gitignore`
  (test output only, not tests/fixtures).

**Interfaces:** Harness builds a disposable image tag, boots a separate Compose project whose name
begins `assay-acceptance-`, and exports `ASSAY_ACCEPTANCE_ENDPOINT`, `ASSAY_ACCEPTANCE_ADMIN_TOKEN` to
browser/SDK tests. E2 production Compose remains two services; this test-only Compose adds a fake judge/
target. Browser uses host port 18080 by default; no existing port 8080 service is modified.

- [ ] Resolve current stable `@playwright/test` and `@axe-core/playwright` versions from official
  sources, read browser install/config docs, record exact pins and audit results. Reuse existing
  Vitest/MSW for fast tests; browser dependency is justified by real layout, CSP, focus, and image
  integration that jsdom cannot validate. Do not add a second unit-test framework.
- [ ] Implement fixture server with stdlib Go HTTP only: `/healthz`, OpenAI-compatible
  `/v1/chat/completions`, and `/answer`. Reuse synthetic judge response shapes from
  `assayd/internal/scoring/judge_test.go`, `scoring_test.go`, and target tests. Select deterministic
  claim-extraction/verification/correctness response by request prompt shape and known synthetic case,
  not live model calls. Unknown request returns a test-failing 422; never silently pass all inputs.
  Provide explicit synthetic questions yielding pass/fail/transient 503/terminal failure. Unit-test
  each response shape and no accidental call to a public provider.
- [ ] Wire test Postgres with fresh named volume, migrations and assayd healthcheck; assayd's judge
  base is `http://fixtures:18090/v1` and generated target `http://fixtures:18090/answer`.
  Generate throwaway admin/encryption/DB credentials into a temporary file outside root `.env` with
  mode 0600. Refuse to run cleanup unless project prefix matches. Explicit teardown applies only to
  this project's volumes; never `docker compose down -v` without explicit project/file arguments.
- [ ] Shell entry point starts with `set -euo pipefail`, verifies dependencies, builds web+Go image,
  `up --wait` with bounded timeout, executes browser and optional SDK acceptance, then safely tears
  down via trap on success/failure. Capture synthetic logs on failure without environment/headers.
  Define `(cd web && pnpm test:e2e)` to run Playwright against the harness URL; a missing env URL
  fails fast, never defaults to the user's running service.
- [ ] Define Playwright test fixtures `workspace` with `{projectId, applicationId, apiKey}` seeded via
  the actual API using disposable admin token, and `capturedTrace` with `{id, otelTraceId}` seeded
  through OTLP. Fixture IDs and UI selection remain per test; browser storage/token is isolated per
  context. Build on API contracts, not database SQL. Use only synthetic content in screenshots.
- [ ] Add the minimal embedded smoke before feature tests:

```ts
test("serves embedded UI with protected routes and CSP", async ({page}) => {
  const response = await page.goto("/apps");
  expect(response?.headers()["content-security-policy"]).toContain("script-src 'self'");
  await expect(page.getByRole("button", {name: "Connect"})).toBeVisible();
  await expect(page).not.toHaveTitle(/Vite/);
});
```

  Install a `securitypolicyviolation` listener and browser console/pageerror collector in fixtures;
  assert no unexpected violations/errors on every test. Unknown errors are failures, not filtered
  because the screenshot looks right. Deterministically freeze timestamps/fonts for visual fixtures.
- [ ] Add keyboard-only dialog/drawer tests and axe scan at 360/1440px in both themes. Exclude no rule
  without a documented actionable reason. Add web lint/format/types hooks using existing commands;
  include E1 files in CI path filters. Run `shellcheck`, `shfmt -d`, `actionlint`, `zizmor`.
- [ ] Run the disposable harness; verify it refuses an endpoint pointing at the ordinary local app,
  and cleanup leaves the preexisting Compose project untouched. Commit
  `Add isolated embedded UI and workflow acceptance harness`.

**Gate:** This guardrail runs before extending screenshots to B/C/D. It exercises actual bundled
assets and the Go CSP; a Vite-only screenshot is insufficient release evidence.

## E2. Complete container publication and published-image Compose

**Files**
- Modify: `assayd/Dockerfile`, `docker-compose.yml`, `.env.example`.
- Create: `compose.published.yaml`, `.github/workflows/container-publish.yml`.
- Modify: `.github/workflows/web.yml`, `docs/ci-cd.md`.
- Create: `tests/acceptance/container-smoke.sh` (image argument, synthetic temporary stack).

**Interfaces:** Image name `ghcr.io/marioweid/assay`. Root Compose keeps source `build`.
Published Compose requires `ASSAY_IMAGE` as a real verified release tag or digest and has **no** build
context. Existing `ASSAY_ADMIN_TOKEN`, `ASSAY_ENCRYPTION_KEY`, optional judge settings retain semantics.

- [x] Establish current Docker build baseline through existing Dockerfile. Assert index references
  hashed assets, known static icon/font exists, `/readyz` returns 200, deep link returns SPA, API auth
  remains protected, and `/assayd healthcheck` works in the distroless image. Do not add curl/sh to
  runtime merely for health checking; current executable already implements it.
- [x] Add OCI labels for source/revision/version and explicit build args. For arm64, use correct
  BuildKit `BUILDPLATFORM`/`TARGETOS`/`TARGETARCH` stages and Go cross-compilation; UI assets are
  architecture-independent. Preserve nonroot distroless runtime and CA certificates. No secrets in
  build args or layers. Keep pinned base digests unless a separately verified update is required.
- [x] Build publishing workflow on `v*` release tags and manual dispatch requiring a version input.
  Validate version/tag and derive lowercase image name. Separate build/test (read-only permissions)
  from publish (`contents:read`, `packages:write`, attestation/OIDC permissions only if used). Verify
  all new Actions' current release SHAs and record version comments. Checkout persist-credentials=false.
  PRs never log in/push; do not use `pull_request_target` with untrusted code.
- [ ] Publish immutable semantic version and git SHA tags plus SBOM/provenance; `latest` only for
  non-prerelease, verified releases. Attach OCI digest to release output. Use an approval-protected
  environment for first publication. Confirm GitHub package public visibility with maintainer; an
  authenticated CI push is not proof of anonymous pullability. No secret PAT required if GITHUB_TOKEN
  package permissions suffice.
- [x] Create the following complete published Compose starting from the existing Postgres digest:

```yaml
services:
  postgres:
    image: postgres:18.6-trixie@sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280
    environment:
      POSTGRES_USER: assay
      POSTGRES_PASSWORD: ${ASSAY_POSTGRES_PASSWORD:?Set a URL-safe database password}
      POSTGRES_DB: assay
    volumes:
      - assay-pgdata:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U assay -d assay"]
      interval: 5s
      timeout: 5s
      retries: 10
    restart: unless-stopped
  assayd:
    image: ${ASSAY_IMAGE:?Set the verified Assay release image tag or digest}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      ASSAY_HTTP_ADDR: ":8080"
      ASSAY_DATABASE_URL: postgres://assay:${ASSAY_POSTGRES_PASSWORD:?Set a database password}@postgres:5432/assay?sslmode=disable
      ASSAY_ADMIN_TOKEN: ${ASSAY_ADMIN_TOKEN:?Set the admin token}
      ASSAY_ENCRYPTION_KEY: ${ASSAY_ENCRYPTION_KEY:?Set a base64-encoded 32-byte key}
      ASSAY_JUDGE_BASE_URL: ${ASSAY_JUDGE_BASE_URL:-}
      ASSAY_JUDGE_MODEL: ${ASSAY_JUDGE_MODEL:-}
      ASSAY_JUDGE_API_KEY: ${ASSAY_JUDGE_API_KEY:-}
      ASSAY_UI_ENABLED: "true"
      ASSAY_TRACE_RETENTION_DAYS: ${ASSAY_TRACE_RETENTION_DAYS:-0}
    ports:
      - "127.0.0.1:${ASSAY_HTTP_PORT:-8080}:8080"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    healthcheck:
      test: ["CMD", "/assayd", "healthcheck"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped
volumes:
  assay-pgdata:
```

  Fixed internal database username/name keeps this example simple; source Compose retains its existing
  configurable names. The generated password uses hex so DSN interpolation is safe. Do not imply
  arbitrary unescaped password characters are supported in this interpolated URL; use a URL-encoded
  DSN in a separately configured deployment. Add host-gateway to source Compose too for Linux judges.
- [x] Root source Compose keeps `build` and loopback-binds app/Postgres ports; update comment to exact
  required command. Do not replace user's root `.env`. `.env.example` distinguishes source/published
  use, removes working-looking fake cloud credentials from the normal trace-only path, includes
  `ASSAY_APPLICATION` in SDK section, and explains stable encryption key/optional judge values.
- [x] Test missing required env fails `docker compose -f compose.published.yaml config --quiet`;
  tracing-only boot with no judge works; configured fake judge works; Linux host-gateway resolves;
  root build and published no-build files each validate. Keep secrets out of `compose config` stdout.
- [ ] Run image smoke on linux/amd64 and, if advertised, linux/arm64 with native runner or explicit
  emulation smoke. Inspect manifest architectures and runtime readiness independently. After approval,
  anonymously pull exact published digest with an isolated empty Docker config and run the same smoke.
  No unverified “multi-arch” or “published” wording. Commit workflow/config changes separately from the
  maintainer's actual release/tag operation: `Publish reproducible Assay container images`.

**Gate:** A checkout can build locally, and a user with only published Compose plus env values can pull
and start the release. Publication is blocked until permission and final E4 acceptance, even if workflow
code has been merged earlier.

## E3. Write runnable Linux-first docs with PowerShell parity

**Files**
- Modify: `README.md`, `clients/python/assay/README.md`, `examples/python-qa/README.md`,
  `docs/architecture.md`, `docs/ci-cd.md`, `docs/specs/2026-08-26-assay-design.md` status only.
- Create: `docs/quickstart-linux.md`, `docs/quickstart-powershell.md`, `docs/deployment.md`.
- Create: `tests/acceptance/docs-smoke.sh`; executable snippets may also live in
  `examples/quickstart/` to allow direct testing rather than manually retyping docs.

**Interfaces:** README is the concise start page: published-image path first, source-development path
second, SDK minimal example, clear environment table, links for full Linux and PowerShell flows.
Design docs remain historical; only mark a milestone complete after verified delivery.

- [ ] Write/execute shell snippets with `set -euo pipefail` in a temporary quickstart directory and
  `umask 077`. For source checkout the start instructions must include exactly:

```bash
cp .env.example .env
# Edit .env: set ASSAY_ADMIN_TOKEN and ASSAY_ENCRYPTION_KEY; use a fresh database password.
docker compose up --build --force-recreate -d
```

  The guide first checks `.env` does not already exist before copying; never instruct overwriting
  working secrets. Explain `--force-recreate` recreates containers, not named data volumes.
- [ ] Give Linux credential generation with existing openssl, not a hardcoded all-zero encryption key:

```bash
openssl rand -hex 32     # admin token or URL-safe Postgres password; generate separately
openssl rand -base64 32  # encryption key; save once and retain with the database backup
```

  Store each generated value in the fresh `.env`, not shell history through an expanded inline token.
  Explain `ASSAY_IMAGE` is set to the exact release/digest verified in E2/E4. Before release, keep this
  section clearly unreleased; at publication replace docs' selected tag with the actual pull-tested
  value, not a version inferred from the Python package.
- [ ] Embed the actual `compose.published.yaml` contents in README, not an incomplete sketch. Document
  filename choices explicitly:

```bash
# If copied to compose.yaml in a clean directory:
docker compose up --build --force-recreate -d
# If using the repository's explicit published configuration:
docker compose -f compose.published.yaml up --build --force-recreate -d
```

  `--build` is intentionally harmless for the published file because it has no build stanza. Do not
  combine both Compose files accidentally. Make README snippet drift a docs-smoke failure.
- [ ] Environment table must include these exact distinctions:

| Variable | Required for | Meaning |
|---|---|---|
| ASSAY_IMAGE | Published Compose | Verified released tag/digest; no source build |
| ASSAY_ADMIN_TOKEN | Server/UI/admin client | Management credential, not tracing ingest key |
| ASSAY_ENCRYPTION_KEY | Server | Base64 32 bytes, stable across restarts/upgrades |
| ASSAY_POSTGRES_PASSWORD | Published Compose | Generated URL-safe DB password, not a judge key |
| ASSAY_DATABASE_URL | Native/server | Compose overrides/wires internal Postgres host |
| ASSAY_JUDGE_BASE_URL / ASSAY_JUDGE_MODEL | Evaluation | OpenAI-compatible judge; optional for tracing |
| ASSAY_JUDGE_API_KEY | Authenticated judge | Provider key; optional for keyless local endpoints |
| ASSAY_ENDPOINT | Host SDK/CLI | `http://localhost:8080`; inside app Compose use `http://assayd:8080` |
| ASSAY_API_KEY | SDK ingest/project operations | One-time project key created in UI/API |
| ASSAY_APPLICATION | SDK | Existing application slug, not UUID/project name |
| ASSAY_HTTP_PORT | Compose host | Optional host port, default 8080 |
| ASSAY_TRACE_RETENTION_DAYS | Server | 0 keeps spans; >0 expires spans, not all score evidence |

- [ ] Add full Linux workflow: create project/key/app via UI, equivalent curl+jq or typed Python
  bootstrap for agents, emit a real SDK trace, flush/shutdown, read by OTel ID, add dataset/run and
  inspect results. Use dynamic IDs/timestamps, not the old fixed OTLP IDs/dates. Show both trace-only
  (no judge secrets) and fake/local/configured judge evaluation path. Document JSON-only OTLP honestly.
- [ ] Keep full PowerShell equivalent in linked guide with `$env:` usage and native secure random/key
  generation, not Bash-only command substitutions. No bash `source .env` instruction for PowerShell.
  Linux guide explains host/container localhost distinction, host-gateway, and `sslmode=disable` only
  for trusted local container networking; network deployments require reverse-proxy TLS.
- [ ] Deployment guide: named volume persistence; backup with `pg_dump`; restore into a **new** database
  and verify; preserve encryption key; pull immutable release; restart applies migrations; backup
  before A3 upgrade; restore backup for rollback when down-migration cannot restore deleted source
  items. No claim that swapping an old binary safely downgrades schema. `down -v` is destructive.
- [ ] Resolve documentation contradictions: M6 is complete at baseline; M7 status depends on E4;
  browser is still single-user admin-token auth; cost/provider instrumentation/protobuf/session UI
  remain unimplemented; dataset item deletion vs whole-dataset cascade is explicit. No changing the
  old approved M5.5 scope to pretend it already promised this release.
- [ ] Run docs-smoke against E1 disposable services using current checkout SDK, shellcheck/shfmt,
  Markdown link check and README-vs-Compose snippet comparison. Test PowerShell snippets on a Windows
  runner if claiming them verified. Commit `Document Linux and published-image quickstarts`.

**Gate:** Two supported setup paths are unmistakable, credentials and URLs have clear roles, and
examples do not depend on PowerShell or unpublished package functions by accident.

## E4. Run end-to-end polish gates and perform an approved release

**Scheduling update (2026-09-22):** Defer this work until E2–E3 are complete and the user approves
the final UI redesign. Do not add broad browser workflow coverage or visual baselines while the UI
is still being reshaped; retain E1's existing smoke/security coverage in the interim.

**Files**
- Create: `web/e2e/product-workflow.spec.ts`, `web/e2e/trace-workbench.spec.ts`,
  `web/e2e/evaluation-review.spec.ts`, `web/e2e/visual.spec.ts` plus synthetic screenshot baselines.
- Extend: `tests/acceptance/run.sh`, `container-smoke.sh`, `docs-smoke.sh`.
- Modify: workflows to run focused changed-area tests and release acceptance; SDK version/release
  files only when an actual SDK release is being prepared.
- Create: `docs/releases/m7-acceptance.md` with factual results, environment and limitations.

**Consumes:** E1 harness, all A–D workflows, E2 delivery files and E3 docs. Release publication requires
explicit maintainer authorization even after all tests pass.

- [ ] Add real embedded-browser product flow: Connect → create project/app/key → emit synthetic trace
  from D4 SDK → find through filters → read messages/tools/context and select overlapping spans →
  attach reference → score → save to dataset → edit case → run evaluation → inspect failed case →
  rerun current data → compare. Verify IDs and persisted state through API as well as rendered text.
- [ ] Add error/accessibility coverage: missing/expired token, disconnected network, missing capture,
  malformed messages, retention without spans, empty dataset, 409 duplicate/import, failed target and
  judge, pending cancel, deleted selected resource, dirty forms, copy failure, keyboard navigation,
  360px viewport/200% zoom, long IDs/100KiB payloads, reduced motion, two themes, deep-link reload.
- [ ] Capture synthetic visual baselines for shell, applications, trace conversation/waterfall, dataset
  editor, run items/comparison and empty/error states. Freeze clock/font/network fixtures. Human review
  must approve hierarchy/spacing/readability; do not auto-accept screenshots because tests generated
  new images. Dark theme should be recognizably Assay and align with the reviewed Odysseus inspiration.
- [ ] Measure the spec's large fixtures on a recorded machine/browser: 1,000 spans/50 model calls,
  100KiB content, 1,000 dataset cases, 10,000 trace summaries. Record selection p95 and long tasks,
  page response sizes and query plans. Fix measured regressions (windowing/query indexes if required),
  not speculative throughput claims. Target p95 selection <100ms and no >200ms selection task after
  initial parsing. Test content retrieval stays paginated, not one unbounded comparison fetch.
- [ ] Security gate: no raw capture executes as HTML, no remote media requests, no CSP violations,
  no credentials in screenshots/storage/URLs/logs except the documented admin-token localStorage,
  project isolation intact, malicious imported text harmless. Review API-key and target secret paths
  separately from presentation; trace export is explicit and potentially sensitive.
- [ ] Run all **affected** cross-layer suites, full frontend lint/format/types/build, sqlc/OpenAPI drift,
  relevant backend race/integration tests, SDK ruff/ty/package tests and D6 acceptance, E2 container
  smoke, E3 docs smoke, actionlint/zizmor/shellcheck/shfmt and prek. List executed and skipped checks in
  acceptance record; skipped mandatory tests block release rather than count as passes.
- [ ] Prepare SDK release only when required for D4/D5 examples: bump its actual version consistently
  in package metadata, `__init__`, instrumentation scope/user agent, fixtures and chat example pin;
  run test_package.py and existing PyPI publishing workflow after approval. Do not claim the current
  `assay-sdk==0.3.0` already contains new helpers. Verify the installed published wheel in a fresh uv
  environment after publication. Source-checkout docs may use checkout methods before release if
  explicitly labeled.
- [ ] After maintainer approval/tag, publish image through E2 workflow; record digest/architectures/
  SBOM/provenance and anonymous pull smoke. Set README's concrete release reference to the verified
  artifact. Mark M7 complete in current README/roadmap only after final human acceptance, not merely
  because tasks have code. Commit `Record verified M7 product acceptance`.

**Final release checklist**

- [ ] All nine improvements have a demonstrated result; lifecycle exceptions are explicit.
- [ ] No app operations depend on undocumented curl or unreleased SDK helpers.
- [ ] Conversation/waterfall and evaluation review are the primary workflows, not JSON-only viewers.
- [ ] Item edits/deletion cannot rewrite run history; permanent cascades are confirmed.
- [ ] SDK stays low-configuration, opt-in, typed, and honest about unsupported instrumentation.
- [ ] Published image is anonymously pullable and boots with the documented Compose/env values.
- [ ] Linux guide, PowerShell guide, source build, and image install have recorded verification.
- [ ] No runtime topology/auth/product scope expansion was smuggled in as polish.
