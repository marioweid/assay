# PowerShell quickstart

This is the Windows equivalent of the [Linux quickstart](quickstart-linux.md). Install Docker Desktop
with Compose, PowerShell 7, and [uv](https://docs.astral.sh/uv/). The containers are still Linux
containers; only the host commands differ.

## Create private credentials

Generate independent tokens in PowerShell. The first two are URL-safe hexadecimal text; the third is
an AES-256-GCM key encoded as base64. Save all three in `.env`, not in command history.

```powershell
function New-HexSecret {
  $bytes = [byte[]]::new(32)
  [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
  [Convert]::ToHexString($bytes).ToLowerInvariant()
}

$adminToken = New-HexSecret
$postgresPassword = New-HexSecret
$keyBytes = [byte[]]::new(32)
[System.Security.Cryptography.RandomNumberGenerator]::Fill($keyBytes)
$encryptionKey = [Convert]::ToBase64String($keyBytes)
```

Keep `ASSAY_ENCRYPTION_KEY` with the database backup. Replacing it later prevents decryption of
stored judge and target-endpoint secrets.

## 1. Published image (release only)

**No image has been published yet.** Wait for an Assay release to name an anonymously pull-tested tag
or digest. In a new directory, save the exact
[`compose.published.yaml`](../compose.published.yaml) as `compose.yaml`, then create `.env` with
`ASSAY_IMAGE` set to that verified release plus `ASSAY_ADMIN_TOKEN`, `ASSAY_POSTGRES_PASSWORD`, and
`ASSAY_ENCRYPTION_KEY` from above:

```powershell
docker compose up --build --force-recreate -d
docker compose ps
Invoke-WebRequest http://localhost:8080/readyz | Select-Object -Expand StatusCode
```

Published Compose has no `build` stanza, so `--build` is intentionally harmless. When using the
repository filename directly, run
`docker compose -f compose.published.yaml up --build --force-recreate -d`. Do not merge it with the
source Compose file.

## 2. Source checkout

From a repository checkout, preserve any existing secrets file and create a new one:

```powershell
if (Test-Path .env) {
  throw '.env already exists; refusing to overwrite it'
}
Copy-Item .env.example .env
# Edit .env: set ASSAY_ADMIN_TOKEN, ASSAY_ENCRYPTION_KEY, and a fresh ASSAY_POSTGRES_PASSWORD.
docker compose up --build --force-recreate -d
```

`--force-recreate` recreates containers but preserves the named `assay-pgdata` volume. Open
<http://localhost:8080/> and connect with the `ASSAY_ADMIN_TOKEN` from `.env`. The embedded UI is a
single-user admin-token tool, not a login system.

## 3. Create an application and emit a trace

Use the UI to create a project, its one-time ingest key, and an application. Or use the current
checkout SDK. Load only the variables needed by the script—PowerShell does not use Bash `source`:

```powershell
Get-Content .env | ForEach-Object {
  if ($_ -match '^(ASSAY_ADMIN_TOKEN)=(.+)$') {
    Set-Item "Env:$($Matches[1])" $Matches[2]
  }
}
$env:ASSAY_ENDPOINT = 'http://localhost:8080'
uv run --project clients/python/assay python examples/quickstart/bootstrap.py `
  --output .quickstart-workspace.json
$traceId = uv run --project clients/python/assay python examples/quickstart/trace.py `
  --workspace .quickstart-workspace.json
"OpenTelemetry trace ID: $traceId"
```

The private workspace file is written with owner-only permissions where the filesystem supports
that mode. It contains the raw project key; remove it when you are done. To find the trace through
the CLI, load the key and application ID from the file without displaying the key:

```powershell
$workspace = Get-Content .quickstart-workspace.json -Raw | ConvertFrom-Json
$env:ASSAY_API_KEY = $workspace.api_key
uv run --project clients/python/assay assay traces list $workspace.application_id --query $traceId
```

The project API key is for trace ingestion and project operations. The admin token is for management,
datasets, scorers, and runs. Assay accepts JSON OTLP/HTTP only; binary protobuf and OTLP/gRPC are not
implemented.

## 4. Evaluate a synthetic case (optional)

Tracing needs no judge credential. To run scorers, configure an OpenAI-compatible judge in the UI or
set `ASSAY_JUDGE_BASE_URL` and `ASSAY_JUDGE_MODEL` in `.env`; set `ASSAY_JUDGE_API_KEY` only when that
endpoint requires authentication, then rerun `docker compose up -d` so the server receives it. On
Docker Desktop, a host-local model is normally reachable from
Assay as `http://host.docker.internal:11434/v1`.

```powershell
$dataset = uv run --project clients/python/assay assay datasets import $workspace.application_id `
  --file examples/quickstart/regression.jsonl | ConvertFrom-Json
$run = uv run --project clients/python/assay assay run create $workspace.application_id `
  --dataset $dataset.dataset_id --scorers groundedness,correctness | ConvertFrom-Json
uv run --project clients/python/assay assay run watch $run.id
```

Inspect the run and item evidence in the UI. Editing or deleting an individual dataset item does not
change an existing run snapshot. Deleting the whole dataset cascades its dependent runs.

## URLs and deployment

Use `http://localhost:8080` from Windows. A service in the same Compose project uses
`http://assayd:8080`. The database URL uses `sslmode=disable` only on the trusted internal Docker
network; use a TLS reverse proxy for network deployments. See [deployment](deployment.md) for
backups, restore, upgrades, and destructive-operation warnings.
