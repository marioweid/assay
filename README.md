<p align="center">
  <img src="assets/assay_gopher.png" alt="Assay gopher and wordmark" width="480">
</p>

**LLM tracing + evaluation in one Go binary and a Postgres.** OTLP-native, judges built in, agent-operable — the self-host that isn't a six-container science project.

Assay ingests traces over OpenTelemetry (OTLP), stores them in Postgres, and runs LLM-as-judge scorers (**groundedness** + **correctness** in v1) both **online** (on live traces) and **offline** (on datasets) — with a Python client, a CLI, and a Claude Code skill so an agent can drive the whole loop.

- **Two components, forever:** `assayd` (single Go binary — API + embedded worker) + Postgres. No ClickHouse, Redis, S3, or Kafka.
- **OTLP-native:** send from any OpenTelemetry SDK (or the ergonomic `assay` Python library). Zero lock-in — plain, exportable Postgres at rest.
- **Judges where your data lives:** groundedness (claim-decomposition) and correctness (reference-based), computed with no extra infrastructure, CI-gate-able.
- **Agent-native:** CLI + Claude Code skill turn a production failure into a regression test.
- **Batteries-included UI:** a minimal React SPA is embedded in the binary (served at `/`) — no extra container. (v1 is a single-user test UI; real login comes later.)
- **License:** Apache-2.0.

> Built for individual developers, small teams, and self-hosters running under ~1M spans/day. **Not** for billion-span enterprises needing SSO/RBAC/HA — that's what the heavy platforms are for.

## Status

M5 tracing, the Python API client, and CLI orchestration are implemented. Assay accepts JSON
OTLP/HTTP traces, automatically or explicitly queues groundedness/correctness scoring, supports
reference attachment, and returns online scores and task state with trace detail. Offline runs
support persisted dataset output or per-run generation through an encrypted application target
configuration. Binary protobuf, OTLP/gRPC, `trace_selection` runs, score export/filter commands,
and the UI remain deferred.

The implementation follows these references:

- **Design & build plan:** [`docs/specs/2026-08-26-assay-design.md`](docs/specs/2026-08-26-assay-design.md)
- **Backend architecture and layer guide:** [`docs/architecture.md`](docs/architecture.md)
- **Semantic conventions (trace attribute contract):** [`docs/semantic-conventions.md`](docs/semantic-conventions.md)
- **CI/CD plan (pinned versions):** [`docs/ci-cd.md`](docs/ci-cd.md)
- **Agent skill:** [`.claude/skills/assay/SKILL.md`](.claude/skills/assay/SKILL.md)

## Python SDK

Install the Python distribution from PyPI with uv:

```bash
uv add assay-sdk
```

The distribution is named `assay-sdk` and imported as `assay`:

```python
import assay

print(assay.__version__)
```

Version 0.2.0 provides opt-in tracing, a typed synchronous API client, dataset import workflows,
and the `assay` CLI. Configure the CLI with `ASSAY_ENDPOINT` and the credential required by the
operation: `ASSAY_ADMIN_TOKEN` for management and evaluation commands or `ASSAY_API_KEY` for trace
commands.

## Repo layout

```
assayd/                 # Go 1.27 backend (single binary): API + OTLP receiver + embedded worker + UI
web/                     # React + Vite + Tailwind + shadcn/ui SPA (embedded into the binary)
clients/python/assay/   # assay-sdk distribution: tracing, typed client, and CLI
.claude/skills/assay/    # Claude Code skill wrapping the CLI
assets/                  # reusable brand assets, including the transparent app icon
docs/                    # design spec and supporting documentation
```

## Deployment

One binary, one Postgres — two ways to run it:

- **Standalone / local:** `docker compose up` (assayd + postgres). One command; migrations auto-apply on start.
- **Scale:** assayd container(s) against a managed/separate Postgres; add replicas for worker capacity (the queue lives in Postgres). No SQLite, no object store — Postgres only.

## Development quickstart

```bash
cp .env.example .env       # PowerShell: Copy-Item .env.example .env
docker compose up          # assayd + postgres; migrations auto-apply on start
```

Create the first project, API key, and application from PowerShell:

```powershell
$adminToken = Read-Host "ASSAY_ADMIN_TOKEN from .env"
$headers = @{ Authorization = "Bearer $adminToken" }
$project = Invoke-RestMethod -Method Post -Uri http://localhost:8080/v1/projects `
  -Headers $headers -ContentType application/json -Body '{"name":"support"}'
$key = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8080/v1/projects/$($project.id)/keys" `
  -Headers $headers -ContentType application/json -Body '{"name":"local"}'
$appBody = @{ project_id = $project.id; name = "Support Bot"; slug = "support-bot" } |
  ConvertTo-Json
$application = Invoke-RestMethod -Method Post -Uri http://localhost:8080/v1/applications `
  -Headers $headers -ContentType application/json -Body $appBody
$key.key # Save now: the plaintext key is returned only once.
```

Export a JSON OTLP trace and read it back:

```powershell
$otlpHeaders = @{ "x-api-key" = $key.key }
$trace = @{
  resourceSpans = @(@{
    resource = @{ attributes = @(@{
      key = "assay.application.slug"; value = @{ stringValue = "support-bot" }
    }) }
    scopeSpans = @(@{ spans = @(@{
      traceId = "00112233445566778899aabbccddeeff"
      spanId = "0102030405060708"
      name = "answer"
      startTimeUnixNano = "1787911200000000000"
      endTimeUnixNano = "1787911201000000000"
    }) })
  })
} | ConvertTo-Json -Depth 10
Invoke-RestMethod -Method Post -Uri http://localhost:8080/v1/traces `
  -Headers $otlpHeaders -ContentType application/json -Body $trace
Invoke-RestMethod -Method Get -Uri http://localhost:8080/v1/traces -Headers $otlpHeaders
```

Assay accepts OTLP's protobuf-defined JSON representation, not binary protobuf payloads. Configure
emitters for `http/json`; binary `application/x-protobuf` and gRPC are planned after v1.

Create and score an offline dataset after setting `ASSAY_JUDGE_BASE_URL` and
`ASSAY_JUDGE_MODEL` to an OpenAI-compatible endpoint:

```powershell
$datasetBody = @{
  application_id = $application.id
  name = "support-regression"
} | ConvertTo-Json
$dataset = Invoke-RestMethod -Method Post -Uri http://localhost:8080/v1/datasets `
  -Headers $headers -ContentType application/json -Body $datasetBody

$itemsBody = @{ items = @(@{
  external_id = "case-1"
  input = @{ question = "What is Assay?" }
  output = "Assay evaluates AI systems."
  expected_output = "Assay evaluates AI systems."
  context = @(@{ id = "k0"; text = "Assay evaluates AI systems." })
}) } | ConvertTo-Json -Depth 8
Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8080/v1/datasets/$($dataset.id)/items" `
  -Headers $headers -ContentType application/json -Body $itemsBody

$runBody = @{
  application_id = $application.id
  dataset_id = $dataset.id
  name = "baseline"
  mode = "score_existing"
  scorers = @("groundedness", "correctness")
} | ConvertTo-Json
$run = Invoke-RestMethod -Method Post -Uri http://localhost:8080/v1/runs `
  -Headers $headers -ContentType application/json -Body $runBody
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/v1/runs/$($run.id)" `
  -Headers $headers
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/v1/runs/$($run.id)/scores" `
  -Headers $headers
```

The judge adapter calls `/chat/completions` with temperature `0` and JSON-object response mode.
Judge settings resolve from process defaults, then project settings, then per-application scorer
overrides. Stored API keys are AES-GCM encrypted; REST responses expose only `has_api_key`.

OpenAPI is available at `http://localhost:8080/openapi.json`; interactive docs are at
`http://localhost:8080/docs`.

```python
import assay
assay.init(endpoint="http://localhost:8080", api_key="asy_...",
           application="support-bot", capture=True)

@assay.trace
def answer(q: str) -> str:
    ...
```
