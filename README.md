<p align="center">
  <img src="assets/assay_gopher.png" alt="Assay gopher and wordmark" width="480">
</p>

# See the conversation. Understand the answer.

**Assay is a self-hosted workspace for LLM tracing and evaluation.** Follow multi-turn conversations,
inspect model/tool/context evidence, score answers, and turn failures into regression datasets.
One Go service with an embedded UI, plus Postgres. A typed Python SDK and CLI are included.

[Documentation](docs/index.md) · [Linux / macOS quickstart](docs/quickstart-linux.md) ·
[Windows quickstart](docs/quickstart-powershell.md) · [Python SDK](docs/python-sdk.md)

## Start locally

Use a source checkout; **published Assay container images are not available yet**.
The quickstarts generate private database/encryption credentials, preserve existing files,
and walk through your first trace without a paid model.

In your private `.env`, enable:

```dotenv
ASSAY_LOCAL_MODE=true
```

Keep your existing `ASSAY_POSTGRES_PASSWORD` and `ASSAY_ENCRYPTION_KEY`, or generate them using the
[Linux](docs/quickstart-linux.md#2-create-a-private-local-configuration) or
[PowerShell](docs/quickstart-powershell.md) instructions for a fresh install. Then:

```bash
docker compose up --build -d
```

Open **http://localhost:8080**. Local mode opens the UI directly — **no admin token**.
Create a project, save its one-time ingest key, and create an application.

> **Local means trusted.** Anyone who can reach a local-mode server has management access,
> including deletion. Keep the Compose loopback binding and trust its Docker network.
> Project keys are still required for SDK ingestion. Local mode defaults to **off**;
> normal mode requires `ASSAY_ADMIN_TOKEN`. Database and encryption secrets remain required.
>
> Already have data? Startup applies migrations automatically. Read
> [upgrade and recovery](docs/deployment.md) before running a newer checkout.

## Instrument one answer

Install the published **assay-sdk 0.4.0** for session/local-mode features in your application's
uv project (older 0.3.0 does not include them):

```bash
uv add 'assay-sdk==0.4.0'
```

For development against the source checkout instead:

```bash
uv add --editable /absolute/path/to/assay/clients/python/assay
```

Set `ASSAY_ENDPOINT`, `ASSAY_API_KEY`, and `ASSAY_APPLICATION` in the application's environment.
The application value is its **slug**, not its UUID. Then run:

```python
import assay

assay.init()  # reads endpoint, project key, and application slug from the environment
try:
    with assay.span("answer", scorable=True) as current:
        # Explicit setters intentionally capture these synthetic values.
        current.set_input("What is Assay?")
        current.set_output("A tracing and evaluation workspace.")
    if not assay.flush():
        raise RuntimeError("Assay export did not finish")
finally:
    assay.shutdown()
```

Find the result in **Traces**. Use [`assay.session(...)`](docs/python-sdk.md#sessions-across-multiple-requests)
to group separate request traces into **Sessions**. Content capture is off by default for decorators;
explicit setters export the values you supply. Redact sensitive content before export.

## What you can do

| Workflow | Start here |
|---|---|
| Follow a conversation and inspect its source traces | [Concepts and UI tour](docs/concepts.md) |
| Capture sync/async calls, messages, tools and retrieval context | [Python SDK guide](docs/python-sdk.md) |
| Grade groundedness/correctness and inspect retained evidence | [Evaluation guide](docs/evaluations.md) |
| Automate project setup, exports, comparisons and CI gates | [CLI cookbook](docs/cli.md) |
| Configure local mode, a judge or Docker networking | [Configuration reference](docs/configuration.md) |
| Recover from connection, tracing or scoring problems | [Troubleshooting](docs/troubleshooting.md) |
| Back up, restore, retain data and upgrade safely | [Deployment guide](docs/deployment.md) |

The live API reference is at `/docs`; `/openapi.json` is machine-readable.
The [Python chat example](examples/python-qa/README.md) is optional and uses a real model provider.

## Scope and status

- JSON OTLP/HTTP ingestion, explicit trace/session capture, project-scoped keys, datasets, evaluation
  runs, score evidence, comparisons and trends are implemented in this checkout.
- Binary protobuf, OTLP/gRPC, provider auto-instrumentation, SSO/RBAC and HA deployment are not.
- Local mode is a trusted-machine convenience, not a multi-user security boundary.
- Normal UI authentication stores the admin token in same-origin `localStorage`; **Disconnect**
  removes it. Local mode stores no dummy admin credential and displays its mode visibly.
- New SDK features and container delivery remain subject to release/acceptance gates. Do not
  infer a container release from a Python package version. Final human visual signoff remains open.
- Apache-2.0. Designed for individual developers and small self-hosted teams.

## Published-image deployment

<details>
<summary>Release-only Compose reference (no image published yet)</summary>

After an image is published and anonymously pull-tested, set `ASSAY_IMAGE` to its verified tag or
digest. Use this as `compose.yaml` in a clean deployment directory, not alongside source Compose.
Set a private database password and a stable base64 32-byte encryption key. Token authentication
is the default; `ASSAY_LOCAL_MODE=true` is an explicit opt-in for trusted local use only.

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
      test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -U assay -d assay"]
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
      ASSAY_LOCAL_MODE: ${ASSAY_LOCAL_MODE:-false}
      ASSAY_ADMIN_TOKEN: ${ASSAY_ADMIN_TOKEN:-}
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

With the repository filename: `docker compose -f compose.published.yaml up -d`.
Never use `down -v` unless you deliberately intend to destroy the database.

</details>

## Development

[Architecture](docs/architecture.md) · [Semantic conventions](docs/semantic-conventions.md) ·
[CI/CD and isolated acceptance](docs/ci-cd.md)

Use the quickstart for configuration, then build from source. Keep tests on disposable databases;
never apply experimental migrations to a persistent database without a backup and approval.
