<p align="center">
  <img src="assets/assay_gopher.png" alt="Assay gopher and wordmark" width="480">
</p>

**LLM tracing and evaluation in one Go binary plus Postgres.** Assay accepts JSON OTLP/HTTP traces,
stores them in Postgres, and runs built-in groundedness and correctness evaluators. It includes an
embedded single-user UI, a typed Python SDK, and a CLI.

- Two services: `assayd` (API, worker, embedded UI) and Postgres.
- JSON OTLP/HTTP only. Binary protobuf and OTLP/gRPC are not implemented.
- One admin credential for management and UI; one project key for trace ingestion.
- Apache-2.0. Built for individual developers and small self-hosted teams, not SSO/RBAC/HA estates.

## Start Assay

The published-image path is first, but **no image has been published yet**. Do not invent an image
tag from the Python package version. After a maintainer publishes and anonymously pull-tests a
release, set `ASSAY_IMAGE` to that recorded tag or digest and use the following exact Compose file
in a clean directory as `compose.yaml`:

<!-- BEGIN compose.published.yaml -->
```yaml
# Published Assay deployment. Set every required value in a private .env file first.
# ASSAY_IMAGE must be a verified released GHCR tag or digest; this file never builds source.

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
      ASSAY_DATABASE_URL: postgres://assay:${ASSAY_POSTGRES_PASSWORD:?Set a URL-safe database password}@postgres:5432/assay?sslmode=disable
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
<!-- END compose.published.yaml -->

Then create a private `.env` beside it and run:

```bash
docker compose up --build --force-recreate -d
```

`--build` is harmless because this file has no build stanza. With the checked-in filename, run
`docker compose -f compose.published.yaml up --build --force-recreate -d`. Do not combine it with
`docker-compose.yml`.

For a source checkout, follow the [Linux quickstart](docs/quickstart-linux.md): it creates `.env`
without overwriting existing secrets, builds the image, creates an app/key, sends a real SDK trace,
and optionally runs an evaluation. Windows users have complete
[PowerShell parity](docs/quickstart-powershell.md).

## Environment

| Variable | Required for | Meaning |
|---|---|---|
| `ASSAY_IMAGE` | Published Compose | Verified released tag/digest; no source build |
| `ASSAY_ADMIN_TOKEN` | Server/UI/admin client | Management credential, not tracing ingest key |
| `ASSAY_ENCRYPTION_KEY` | Server | Base64 32 bytes, stable across restarts/upgrades |
| `ASSAY_POSTGRES_PASSWORD` | Published Compose | Generated URL-safe DB password, not a judge key |
| `ASSAY_DATABASE_URL` | Native/server | Compose overrides/wires internal Postgres host |
| `ASSAY_JUDGE_BASE_URL` / `ASSAY_JUDGE_MODEL` | Evaluation | OpenAI-compatible judge; optional for tracing |
| `ASSAY_JUDGE_API_KEY` | Authenticated judge | Provider key; optional for keyless local endpoints |
| `ASSAY_ENDPOINT` | Host SDK/CLI | `http://localhost:8080`; inside app Compose use `http://assayd:8080` |
| `ASSAY_API_KEY` | SDK ingest/project operations | One-time project key created in UI/API |
| `ASSAY_APPLICATION` | SDK | Existing application slug, not UUID/project name |
| `ASSAY_HTTP_PORT` | Compose host | Optional host port, default 8080 |
| `ASSAY_TRACE_RETENTION_DAYS` | Server | 0 keeps spans; >0 expires spans, not all score evidence |

Generate separate token/password values with `openssl rand -hex 32`, and the encryption key with
`openssl rand -base64 32`. Keep the encryption key with the database backup. Tracing works without
judge credentials. The Compose files bind HTTP to loopback and keep Postgres private.

## Python SDK

The distribution is `assay-sdk` and imports as `assay`:

```bash
uv add assay-sdk
```

```python
import assay

assay.init(
    endpoint="http://localhost:8080",
    api_key="asy_...",
    application="support-bot",
    capture=True,
)

with assay.span("answer", scorable=True) as span:
    span.set_input({"question": "What is Assay?"})
    span.set_output("An LLM tracing and evaluation service.")
assay.flush()
assay.shutdown()
```

See the [SDK README](clients/python/assay/README.md) for typed client and CLI workflows. The
[Python Q&A example](examples/python-qa/README.md) is optional and uses a real model provider.

## Operations and status

M6 functionality—score filters/export, trace-to-regression imports, metrics, trends, and optional
span retention—is complete. The current M7 delivery remains in acceptance: structured SDK capture,
run evidence/comparison, and disposable acceptance exist; final UI redesign and E4 browser, visual,
accessibility, and performance approval remain.

The UI is a single-user admin-token test tool. It stores that token in same-origin `localStorage`;
use a trusted browser and origin, and choose **Disconnect** to remove it. Cost/provider
instrumentation, provider auto-instrumentation, session UI, binary protobuf, and gRPC remain
unimplemented.

Read [deployment and recovery guidance](docs/deployment.md) before upgrades or deletion. In
particular, `docker compose down -v` destroys data; deleting an individual dataset item preserves
existing run snapshots, while deleting a dataset cascades its dependent runs.

## Development

```bash
cp .env.example .env
docker compose up --build --force-recreate -d
```

For architecture, semantics, and CI references, see [architecture](docs/architecture.md),
[semantic conventions](docs/semantic-conventions.md), and [CI/CD](docs/ci-cd.md).
