# First run: Linux and macOS

[Documentation](index.md) / Getting started · [Windows instructions](quickstart-powershell.md)

**Goal:** open Assay, create a workspace, and see a real SDK trace. No paid model or judge is needed.

## 1. Prerequisites

Install Git, Docker with Compose, OpenSSL, and [uv](https://docs.astral.sh/uv/).
Start Docker, then work from a fresh source checkout:

```bash
git clone https://github.com/marioweid/assay.git
cd assay
```

Use the checkout containing local-mode support. There is no published Assay container image yet;
SDK local-mode/session features are in published `assay-sdk` 0.4.0 or this checkout (not 0.3.0).
If you already have a database, read [upgrade and rollback](deployment.md#upgrade-and-rollback)
before starting a newer checkout: server startup automatically applies migrations.

## 2. Create a private local configuration

This creates `.env` **only if it does not already exist**. It generates the database password and
stable encryption key directly into the file, without displaying either:

```bash
(
  set -euo pipefail
  umask 077
  set -o noclobber
  {
    printf 'ASSAY_LOCAL_MODE=true\n'
    printf 'ASSAY_POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"
    printf 'ASSAY_ENCRYPTION_KEY=%s\n' "$(openssl rand -base64 32)"
  } > .env
)
```

Already have `.env`? **Do not regenerate the encryption key or database password.** Add/change only
`ASSAY_LOCAL_MODE=true` in your own trusted configuration. Changing a Postgres environment password
does not change the password inside an initialized database. Keep the encryption key with backups.

Local mode skips the admin token, not the project ingest key. It is unsafe on a public/network-facing
instance. Keep the checked-in `127.0.0.1` port bindings. See [configuration](configuration.md).

## 3. Start and open Assay

```bash
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/readyz
curl --fail http://localhost:8080/v1/server-info
```

Open **<http://localhost:8080/>**. The UI opens directly and displays **Local mode**;
`/v1/server-info` reports `"local_mode": true`. No admin token or judge credential is needed.
A cold image build takes longer than a restart.

If the UI still asks for a token, check [local-mode troubleshooting](troubleshooting.md).
To stop later, use `docker compose down` — **without `-v`**, which would delete your data.

## 4. Create a project, key and application

Choose either path:

- **UI:** Projects → New project → create an API key → save its one-time value →
  Applications → New application, selecting that project. The application slug is the SDK name.
- **CLI/SDK bootstrap:** the following creates a new isolated project/application and saves their
  IDs and one-time key in a private workspace file. It refuses to overwrite that file.

```bash
export ASSAY_ENDPOINT=http://localhost:8080
export ASSAY_LOCAL_MODE=true
uv run --project clients/python/assay python examples/quickstart/bootstrap.py \
  --output .quickstart-workspace.json
```

`ASSAY_LOCAL_MODE` must be exported for host-side CLI/client commands: Docker's `.env` is not
implicitly loaded by Python. The workspace file contains a plaintext project key; never commit,
share, or paste it. On Unix it is created with mode `0600`.

## 5. Send and find a trace

```bash
TRACE_ID=$(uv run --project clients/python/assay python examples/quickstart/trace.py \
  --workspace .quickstart-workspace.json)
printf 'OpenTelemetry trace ID: %s\n' "$TRACE_ID"
```

Open the new application → **Traces**, then search for that ID. Open the trace to inspect its
captured question, answer, retrieval context and timing. The example emits synthetic content,
checks `flush()`, and shuts down cleanly.

For CLI inspection, load the key without printing it:

```bash
ASSAY_API_KEY=$(uv run --project clients/python/assay python -c \
  'import json; print(json.load(open(".quickstart-workspace.json"))["api_key"])')
APP_ID=$(uv run --project clients/python/assay python -c \
  'import json; print(json.load(open(".quickstart-workspace.json"))["application_id"])')
export ASSAY_API_KEY
uv run --project clients/python/assay assay traces list "$APP_ID" --query "$TRACE_ID"
```

A trace's OpenTelemetry hex ID differs from Assay's database UUID. The UI can search by either;
management calls such as `traces.get()` use the Assay UUID. See [concepts](concepts.md).

## 6. Add your application

Follow the [Python SDK guide](python-sdk.md). Start with an explicit span around one answer;
add `assay.session(...)` when multiple requests belong to one conversation. The quickstart trace
above deliberately remains an ordinary, untagged trace.

Ready to score answers? Continue with [your first evaluation](evaluations.md).
Tracing works without a judge; evaluation needs a configured OpenAI-compatible judge.

## Use token authentication instead

For a fresh token-protected setup, set `ASSAY_LOCAL_MODE=false` and generate a separate
`ASSAY_ADMIN_TOKEN` with `openssl rand -hex 32`, storing it privately in `.env`. Recreate the server
with `docker compose up -d`, then reload the UI and connect with that token. The host-side CLI needs
`ASSAY_ADMIN_TOKEN` exported and `ASSAY_LOCAL_MODE=false`. Do not use an ingest key as the admin token.

Network deployments additionally need HTTPS/access controls; see [deployment](deployment.md).

## Published images (future release path)

After a maintainer publishes and verifies an image, use
[`compose.published.yaml`](../compose.published.yaml) with `ASSAY_IMAGE` set to that verified tag/digest:

```bash
docker compose -f compose.published.yaml up -d
```

Do not combine it with source Compose or invent a container tag from the Python package version.

**Next:** [UI tour](concepts.md) · [SDK](python-sdk.md) · [CLI](cli.md) · [Troubleshooting](troubleshooting.md)
