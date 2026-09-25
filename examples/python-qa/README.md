# Traced Python Q&A example

A small uv project using the published `assay-sdk==0.3.0` and an OpenAI-compatible model.
Ask about Assay using three built-in context snippets, or ask general questions; every reply
sends a trace to your local Assay instance.
This optional example is separate from the current-checkout SDK workflow in the
[Linux quickstart](../../docs/quickstart-linux.md), which needs no paid provider just to trace.

The server creates or reuses the **Python Q&A example** project/application and records each
chat turn with context-loading and generation spans. Questions explicitly mentioning Assay are
eligible for automatic groundedness scoring; general questions are traced without scoring them
against unrelated Assay context. Traces contain the question, answer, supplied context, model,
and token usage. The demo project and traces remain in Assay; its temporary ingestion key is
revoked on graceful shutdown. No key is sent to the browser.

## Setup

Use the repository root `.env`. Follow the [Linux](../../docs/quickstart-linux.md) or
[PowerShell](../../docs/quickstart-powershell.md) setup first; if you do not already have `.env`,
copy `.env.example` there and set:

- `ASSAY_ADMIN_TOKEN`: the token used by the local server.
- `ASSAY_JUDGE_API_KEY`: a valid OpenAI API key if you want real OpenAI generation/scoring;
  alternatively use the local-model overlay below without a provider account.
- `ASSAY_JUDGE_MODEL`: a model available from your configured judge provider; for separate
  generator settings, set `OPENAI_MODEL` instead.

The example uses that same API key and model for its answer by default. Set `OPENAI_API_KEY` and/or
`OPENAI_MODEL` to use separate generator settings; `OPENAI_BASE_URL` points the OpenAI-compatible
client to a different endpoint. Hosted-model calls incur API usage. The local-only procedure
below uses local models for this project's generation **and** judging once its judge override is
set; other projects keep their own settings. Local models have no live internet knowledge.

## Run everything with Docker Compose

With Docker running and the root `.env` configured, run from this folder:

```bash
docker compose --env-file ../../.env up --build -d
```

Open [the chat app](http://localhost:8090) and ask a sample question. Each reply includes a
**View trace in Assay** link. The chat server uses the published SDK from PyPI.

This includes the repository's Assay/Postgres stack and waits for Assay to be healthy. It uses the
same `assay` Compose project and persistent Postgres volume as the root setup. All three services
stay running. From this folder, use:

```bash
docker compose --env-file ../../.env ps
docker compose --env-file ../../.env logs -f chat
docker compose --env-file ../../.env stop
```

The chat port binds to localhost. Set `ASSAY_CHAT_PORT` in the root `.env` to change port 8090.
Conversation history lives in browser memory; **New chat** clears it. Existing traces remain in
Assay. Requests include up to 19 recent messages, with a 4,000-character limit per message.

## Local model without an API key

With Assay and a private root `.env` already running, use the optional overlay. It runs Ollama on
the internal Compose network (no model-server host port), keeps downloaded models in a named
volume, and overrides only the chat's model connection with `qwen3:4b`. Docker CPU inference is
supported; GPU acceleration requires a Docker host with GPU passthrough. Allow roughly 7 GB for
both models and expect a slower first response while the model loads.

```bash
cd examples/python-qa
docker compose --env-file ../../.env -f compose.yaml -f compose.local-model.yaml up -d --no-deps --wait ollama
docker compose --env-file ../../.env -f compose.yaml -f compose.local-model.yaml exec ollama ollama pull qwen3:4b
docker compose --env-file ../../.env -f compose.yaml -f compose.local-model.yaml exec ollama ollama pull mistral:7b
docker compose --env-file ../../.env -f compose.yaml -f compose.local-model.yaml up -d --build --no-deps --wait chat
```

**Required for local-only operation:** configure this **project only** to use the cross-family
local Mistral judge before sending Assay questions. The overlay changes the chat generator, not
the Assay server's judge defaults. Without this step, a valid hosted-judge credential in the root
`.env` can still trigger paid requests (or an invalid credential will fail scoring). From the same
folder, with the admin token in `.env`:

```bash
uv run --env-file ../../.env python - <<'PY'
import os
import assay

with assay.Client("http://127.0.0.1:8080", admin_token=os.environ["ASSAY_ADMIN_TOKEN"]) as api:
    project = next(p for p in api.projects.list() if p.name == "python-qa-example")
    api.projects.update(project.id, judge_config=assay.JudgeConfig(
        base_url="http://ollama:11434/v1", model="mistral:7b", api_key="ollama"
    ))
PY
```

Ollama ignores the literal `ollama` key; it is not a paid provider credential. Open
<http://127.0.0.1:8090/>. The Assay project and traces persist across chat restarts. These local
models demonstrate the workflow, not independent ground truth or web search. If using the same
model for both generation and judging instead, expect self-preference bias. The project judge
override persists after stopping the overlay; clear it explicitly before switching back to a
hosted judge.

## Run with uv against an existing local instance

From this folder:

```bash
uv run --env-file ../../.env uvicorn server:app --host 127.0.0.1 --port 8090
```

Or ask a single question from the terminal:

```bash
uv run --env-file ../../.env app.py
uv run --env-file ../../.env app.py "Where does Assay store traces?"
```

`uv` installs Python 3.13 and the locked dependencies as needed. The endpoint defaults to
`http://localhost:8080`. Set `ASSAY_ENDPOINT` for a different address. If the server is not running,
start it from the repository root with `docker compose up --build -d`.

## Inspect the result

For offline evaluations, create a dataset and add questions with expected answers and context.
Choose **Generate then score** when items have no recorded answer. Compose registers the chat API
as the example application's target endpoint on first startup; an existing endpoint is preserved.
**Score existing outputs** requires a recorded answer on every item. Correctness requires expected
answers. When running with uv, set `ASSAY_TARGET_URL` to the chat API address reachable from Assay
(for Docker Desktop, typically `http://host.docker.internal:8090/api/chat`).

Click the reply's trace link, or open [local Assay](http://localhost:8080), and connect with the
admin token from `.env`. Choose **Python Q&A example** and open its newest trace. Expand
`generate-answer` to inspect the captured context and answer. Groundedness scores for questions
explicitly mentioning Assay arrive asynchronously when the judge is configured; general questions
do not receive a groundedness score for unrelated context. Refresh after a few seconds. The Score
trends tab includes completed scores.

The example deliberately captures synthetic Q&A content. When adapting it to real inputs, choose
what to capture and redact sensitive content before calling the tracing setters. To turn a failed
trace into a regression, attach a reference, score it, import its retained evidence into a dataset,
and create a new run; editing that dataset later does not rewrite earlier run snapshots.

## Checks

```bash
uv run ruff check .
uv run ruff format --check .
uv run ty check
uv run pytest -q
```

Tests use an in-memory span exporter and a mocked OpenAI boundary, so they need neither Docker nor
API credentials. `app.py` demonstrates `assay.init`, `@assay.trace`, `assay.span`, captured input,
output and context, usage attributes, `assay.flush`, and `assay.shutdown`.
