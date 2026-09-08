# Traced Python Q&A example

A small uv project using the published `assay-sdk==0.3.0` and OpenAI. Ask a question about Assay;
the app answers from three built-in context snippets and sends a trace to your local Assay instance.

The server creates or reuses the **Python Q&A example** application, records each chat turn with
context-loading and generation spans, and enables automatic groundedness scoring. Traces contain
the question, answer, supplied context, model, and token usage. The demo project and traces remain
in Assay; its temporary ingestion key is revoked on graceful shutdown. No key is sent to the browser.

## Setup

Use the repository root `.env`. If you do not already have it, copy `.env.example` there and set:

- `ASSAY_ADMIN_TOKEN`: the token used by the local server.
- `ASSAY_JUDGE_API_KEY`: your OpenAI API key.
- `ASSAY_JUDGE_MODEL`: an OpenAI judge model, for example `gpt-5.6-luna`.

The example uses that same API key and model for its answer by default. Set `OPENAI_API_KEY` and/or
`OPENAI_MODEL` to use separate generator settings. Real model calls incur API usage: normally one
answer request plus two asynchronous groundedness judge requests per run.

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
`generate-answer` to inspect the captured context and answer. Groundedness scores arrive
asynchronously; refresh after a few seconds. The Score trends tab includes completed scores.

The example deliberately captures synthetic Q&A content. When adapting it to real inputs, choose
what to capture and redact sensitive content before calling the tracing setters.

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
