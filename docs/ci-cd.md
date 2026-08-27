# Assay — CI/CD Plan

*Design for `.github/workflows/`. Verified pins as of 2026-08-26. Companion to `docs/specs/2026-08-26-assay-design.md`. This is a plan, not yet implemented.*

## Philosophy

Verify at every level, as a gate, not an afterthought: compiler → linters → type checkers → tests → security scan. A red check blocks merge. Actions are **SHA-pinned** with version comments; workflows run with least privilege and `persist-credentials: false`.

## Pipelines

Three build surfaces (Go backend, React web, Python client) + an image publish. Each is a separate workflow triggered on PRs touching its paths, plus `main`.

| Workflow | Triggers | Gate |
|---|---|---|
| `backend.yml` | `assayd/**`, `.github/workflows/backend.yml` | build · golangci-lint v2 · `go test` (unit + testcontainers) · sqlc-drift · goose validate |
| `web.yml` | `web/**` | install · lint · `tsc --noEmit` · vitest · `vite build` · OpenAPI-client drift |
| `python.yml` | `clients/python/assay/**` | `ruff check` · `ruff format --check` · `ty check` · `pytest` · `pip-audit` |
| `image.yml` | push to `main`, tags `v*` | multi-stage build (web → embed → go) · push to GHCR |
| `security.yml` | PRs, weekly cron | `actionlint` · `zizmor` (workflow audit) |

Notes:
- **testcontainers** needs Docker; GitHub-hosted runners provide it, so the Go integration suite (real Postgres, `SKIP LOCKED`, migrations) runs in CI unchanged.
- **sqlc-drift**: run `sqlc generate` (or `sqlc diff`) and fail if the tree changes — generated code must be committed and current.
- **OpenAPI-client drift**: regenerate the web API client from `assayd`'s emitted OpenAPI and fail on diff, so the frontend contract can't silently rot.

## Verified pins (2026-08-26)

| Item | Pin |
|---|---|
| Go | `1.27.0` |
| Postgres (tests/compose) | `postgres:18.6-trixie` (volume at `/var/lib/postgresql`) |
| Node.js | `24` (Active LTS) → `node:24-trixie-slim` |
| pnpm | `11.23.0` |
| uv | `0.12.6` |
| golangci-lint | `v2.13.1` (config format v2) |
| testcontainers-go | `v0.44.0` |
| Vite (bundler) | `8` (scaffolder `create-vite@9`) |
| Tailwind CSS | `4.3.x` (`@tailwindcss/vite`) |
| shadcn CLI | `shadcn@latest` (v4; package renamed from `shadcn-ui`) |

**SHA-pinned actions** (dereferenced release commits — re-verify before merge):
```
actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1        # v7.0.1
actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e        # v7.0.0
actions/setup-node@820762786026740c76f36085b0efc47a31fe5020      # v7.0.0
astral-sh/setup-uv@20cfd1bf945f4377ade1205e4dbc17946fc9a30d      # v10.0.1
docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e   # v4.3.0
docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a     # v7.3.0
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

## `web.yml` (sketch)

```yaml
name: web
on:
  pull_request: { paths: ["web/**", ".github/workflows/web.yml"] }
  push: { branches: [main], paths: ["web/**"] }
permissions: { contents: read }
jobs:
  build:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: web } }
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1   # v7.0.1
        with: { persist-credentials: false }
      - uses: actions/setup-node@820762786026740c76f36085b0efc47a31fe5020   # v7.0.0
        with: { node-version: "24" }
      - run: corepack enable && corepack prepare pnpm@11.23.0 --activate
      - run: pnpm install --frozen-lockfile
      - run: pnpm lint            # oxlint
      - run: pnpm exec tsc --noEmit
      - run: pnpm test            # vitest
      - run: pnpm build           # vite build → dist/ (embedded by the Go image build)
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

## `image.yml` (sketch — multi-stage: web build → embed → go build)

```yaml
name: image
on:
  push:
    branches: [main]
    tags: ["v*"]
permissions:
  contents: read
  packages: write            # GHCR push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1   # v7.0.1
        with: { persist-credentials: false }
      - uses: docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e   # v4.3.0
      - uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f         # v4.6.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a     # v7.3.0
        with:
          context: .                       # repo root: Dockerfile needs web/ + assayd/
          file: assayd/Dockerfile
          push: true
          tags: ghcr.io/marioweid/assay:latest
```

## `assayd/Dockerfile` (multi-stage shape)

```dockerfile
# 1) build the embedded SPA
FROM node:24-trixie-slim AS web
WORKDIR /web
RUN corepack enable && corepack prepare pnpm@11.23.0 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build                                   # -> /web/dist

# 2) build the Go binary with the SPA embedded
FROM golang:1.27.0-trixie AS go
WORKDIR /src
COPY assayd/go.mod assayd/go.sum ./assayd/
RUN cd assayd && go mod download
COPY assayd/ ./assayd/
COPY --from=web /web/dist ./assayd/internal/ui/dist
RUN cd assayd && CGO_ENABLED=0 go build -o /assayd ./cmd/assayd

# 3) minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /assayd /assayd
EXPOSE 8080
ENTRYPOINT ["/assayd"]
```

## Supply chain & hygiene

- **Dependabot** (`.github/dependabot.yml`): ecosystems `gomod` (assayd), `npm` (web, pnpm-aware), `pip`/`uv` (python client), and `github-actions`. Grouped updates, **7-day cooldown**.
- **pnpm hardening:** `minimumReleaseAge 1440` (24h publish delay), `ignore-scripts true` (block postinstall), exact-pinned versions (no `^`/`~`).
- **Python:** pinned `==` versions, `pip-audit` in CI, `uv.lock` committed.
- **Go:** `go.sum` committed; consider `govulncheck` as an added job.
- **Workflow audit:** `actionlint` + `zizmor` in `security.yml`; keep `permissions:` minimal per job; `persist-credentials: false` everywhere.
