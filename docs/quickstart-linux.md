# Linux quickstart

This guide has two supported paths. The published-image path is for a release; the source path is
for a checkout. Both run Assay and Postgres on loopback-only ports.

## Before you start

Install Docker Engine with the Compose plugin, OpenSSL, and (for the SDK examples) [uv](https://docs.astral.sh/uv/).
Assay needs an admin token, a database password, and an encryption key. Generate each separately:

```bash
openssl rand -hex 32     # admin token or URL-safe Postgres password; generate separately
openssl rand -base64 32  # encryption key; save once and retain with the database backup
```

Put the values in a private `.env` file. Do not put expanded secrets in shell history. The encryption
key must decode to 32 bytes and must survive restarts and upgrades; losing it prevents Assay from
decrypting stored judge and target-endpoint secrets.

## 1. Published image (release only)

**No image has been published yet.** Wait for the release notes to name an anonymously pull-tested
tag or digest; do not substitute the Python package version. In a new directory, save the exact
[`compose.published.yaml`](../compose.published.yaml) from this repository as `compose.yaml`, create
`.env` with `ASSAY_IMAGE` set to that verified release plus the three required credentials, then run:

```bash
umask 077
docker compose up --build --force-recreate -d
docker compose ps
curl --fail http://localhost:8080/readyz
```

`--build` is harmless here: published Compose deliberately has no `build` stanza. If you keep the
repository filename instead, run the same command with
`docker compose -f compose.published.yaml up --build --force-recreate -d`. Do not combine the source
and published Compose files.

## 2. Source checkout

Clone the repository and enter it. Refuse to replace an existing secrets file, then start the source
Compose stack:

```bash
umask 077
if [ -e .env ]; then
  printf '%s\n' '.env already exists; refusing to overwrite it' >&2
  exit 1
fi
cp .env.example .env
# Edit .env: set ASSAY_ADMIN_TOKEN and ASSAY_ENCRYPTION_KEY; use a fresh database password.
docker compose up --build --force-recreate -d
```

`--force-recreate` recreates containers, not the named `assay-pgdata` volume. Check the service with
`docker compose ps` and `curl --fail http://localhost:8080/readyz`, then open
<http://localhost:8080/>. Connect the single-user UI with `ASSAY_ADMIN_TOKEN` from `.env`.

## 3. Create an application and send a trace

In the UI, create a project, create its one-time ingest key, then create an application. The UI shows
the raw key once; save it in a password manager. An agent can do the equivalent with the current
checkout SDK without retyping IDs or keys:

```bash
set -a
. ./.env
set +a
export ASSAY_ENDPOINT=http://localhost:8080
uv run --project clients/python/assay python examples/quickstart/bootstrap.py \
  --output .quickstart-workspace.json
TRACE_ID=$(uv run --project clients/python/assay python examples/quickstart/trace.py \
  --workspace .quickstart-workspace.json)
printf 'OpenTelemetry trace ID: %s\n' "$TRACE_ID"
```

The workspace file contains the plaintext ingest key, so it is mode `0600`; delete it when finished.
The trace example captures only synthetic content, calls `flush`, then shuts down. Find the trace in
the UI by its OpenTelemetry ID, or query it with the project key:

```bash
ASSAY_API_KEY=$(uv run --project clients/python/assay python -c \
  'import json; print(json.load(open(".quickstart-workspace.json"))["api_key"])')
APP_ID=$(uv run --project clients/python/assay python -c \
  'import json; print(json.load(open(".quickstart-workspace.json"))["application_id"])')
ASSAY_API_KEY="$ASSAY_API_KEY" uv run --project clients/python/assay \
  assay traces list "$APP_ID" --query "$TRACE_ID"
```

Trace ingestion needs the project API key. Creating projects, datasets, scorers, and runs needs the
admin token. The SDK uses JSON OTLP/HTTP; binary protobuf and OTLP/gRPC are not supported.

## 4. Run an evaluation (optional)

Tracing works with no judge configuration. To evaluate, configure an OpenAI-compatible judge in the
UI (global, project, or scorer settings) with `ASSAY_JUDGE_BASE_URL` and `ASSAY_JUDGE_MODEL`; set
`ASSAY_JUDGE_API_KEY` only when that judge requires one. After editing `.env`, rerun
`docker compose up -d` so the server receives it. For a host-local Linux model, Compose maps
`host.docker.internal` to the host gateway, so a typical URL is
`http://host.docker.internal:11434/v1`.

After the judge is configured, import the included synthetic case, create a score-existing run, and
watch it:

```bash
DATASET_ID=$(uv run --project clients/python/assay assay datasets import "$APP_ID" \
  --file examples/quickstart/regression.jsonl | uv run --project clients/python/assay python -c \
  'import json,sys; print(json.load(sys.stdin)["dataset_id"])')
RUN_ID=$(uv run --project clients/python/assay assay run create "$APP_ID" --dataset "$DATASET_ID" \
  --scorers groundedness,correctness | uv run --project clients/python/assay python -c \
  'import json,sys; print(json.load(sys.stdin)["id"])')
uv run --project clients/python/assay assay run watch "$RUN_ID"
```

The fake judge in the disposable acceptance harness is for tests only; it is not part of a normal
deployment. View run items and scores in the UI. Dataset-item edits or deletion do not rewrite an
existing run's immutable snapshot; deleting the whole dataset cascades its dependent runs.

## URLs and local-network limits

`http://localhost:8080` is for a host SDK, CLI, or browser. Inside another Compose service, use
`http://assayd:8080`. The Compose database URL uses `postgres` and `sslmode=disable` only on the
trusted internal Docker network. For a network deployment, put Assay behind a TLS-terminating reverse
proxy; do not expose Postgres or use this local-only TLS setting across an untrusted network.

For backups, upgrades, retention, and production network guidance, see
[deployment](deployment.md). Windows users should follow the complete
[PowerShell guide](quickstart-powershell.md).
