# Assay documentation

**Trace a conversation. Inspect the evidence. Turn a failure into a regression test.**

Assay is a self-hosted tracing and evaluation workspace: one Go service, its embedded UI,
Postgres, and a Python SDK/CLI. You can start capturing traces without a model provider or judge.

> **Start here:** [Linux / macOS terminal](quickstart-linux.md) ·
> [Windows PowerShell](quickstart-powershell.md)
>
> The quickstarts use a source build and opt-in local mode: no admin-token prompt, but project
> ingest keys remain. Published container images are not available yet. Local-mode/session SDK
> features require `assay-sdk` 0.4.0 or newer, once its release is verified, or this checkout;
> published 0.3.0 does not include them.

## Your first hour

| Step | Outcome | Guide |
|---|---|---|
| 1. Start locally | Open Assay at `http://localhost:8080` | [Linux](quickstart-linux.md) / [Windows](quickstart-powershell.md) |
| 2. Create a workspace | A project, application and one-time ingest key | [Concepts and UI tour](concepts.md) |
| 3. Send your first trace | See inputs, outputs, context and timings | [Python SDK](python-sdk.md) |
| 4. Connect conversation turns | Multiple traces appear as one session | [Sessions](python-sdk.md#sessions-across-multiple-requests) |
| 5. Evaluate an answer | Groundedness/correctness with retained evidence | [Evaluations](evaluations.md) |
| 6. Keep it safe | Stable secrets, backups, controlled upgrades | [Deployment and recovery](deployment.md) |

## Guides and reference

- **[Python SDK](python-sdk.md)** — install, lifecycle, decorators, explicit spans, async use,
  structured messages/tools, redaction, session context, management client and pagination.
- **[CLI cookbook](cli.md)** — bootstrap, inspect, score, import, evaluate, export and CI gates.
- **[Configuration](configuration.md)** — server, Compose, SDK, judge and local-mode settings.
- **[Troubleshooting](troubleshooting.md)** — token prompts, 401/403, missing traces/sessions,
  Docker networking, scoring failures and SDK version mismatches.
- **[Semantic conventions](semantic-conventions.md)** — the exact OTLP/GenAI attribute contract.
- **API reference:** open `/docs` on your running Assay server; `/openapi.json` is machine-readable.
- **[Python chat example](../examples/python-qa/README.md)** — a separate chat service using a
  real model, persisted conversations and optional evaluation. It is not needed for onboarding.

## Understand the boundaries

Local mode is not a login system. **Anyone who can reach a local-mode server has management
access**, including deletion. Keep the Compose loopback binding and trust every container on
its network. Project keys still scope ingestion and explicit project-key API reads, but are
not an isolation boundary against someone who can make anonymous local management requests.

The SDK sends JSON OTLP/HTTP. Binary protobuf, OTLP/gRPC, automatic provider instrumentation,
SSO/RBAC and high-availability deployment are not implemented. Capture is opt-in; explicit
message/context setters intentionally export the values you supply.

## Working on Assay

[Architecture](architecture.md) · [CI/CD and acceptance](ci-cd.md) ·
[Current UI/session rollout plan](plans/2026-09-25-session-explorer-and-ui.md)

The guides describe the current checkout. A successful source build is not evidence that a
container image or SDK release has been published; check PyPI for the verified package version.
