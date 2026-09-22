# Assay — CI/CD Plan

*Design and implementation reference for `.github/workflows/`. Verified pins as of 2026-09-22.
The backend, frontend, Python package, and container publishing workflows are implemented; the
scheduled security workflow remains planned. Companion to
`docs/specs/2026-08-26-assay-design.md`.*

## Philosophy

Verify at every level, as a gate, not an afterthought: compiler → linters → type checkers → tests → security scan. A red check blocks merge. Actions are **SHA-pinned** with version comments; workflows run with least privilege and `persist-credentials: false`.

Tests protect behavior, boundaries, error paths, and regressions rather than internal call sequences. CI has **no line-coverage percentage gate**: coverage may be inspected to find suspicious gaps, but it is not a quality score or a reason to add low-value tests. Mock external boundaries such as HTTP and judge providers; use real Postgres for database and queue semantics.

## Pipelines

Three build surfaces (Go backend, React web, Python client) plus package and image publishing.
Validation workflows run for relevant pull requests and `main`; publishing uses dedicated tags.

| Workflow | Triggers | Gate |
|---|---|---|
| `backend.yml` | `assayd/**`, `.github/workflows/backend.yml` | build · golangci-lint v2 · `go test` (unit + testcontainers) · sqlc-drift · goose validate |
| `web.yml` | frontend, UI-serving, and OpenAPI-affecting files | generation drift · frontend gates · production assets · embedded Go tests/build · release image |
| `python.yml` | `clients/python/assay/**` | `ruff check` · `ruff format --check` · `ty check` · `pytest` · `pip-audit` |
| `python-publish.yml` | tags `python-v*` | version/tag match · test · build · isolated wheel smoke test · PyPI Trusted Publishing |
| `container-publish.yml` | `v*` tags, manual version | amd64/arm64 build + smoke · SBOM/provenance · protected GHCR publish · anonymous pull smoke |
| `security.yml` | PRs, weekly cron | `actionlint` · `zizmor` (workflow audit) |

Notes:
- **testcontainers** needs Docker; GitHub-hosted runners provide it, so the Go integration suite (real Postgres, `SKIP LOCKED`, migrations) runs in CI unchanged.
- **sqlc-drift**: run `sqlc generate` (or `sqlc diff`) and fail if the tree changes — generated code must be committed and current.
- **OpenAPI-client drift**: regenerate the web API client from `assayd`'s emitted OpenAPI and fail on diff, so the frontend contract can't silently rot.

## Verified pins (2026-08-27)

| Item | Pin |
|---|---|
| Go | `1.27.0` |
| Postgres (tests/compose) | `postgres:18.6-trixie` (volume at `/var/lib/postgresql`) |
| Node.js | `22.22.0` → `node:22.22.0-trixie-slim` |
| pnpm | `11.25.0` |
| uv | `0.12.6` |
| golangci-lint | `v2.13.1` (config format v2) |
| testcontainers-go | `v0.44.0` |
| Vite (bundler) | `8` (scaffolder `create-vite@9`) |
| Tailwind CSS | `4.3.x` (`@tailwindcss/vite`) |
| shadcn CLI | v4 (scaffolding only; not installed by the frontend build) |

**SHA-pinned actions** (dereferenced release commits — re-verify before merge):
```
actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1        # v7.0.1
actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e        # v7.0.0
actions/setup-node@820762786026740c76f36085b0efc47a31fe5020      # v7.0.0
astral-sh/setup-uv@20cfd1bf945f4377ade1205e4dbc17946fc9a30d      # v10.0.1
actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0 # v7.0.1
actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1
pypa/gh-action-pypi-publish@dc37677b2e1c63e2034f94d8a5b11f265b73ba33 # v1.14.2
docker/setup-qemu-action@c7c53464625b32c7a7e944ae62b3e17d2b600130     # v3
docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069   # v4.4.1
docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc     # v7.4.0
docker/login-action@dbcb813823bdd20940b903addbd779551569679f         # v4.6.0
golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a  # v9.3.0
```

## `backend.yml` (reference implementation)

```yaml
name: backend
on:
  pull_request:
    paths: ["assayd/**", ".github/workflows/backend.yml"]
  push:
    branches: [main]
    paths: ["assayd/**"]
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: assayd
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1   # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e   # v7.0.0
        with:
          go-version: "1.27.0"
          cache-dependency-path: assayd/go.sum
      - name: Verify sqlc output is current
        run: |
          go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
          git diff --exit-code
      - name: Build
        run: go build ./...
      - name: Lint
        uses: golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a   # v9.3.0
        with:
          version: v2.13.1
          working-directory: assayd
      - name: Test (unit + integration; testcontainers uses the runner's Docker)
        run: go test -race -count=1 ./...
```

## `web.yml`

```yaml
name: web
on:
  pull_request:
    paths:
      - "web/**"
      - "assayd/cmd/openapi/**"
      - "assayd/internal/api/**"
      - "assayd/internal/ui/**"
      - "assayd/Dockerfile"
      - "assayd/go.mod"
      - "assayd/go.sum"
      - ".dockerignore"
      - ".env.example"
      - "docker-compose.yml"
      - ".github/workflows/web.yml"
  push:
    branches: [main]
    paths:
      - "web/**"
      - "assayd/cmd/openapi/**"
      - "assayd/internal/api/**"
      - "assayd/internal/ui/**"
      - "assayd/Dockerfile"
      - "assayd/go.mod"
      - "assayd/go.sum"
      - ".dockerignore"
      - ".env.example"
      - "docker-compose.yml"
      - ".github/workflows/web.yml"
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1   # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: "1.27.0"
          cache-dependency-path: assayd/go.sum
      - uses: actions/setup-node@820762786026740c76f36085b0efc47a31fe5020   # v7.0.0
        with:
          node-version: "22.22.0"
      - run: corepack enable
      - working-directory: web
        run: pnpm install --frozen-lockfile
      - working-directory: web
        run: |
          pnpm generate:api
          git diff --exit-code -- openapi.json src/api/generated
      - working-directory: web
        run: pnpm lint
      - working-directory: web
        run: pnpm format:check
      - working-directory: web
        run: pnpm typecheck
      - working-directory: web
        run: pnpm test
      - working-directory: web
        run: pnpm build
      - working-directory: assayd
        run: |
          rg --fixed-strings 'src="/assets/' internal/ui/dist/index.html
          test -f internal/ui/dist/assay-icon.png
      - working-directory: assayd
        run: go test ./internal/ui ./internal/app
      - working-directory: assayd
        run: go build ./cmd/assayd
      - run: cp .env.example .env
      - run: docker compose config --quiet
      - run: docker build --file assayd/Dockerfile --tag assay-web-ci .
```

## `python.yml` (sketch)

```yaml
name: python
on:
  pull_request: { paths: ["clients/python/assay/**", ".github/workflows/python.yml"] }
  push: { branches: [main], paths: ["clients/python/assay/**"] }
permissions: { contents: read }
jobs:
  test:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: clients/python/assay } }
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1   # v7.0.1
        with: { persist-credentials: false }
      - uses: astral-sh/setup-uv@20cfd1bf945f4377ade1205e4dbc17946fc9a30d   # v10.0.1
        with: { version: "0.12.6" }
      - run: uv sync --frozen
      - run: uv run ruff check .
      - run: uv run ruff format --check .
      - run: uv run ty check
      - run: uv run pytest -q
      - run: uvx pip-audit
```

## `python-publish.yml` (implemented)

The Python distribution is `assay-sdk`; its import namespace remains `assay`. Publishing is
triggered by a `python-v<version>` tag and fails before upload when the tag does not match
`project.version` in `clients/python/assay/pyproject.toml`. The build job has read-only access.
Only the separate publish job receives `id-token: write`, and it publishes through the protected
`pypi` GitHub environment without a long-lived PyPI token.

Before the first publish, create a pending Trusted Publisher at
<https://pypi.org/manage/account/publishing/> with:

| Field | Value |
|---|---|
| PyPI project name | `assay-sdk` |
| GitHub owner | `marioweid` |
| Repository | `assay` |
| Workflow | `python-publish.yml` |
| Environment | `pypi` |

Create a GitHub environment named `pypi`, protect it as appropriate, merge the package and
workflow, then publish the current package version:

```bash
git tag python-v0.2.0
git push origin python-v0.2.0
```

The workflow builds the sdist and wheel, smoke-tests the wheel in an isolated uv environment,
and publishes both files to PyPI. Confirm the release with `uv add assay-sdk`.

## `container-publish.yml`

`v<semver>` tags and manual dispatches with a validated `version` input first build and smoke both
linux/amd64 and linux/arm64 images. Only the subsequent `ghcr` environment-protected job receives
`packages: write`: it publishes `ghcr.io/marioweid/assay:<version>` and `sha-<commit>`, adds `latest`
only for non-prereleases, and attaches BuildKit SBOM/provenance. It then checks the manifest and pulls
the digest through an empty Docker configuration before rerunning the smoke test. The GHCR package
must be made public by a maintainer before that anonymous pull can pass.

`tests/acceptance/docs-smoke.sh` verifies that README embeds the exact published Compose file, checks
local Markdown links, validates Linux credential generation, and sends the quickstart SDK trace through
the E1 disposable fixture stack. `tests/acceptance/run.sh` invokes it before the SDK lifecycle
acceptance. PowerShell syntax requires a Windows runner before it can be recorded as executed.

## `assayd/Dockerfile` (multi-stage shape)

```dockerfile
ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown

# Build the architecture-independent SPA on the build platform.
FROM --platform=$BUILDPLATFORM node:22.22.0-trixie-slim@sha256:465a8c8f0f4103861bcbcf3e512608394b7155eccb1955425f4ea3f672ddc53e AS web-build
# … install web dependencies and run pnpm build

# Cross-compile the Go binary for BuildKit's target platform.
FROM --platform=$BUILDPLATFORM golang:1.27.0-trixie@sha256:ae28539d2ef595b9a2930dd7f031d9592376829dc0eae7cb869559f7d5812c3a AS build
ARG TARGETOS
ARG TARGETARCH
# … copy the SPA into internal/ui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/assayd ./cmd/assayd

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG VERSION
ARG REVISION
ARG CREATED
LABEL org.opencontainers.image.source="https://github.com/marioweid/assay" \
      org.opencontainers.image.revision=$REVISION \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.created=$CREATED
COPY --from=build /out/assayd /assayd
EXPOSE 8080
ENTRYPOINT ["/assayd"]
```

## Supply chain & hygiene

- **Planned Dependabot:** add `gomod`, pnpm-aware `npm`, `uv`, and `github-actions` updates with
  groups and a 7-day cooldown in `.github/dependabot.yml`.
- **pnpm hardening:** `minimumReleaseAge 1440` (24h publish delay), `ignore-scripts true` (block postinstall), exact-pinned versions (no `^`/`~`).
- **Python:** pinned `==` versions, `pip-audit` in CI, `uv.lock` committed.
- **Go:** `go.sum` committed; consider `govulncheck` as an added job.
- **Workflow audit:** `actionlint` + `zizmor` in `security.yml`; keep `permissions:` minimal per job; `persist-credentials: false` everywhere.
