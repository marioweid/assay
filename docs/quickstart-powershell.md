# First run: Windows PowerShell

[Documentation](index.md) / Getting started · [Linux / macOS instructions](quickstart-linux.md)

**Goal:** open Assay without an admin-token prompt and send a real trace. No model provider needed.

## 1. Prerequisites

Install Git, Docker Desktop (Linux containers), PowerShell 7, and [uv](https://docs.astral.sh/uv/).
Start Docker Desktop, then open PowerShell in a private user-owned directory:

```powershell
git clone https://github.com/marioweid/assay.git
Set-Location assay
```

Use the checkout containing local-mode support. No Assay container image is published yet;
SDK local-mode/session features require a verified 0.4.0+ release or this checkout (not 0.3.0).
If reusing an existing database, read [upgrade and rollback](deployment.md#upgrade-and-rollback)
first: migrations run automatically when the server starts.

## 2. Create a private local configuration

The following refuses to overwrite `.env` and writes generated secrets without displaying them:

```powershell
function New-HexSecret {
  $bytes = [byte[]]::new(32)
  [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
  [Convert]::ToHexString($bytes).ToLowerInvariant()
}

$postgresPassword = New-HexSecret
$keyBytes = [byte[]]::new(32)
[System.Security.Cryptography.RandomNumberGenerator]::Fill($keyBytes)
$encryptionKey = [Convert]::ToBase64String($keyBytes)
$content = "ASSAY_LOCAL_MODE=true`nASSAY_POSTGRES_PASSWORD=$postgresPassword`nASSAY_ENCRYPTION_KEY=$encryptionKey`n"
$path = Join-Path (Get-Location) '.env'
$stream = [System.IO.File]::Open($path, 'CreateNew', 'Write', 'None')
try {
  $bytes = [System.Text.UTF8Encoding]::new($false).GetBytes($content)
  $stream.Write($bytes, 0, $bytes.Length)
} finally {
  $stream.Dispose()
}
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls .env /inheritance:r /grant:r "${identity}:(F)" '*S-1-5-18:(F)'
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict .env permissions' }
Remove-Variable content, postgresPassword, encryptionKey, keyBytes, bytes
```

Already have `.env`? Keep its database password and encryption key. Change only
`ASSAY_LOCAL_MODE=true` for local use. Regenerating the encryption key prevents decryption of stored
judge/target secrets; changing an environment password does not rotate an existing database's password.

## 3. Start Assay

```powershell
docker compose up --build -d
docker compose ps
Invoke-RestMethod http://localhost:8080/readyz
Invoke-RestMethod http://localhost:8080/v1/server-info
```

Open **<http://localhost:8080/>**. You should see **Local mode**, not a token form.
`/v1/server-info` should report `local_mode: true`.

Keep the checked-in loopback bindings. Local mode gives management access to anyone who can reach
this server; never expose it publicly. It does **not** remove project ingest keys.

Stop with `docker compose down`. Do **not** add `-v`: that deletes the database volume.

## 4. Create a workspace

In the UI: **Projects → New project → API key**, then **Applications → New application**.
Save the raw project key when shown; it is only displayed once. Or bootstrap through the checkout SDK:

```powershell
$env:ASSAY_ENDPOINT = 'http://localhost:8080'
$env:ASSAY_LOCAL_MODE = 'true'
uv run --project clients/python/assay python examples/quickstart/bootstrap.py `
  --output .quickstart-workspace.json
if ($LASTEXITCODE -ne 0) { throw 'Workspace creation failed' }
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls .quickstart-workspace.json /inheritance:r /grant:r "${identity}:(F)" '*S-1-5-18:(F)'
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict workspace permissions' }
```

Python does not implicitly load Docker's `.env`. Export `ASSAY_LOCAL_MODE` for host-side management
commands as shown. The workspace JSON contains a plaintext project key; never commit or share it.
The script refuses to overwrite an existing workspace file. Unix file modes alone do not establish
private Windows ACLs, hence the explicit `icacls` command above.

## 5. Send your first trace

```powershell
$traceId = uv run --project clients/python/assay python examples/quickstart/trace.py `
  --workspace .quickstart-workspace.json
if ($LASTEXITCODE -ne 0) { throw 'Trace export failed' }
"OpenTelemetry trace ID: $traceId"
```

Open the new application → **Traces** and search for the printed ID. Inspect its question, answer,
context and timing. The example exports synthetic content and checks its flush result.
To inspect through the CLI without displaying the project key:

```powershell
$workspace = Get-Content .quickstart-workspace.json -Raw | ConvertFrom-Json
$env:ASSAY_API_KEY = $workspace.api_key
uv run --project clients/python/assay assay traces list $workspace.application_id --query $traceId
```

Next, instrument your own application with the [Python SDK](python-sdk.md). Use
`assay.session(...)` for multiple request traces belonging to one conversation; the quickstart
example remains a normal untagged trace. Continue with [evaluations](evaluations.md) when ready.

## Token mode and published deployments

For normal token authentication, set `ASSAY_LOCAL_MODE=false` and a separately generated
`ASSAY_ADMIN_TOKEN` in `.env`; recreate the server with `docker compose up -d`, then reload the UI.
Set `$env:ASSAY_LOCAL_MODE = 'false'` and supply the admin token privately to the host CLI/client.
Network deployments also need HTTPS and access controls; see [deployment](deployment.md).

When a container release is published and verified, use
[`compose.published.yaml`](../compose.published.yaml) with the recorded `ASSAY_IMAGE` tag/digest:

```powershell
docker compose -f compose.published.yaml up -d
```

Do not combine it with source Compose or infer an image tag from the Python package version.

**Next:** [UI tour](concepts.md) · [SDK](python-sdk.md) · [CLI](cli.md) · [Troubleshooting](troubleshooting.md)
