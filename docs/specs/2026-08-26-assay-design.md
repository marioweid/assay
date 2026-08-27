# Assay — Design & Build Plan

*Status: design spec, 2026-08-26. This is the single source of truth for what Assay is and how it's built. It is intentionally detailed so implementation has as few knowledge gaps as possible.*

For the implemented backend layers, dependency direction, and an API-key request walkthrough,
see [`docs/architecture.md`](../architecture.md).

---

## 0. TL;DR

Assay is a **self-hostable GenAI evaluation + tracing platform**: one Go binary + one Postgres. It ingests traces over **OTLP** (OpenTelemetry, no proprietary SDK required), stores them in Postgres, and runs **LLM-as-judge scorers** (groundedness + correctness in v1) both **online** (on live traces) and **offline** (on datasets). A Python client (`assay`) provides an ergonomic `@assay.trace` decorator + a full API client, a `assay` CLI wraps the API, and a Claude Code skill wraps the CLI so an agent can drive the whole loop.

- **Language/runtime:** Go 1.27, single binary `assayd` (API + embedded worker).
- **Store:** Postgres only (no ClickHouse/Redis/S3). Job queue is Postgres (`SELECT … FOR UPDATE SKIP LOCKED`).
- **Judge:** OpenAI-compatible chat API (OpenAI/Azure/OpenRouter/vLLM/Ollama), config resolved global → project → scorer.
- **License:** Apache-2.0 (OSI).

---

## 1. Principles & non-goals

**Principles**
1. **Two components, forever.** Binary + Postgres. Adding a fifth service is a design failure.
2. **OTLP-native, zero lock-in.** Standard OTel in; plain exportable Postgres at rest.
3. **Eval where the data already lives.** Scores are computed against the same spans you traced, persisted natively.
4. **Agent-native.** Every outcome a human can achieve via CLI/API, an agent can too (Claude skill).
5. **Honest.** Publish the Postgres span ceiling; frame judges as CI gates + triage, not oracle truth.
6. **Config-agnostic core.** Business logic never reads env directly; runtime config is injected once at startup.

**Non-goals for v1:** no ClickHouse/Redis/S3/Kafka/Qdrant; no managed SaaS; no prompt-management empire, fine-tuning, RBAC/SSO/audit, or red-teaming; not a general APM; no gRPC OTLP (HTTP only in v1).

**Storage decision (why one engine, not two):** Assay uses **Postgres only** — no SQLite backend, no object store (MinIO/S3). This is deliberate. A SQLite mode would fight every core design decision: the job queue is `SELECT … FOR UPDATE SKIP LOCKED` (SQLite has no `SKIP LOCKED` — it's single-writer with a global lock), and we rely on `JSONB`, `text[]` arrays, and monthly range **partitioning** (none exist in SQLite). Supporting both would force a storage-abstraction layer, dual query code, and a permanent 2× test matrix. Object storage (the role MinIO plays in Langfuse/Helicone — holding large blob payloads) is unnecessary because large text stays in Postgres behind **opt-in capture + size caps**. Our differentiator is "2 components, not 6," not "1 component." "Standalone vs multi-container" is achieved by **deployment shape, not a second database engine** (see §14).

---

## 2. Pinned stack (verified 2026-08-26)

| Concern | Choice | Version | Import path |
|---|---|---|---|
| Language | Go | **1.27.0** | `go 1.27` in `go.mod` |
| Base router | stdlib `net/http` ServeMux | Go 1.27 | `net/http` |
| REST API layer | **huma v2** (code-first, auto OpenAPI 3.1 + validation) | **v2.39.1** | `github.com/danielgtaylor/huma/v2` (+ `humago` adapter) |
| Postgres driver | pgx + pgxpool | **v5.10.0** | `github.com/jackc/pgx/v5`, `.../pgxpool`, `.../stdlib` |
| Type-safe SQL | sqlc (`sql_package: pgx/v5`) | **v1.31.1** | `github.com/sqlc-dev/sqlc` (codegen) |
| Migrations | goose Provider API + `embed.FS` | **v3.27.3** | `github.com/pressly/goose/v3` |
| OTLP proto types | opentelemetry-proto-go | **v1.11.0** | `go.opentelemetry.io/proto/otlp` |
| Config | caarlos0/env | **v11.4.1** | `github.com/caarlos0/env/v11` |
| Logging | stdlib `log/slog` | Go 1.27 | `log/slog` |
| Python client | OTel SDK + OTLP/HTTP exporter | **1.44.0** | `opentelemetry-{api,sdk}`, `opentelemetry-exporter-otlp-proto-http` |
| Python tooling | uv, ruff, ty, pytest | latest | — |

Notes: goose runs on `database/sql`, so open a `pgx/v5/stdlib` handle for migrations alongside the `pgxpool` the app uses. `proto/otlp` v1.11 requires Go ≥1.25 (we use 1.27). Do **not** embed `go.opentelemetry.io/collector` — implement the OTLP endpoint directly.

---

## 3. Architecture

```
                       ┌──────────────────────────────────────────────┐
   OTel SDKs / any     │                  assayd (1 binary)            │
   OTLP emitter  ─OTLP▶│  ┌────────────┐   ┌───────────────────────┐  │
   (incl. `assay` lib, │  │ OTLP recv  │──▶│  ingest → spans/traces │  │
    Claude Code)       │  │ /v1/traces │   └───────────┬───────────┘  │
                       │  └────────────┘               │ enqueue      │
   Frontend / CLI /    │  ┌────────────┐   ┌───────────▼───────────┐  │      ┌──────────┐
   Claude skill ─HTTP─▶│  │ huma REST  │──▶│  domain services       │◀─┼─────▶│ Postgres │
                       │  │ /v1/...    │   │  (projects/apps/       │  │ SQL  │  (only    │
                       │  └────────────┘   │   datasets/runs/scores)│  │      │   store)  │
                       │  ┌────────────┐   └───────────┬───────────┘  │      └──────────┘
                       │  │ worker pool│◀──── claim jobs (SKIP LOCKED)─┘  │        ▲
                       │  │ goroutines │──▶ scoring engine ──▶ judge LLM ─┼────────┘
                       │  └────────────┘        │                        │   (OpenAI-compatible,
                       └────────────────────────┼────────────────────────┘    external HTTP)
                                                 ▼
                                         scores + rationales
```

**Component responsibilities (each independently testable):**

- **`otlp`** — parse OTLP/HTTP requests (protobuf + JSON + gzip), authenticate via API key, resolve owning application from resource attributes, map `ResourceSpans → traces/spans` rows, enqueue online scoring jobs. No business logic beyond ingest.
- **`api`** — huma handlers; thin adapters that authenticate and delegate to domain services. Emits OpenAPI 3.1.
- **`domain`** — services owning projects, applications, api keys, datasets, runs, scorer configs. All orchestration lives here.
- **`scoring`** — the scorer registry + judge client + groundedness/correctness implementations + prompt templates. Pure logic; judge HTTP is injected.
- **`worker`** — the Postgres-backed job queue and goroutine pool; claims and executes `eval_run` and `scoring_task` jobs; leases, retries, reaps.
- **`store`** — sqlc-generated queries + pgxpool; the only package that talks SQL.
- **`config`** — env parsing (caarlos0/env) at startup; produces a typed `Config`.
- **`crypto`** — AES-256-GCM encryption for judge API keys and target-endpoint credentials.
- **`ui`** — the embedded React SPA (built assets) served from `embed.FS` at `/`, with SPA fallback to `index.html`; talks only to `/v1`. Optional, toggled by `ASSAY_UI_ENABLED` (default on). No extra container.

**App context object** (global CLAUDE.md rule — bundle shared singletons): a single `App` struct built once in `main` holds `*pgxpool.Pool`, `*Config`, `*slog.Logger`, `*crypto.Cipher`, the domain services, and the worker handle. Handlers receive it via closure; services receive their collaborators via constructor injection (hand-wired, no DI framework).

---

## 4. Domain model (ERD)

```
Project (1)───(N) ApiKey
   │
   └──(N) Application
             ├──(N) Trace ──(N) Span
             ├──(N) Dataset ──(N) DatasetItem
             ├──(N) ScorerConfig
             └──(N) EvalRun ──(N) Score
Score → (FK) EvalRun            (offline)
Score → (FK) Trace / Span       (online)
Score → (FK) ScorerConfig
Job (queue) → polymorphic (eval_run | scoring_task)
```

- **Project** — top-level grouping. Owns API keys and applications. Holds an optional project-level judge override.
- **ApiKey** — `asy_`-prefixed random token; **stored SHA-256 hashed** (never plaintext), shown once at creation. Scoped to a project. Supports multiple active keys + revocation (rotation).
- **Application** — a GenAI use case (the old "experiment"). Holds: small JSON `config`, `auto_score` flags, and an optional **target endpoint** (URL + auth + request/response mapping) for offline generation. Endpoint credentials **AES-GCM encrypted**.
- **Trace** — one OTLP trace, owned by an application. Denormalized roots for fast listing (root span name, start/end, status, total tokens/cost).
- **Span** — OTel span; `parent_span_id` nullable for roots; attributes stored as `JSONB`. Indexed by trace, by application+time.
- **Dataset / DatasetItem** — offline eval data. Item = `{input JSONB, expected_output text NULL, context JSONB NULL, metadata JSONB}`.
- **ScorerConfig** — binds a built-in scorer (`groundedness` | `correctness`) to a judge config override + threshold + optional custom prompt template version, per application.
- **EvalRun** — an offline execution over a dataset (or a trace selection). Holds status, params, aggregates.
- **Score** — one scorer result: `{value float 0–1, passed bool, rationale text, details JSONB, judge_model, judge_provider, judge_tokens}`. Attaches to either an `eval_run`+`dataset_item` (offline) or a `trace`/`span` (online).
- **Job** — the work queue row (see §8).

---

## 5. Database schema & migrations

**Migrations:** goose `.sql` files in `assayd/db/migrations/`, embedded via `//go:embed`, applied at startup with the **goose Provider API** before the server binds (see §14). A `PostgresSessionLocker` guards concurrent replicas. Restart the container ⇒ DB migrated, zero manual steps.

**IDs:** UUID v7 (time-ordered) generated app-side for external entities; `bigint` identity for high-volume `spans`/`scores` internal PKs where helpful. (UUID v7 chosen for sortable, index-friendly keys without a DB extension.)

**Core tables (columns abbreviated; every table has `created_at`, `updated_at`):**

```sql
projects(id uuid pk, name text unique, judge_config jsonb null, created_at, updated_at)

api_keys(id uuid pk, project_id uuid fk, name text, key_hash bytea unique,
         key_prefix text,                       -- first 8 chars for display, e.g. 'asy_ab12'
         last_used_at timestamptz null, revoked_at timestamptz null)

applications(id uuid pk, project_id uuid fk, name text, slug text,
             config jsonb not null default '{}',
             auto_score_scorers text[] not null default '{}',  -- scorers to run on ingest
             target_endpoint jsonb null,          -- see §10.2; secrets encrypted
             unique(project_id, slug))

traces(id uuid pk, application_id uuid fk, otel_trace_id bytea,   -- 16 bytes
       root_name text, start_time timestamptz, end_time timestamptz,
       status text, span_count int, total_tokens int, total_cost numeric null,
       reference_answer text null,               -- attached ground truth (enables online correctness)
       attributes jsonb, unique(application_id, otel_trace_id))

spans(id bigint identity pk, trace_id uuid fk, application_id uuid fk,
      otel_span_id bytea, parent_span_id bytea null,
      name text, kind text, operation_name text,          -- gen_ai.operation.name
      start_time timestamptz, end_time timestamptz, duration_ms int,
      status_code text, status_message text null,
      is_scorable bool not null default false,             -- assay.scorable
      scorable_kind text null,
      attributes jsonb not null,                           -- full attribute bag incl gen_ai.* / assay.*
      events jsonb not null default '[]')

datasets(id uuid pk, application_id uuid fk, name text, description text null,
         unique(application_id, name))

dataset_items(id uuid pk, dataset_id uuid fk, external_id text null,
              input jsonb not null, expected_output text null,
              context jsonb null, metadata jsonb not null default '{}')

scorer_configs(id uuid pk, application_id uuid fk,
               scorer text not null,                       -- 'groundedness' | 'correctness'
               enabled bool not null default true,
               threshold numeric not null default 0.5,
               judge_config jsonb null,                    -- overrides project/global; secrets encrypted
               prompt_template_id text null,               -- null = built-in default
               unique(application_id, scorer))

eval_runs(id uuid pk, application_id uuid fk, dataset_id uuid null,
          name text, status text,                          -- pending|running|succeeded|failed|canceled
          mode text,                                        -- 'score_existing' | 'generate_then_score' | 'trace_selection'
          params jsonb, scorers text[],
          aggregates jsonb null,                            -- {scorer: {mean, pass_rate, n}}
          started_at timestamptz null, finished_at timestamptz null,
          error text null)

scores(id bigint identity pk, scorer text, scorer_config_id uuid null,
       value numeric not null, passed bool not null, rationale text,
       details jsonb not null default '{}',                -- per-claim verdicts / per-fact statuses
       judge_model text, judge_provider text, judge_tokens int,
       -- exactly one target context:
       eval_run_id uuid null, dataset_item_id uuid null,   -- offline
       trace_id uuid null, span_id bigint null,            -- online
       created_at timestamptz)

jobs(id uuid pk, kind text, payload jsonb, status text,    -- see §8
     run_after timestamptz, attempts int, max_attempts int,
     locked_by text null, locked_at timestamptz null, lease_expires_at timestamptz null,
     last_error text null, created_at, updated_at)
```

**Indexes:** `spans(trace_id)`, `spans(application_id, start_time desc)`, `traces(application_id, start_time desc)`, `scores(trace_id)`, `scores(eval_run_id)`, `jobs(status, run_after)` partial on `status='pending'`. **Partitioning:** `spans` and `scores` are `PARTITION BY RANGE (created_at)` monthly from day one (cheap now, essential at the published ceiling). A retention job drops old partitions per the configured TTL; scores survive span pruning (score rows carry denormalized judged text where needed).

---

## 6. OTLP ingestion

**Endpoint:** `POST /v1/traces` (OTLP/HTTP).

- **Encodings:** `application/x-protobuf` (request + response) and `application/json` (proto3 JSON with OTLP deviations — trace/span IDs **hex**, enums as ints). Support `Content-Encoding: gzip` request bodies (SDKs gzip by default).
- **Types:** decode into `go.opentelemetry.io/proto/otlp/collector/trace/v1.ExportTraceServiceRequest` → `[]ResourceSpans`; respond with `ExportTraceServiceResponse`.
- **Success:** `200` + serialized response. **Partial success:** `200` with `partial_success.rejected_spans` + `error_message` (clients must not retry). **Errors:** non-2xx with a `google.rpc.Status`; `429/503` are retryable.
- **Auth:** API key from `Authorization: Bearer asy_…` or `x-api-key` header → look up by SHA-256 hash → resolves project. Reject with `401` if missing/invalid/revoked. Update `last_used_at` async.
- **Application resolution:** read resource attribute `assay.application.slug` (preferred) or `service.name`; resolve `(project_id, slug) → application`. If unknown and `ASSAY_AUTO_CREATE_APPS=true`, auto-create; else reject those spans via `partial_success`.

**Mapping `ResourceSpans → rows`:**
1. Group spans by `trace_id`. Upsert a `traces` row (root = span with no parent or earliest; denormalize name/time/status/tokens).
2. Insert `spans` rows. Copy full attribute bag to `attributes` JSONB. Extract typed columns: `operation_name` from `gen_ai.operation.name`, `is_scorable`/`scorable_kind` from `assay.scorable*`, tokens from `gen_ai.usage.*`.
3. If any span carries `assay.reference.answer`, set `traces.reference_answer`.
4. For each configured `auto_score_scorers` on the app, if the trace has a scorable span and required inputs are present, enqueue a `scoring_task` job (idempotent on `(trace_id, scorer)`).

Ingestion is a single Postgres transaction per trace batch; large batches chunked. Content capture is **opt-in on the client side** (mirrors OTel default-off privacy norm).

---

## 7. Semantic conventions

Baseline = **OTel GenAI semconv** (`gen_ai.*`) — everything downstream reads it; never invent a key OTel already defines. The conventions are **Development-status and move fast**, so: pin to a spec commit, version our mapping, and co-populate standard keys.

**Read from `gen_ai.*` (baseline):**
- `gen_ai.operation.name`, `gen_ai.provider.name`, `gen_ai.request.model`, `gen_ai.response.model`, `gen_ai.response.finish_reasons` (**array**), `gen_ai.usage.input_tokens` / `output_tokens` (+ `cache_read`, `cache_creation`, `reasoning` variants).
- Retrieval (already defined, Development): `gen_ai.retrieval.query.text`, `gen_ai.retrieval.documents`.
- Evaluation results (already defined): `gen_ai.evaluation.name`, `gen_ai.evaluation.score.value`, `gen_ai.evaluation.score.label`, `gen_ai.evaluation.explanation` — **we emit our scores in these keys** so Datadog/Phoenix/Grafana render them.

**`assay.*` extensions** (the genuine gaps OTel doesn't standardize):

```
# (a) mark the scorable unit
assay.scorable            bool     # true on exactly one span per eval unit
assay.scorable.kind       string   # rag_answer | generation | agent_final | tool_result
assay.unit.id             string   # stable id (question/row id)
assay.dataset.id          string
assay.run.id              string

# (b) retrieved context (grading extras beyond gen_ai.retrieval.documents; flattened, zero-based)
assay.context.chunk.count int
assay.context.chunks.<i>.id     string
assay.context.chunks.<i>.text   string
assay.context.chunks.<i>.score  double
assay.context.chunks.<i>.source string

# (c) reference answer (the real OTel gap)
assay.reference.answer    string
assay.reference.id        string
assay.reference.source    string   # human | synthetic | curated

# (d) judge audit trail (alongside gen_ai.evaluation.*)
assay.evaluation.judge.model     string
assay.evaluation.judge.provider  string
assay.evaluation.verifier        string   # llm | hhem
```

Rules: lowercase dotted keys; lists flattened with zero-based `<i>`; large text behind opt-in capture; always co-populate the `gen_ai.*` equivalent. The `assay` Python lib sets these via helpers so users never hand-type them. This convention set is documented in full in `docs/semantic-conventions.md`.

---

## 8. Job queue & worker

**One binary, embedded pool.** N worker goroutines (configurable, default = `GOMAXPROCS`) poll the `jobs` table.

**Claim (atomic):**
```sql
UPDATE jobs SET status='running', locked_by=$worker, locked_at=now(),
       lease_expires_at = now() + $lease, attempts = attempts + 1
WHERE id = (
  SELECT id FROM jobs
  WHERE status='pending' AND run_after <= now()
  ORDER BY run_after
  FOR UPDATE SKIP LOCKED
  LIMIT 1)
RETURNING *;
```

**Job kinds:**
- `scoring_task` — payload `{trace_id, scorer}`. Loads the scorable span + context (+ reference for correctness), runs the scorer, writes a `scores` row keyed to the trace/span, and emits an evaluator child-span's-worth of `gen_ai.evaluation.*` back onto the span attributes.
- `eval_run` — payload `{eval_run_id}`. Drives an offline run (§10). May itself fan out per-item scoring inline (bounded concurrency) rather than enqueue thousands of sub-jobs, updating `eval_runs.aggregates` on completion.

**Reliability:** lease + heartbeat; a **reaper** goroutine returns jobs whose `lease_expires_at < now()` to `pending`. Retries up to `max_attempts` (default 3) with exponential backoff (`run_after = now() + base * 2^attempts`). Terminal failure → `status='failed'`, `last_error` set. Idempotency: `scoring_task` upserts on `(trace_id, scorer)` so re-runs replace, not duplicate.

**Graceful shutdown:** on SIGTERM, stop claiming, let in-flight jobs finish within a drain timeout, release leases.

---

## 9. Scoring engine

**Scorer interface (Go):**
```go
type Scorer interface {
    Name() string
    // Inputs assembled by the caller (from a span or a dataset item).
    Score(ctx context.Context, in ScoreInput, judge Judge) (ScoreResult, error)
}
type ScoreInput struct {
    Input          string    // the question/prompt
    Output         string    // the generated answer (the scorable text)
    Context        []Chunk   // retrieved context (groundedness)
    Reference      string    // gold answer (correctness)
}
type ScoreResult struct {
    Value    float64                // 0..1
    Passed   bool                   // Value >= threshold
    Rationale string
    Details  map[string]any         // per-claim verdicts / per-fact statuses
    JudgeTokens int
}
```

**Judge** = an OpenAI-compatible chat client (injected): `{BaseURL, APIKey (decrypted), Model, Temperature=0}`. Config resolution: **ScorerConfig.judge_config → Project.judge_config → global env default**. All judge calls use `temperature=0` and **structured JSON output** (response_format / function-calling where supported), validated against a schema, with one reparse-retry on malformed output.

### 9.1 Groundedness (reference-free, claim decomposition)

Two judge calls (verifier pluggable so an HHEM classifier can replace call 2 later).

**Call 1 — claim extraction** (system prompt):
```
You extract atomic factual claims from an answer for grounding verification.
Rules:
- One verifiable fact per claim. Split compound sentences.
- Resolve pronouns/references using the question and answer so each claim stands alone.
- Drop non-factual content: greetings, hedges, meta-statements, restatements of the
  question, and subjective opinions. Mark these excluded; do not emit them as claims.
- Do NOT use outside knowledge. Copy facts as asserted by the answer.
Return ONLY JSON matching the schema.
```
User: `{"question": ..., "answer": ...}` → output:
```json
{ "claims": [{"id":"c1","text":"..."}],
  "excluded": [{"text":"...","reason":"greeting|opinion|restatement|hedge"}] }
```

**Call 2 — verification** (system prompt):
```
You judge whether each claim is supported by the retrieved context ONLY.
For each claim assign a verdict:
  "supported"    - directly stated or unambiguously entailed by the context
  "contradicted" - the context asserts the opposite
  "unsupported"  - context is silent / insufficient (treat as NOT grounded)
Cite the id(s) of the supporting/contradicting chunk(s). Use no outside knowledge.
Return ONLY JSON matching the schema.
```
User: `{"context_chunks":[{"id":"k0","text":...}], "claims":[...]}` → output:
```json
{ "verdicts":[{"id":"c1","verdict":"supported|contradicted|unsupported",
               "supporting_chunk_ids":["k0"],"reason":"..."}] }
```

**Aggregation:** `grounded = count(verdict=="supported")`, `total = len(claims)`; `score = grounded/total` (or `1.0` if `total==0` — vacuously grounded). `Details` stores per-claim verdicts + `contradicted_claims`/`unsupported_claims`. Verify each claim against **all** chunks jointly (not chunk-by-chunk).

### 9.2 Correctness (reference-based, G-Eval-style, single call)

**System prompt:**
```
You are a strict grader comparing a generated answer to a reference (gold) answer for a
question. Grade ONLY factual correctness and completeness relative to the reference.
Ignore style, tone, length, and formatting. Do not reward extra content not in the
reference; do not use outside knowledge as ground truth.

Procedure (think step by step, in this order):
1. List the key facts in the REFERENCE answer.
2. For each, mark whether the GENERATED answer states it correctly, contradicts it, or omits it.
3. List any factual claims in the GENERATED answer that CONTRADICT the reference.
4. Assign a score in [0.0, 1.0]:
   1.0 = all reference facts correct, no contradictions
   0.0 = wrong/contradicts, or no reference facts covered
   A contradiction of a key fact caps the score at 0.3 regardless of coverage.
Return ONLY JSON matching the schema.
```
User: `{"question":...,"generated":...,"reference":...}` → output (rationale **before** score):
```json
{ "reference_facts":[{"fact":"...","status":"correct|contradicted|missing"}],
  "contradictions":["..."],
  "reasoning":"...",
  "score": 0.0 }
```
Post-processing: enforce schema (retry on parse failure); optional deterministic cross-check `coverage = correct/len(reference_facts)` — flag for review if `|score-coverage|` exceeds tolerance.

### 9.3 Bias mitigations (baked into defaults)

- `temperature=0`; structured JSON output; rationale-before-score (CoT ordering).
- Anchored rubric levels; conciseness handled by reference-based grading.
- **Cross-family judge recommended** (judge from a different provider than the generator) to curb self-preference — documented, not enforced.
- Prompts are **versioned** (`prompt_template_id`), so score history stays interpretable when a prompt changes.
- Honest framing everywhere: judges are triage + CI gates, ~70–90% precision ceiling.

### 9.4 Prompt templates

Built-in default templates ship in the binary, versioned by id (e.g. `groundedness@v1`). A `ScorerConfig` may set `prompt_template_id` to a stored custom template (Go `text/template`, whitelisted variables). Every `Score` records which template version produced it.

---

## 10. Evaluation modes

### 10.1 Online (traces)
On ingest, for each app scorer in `auto_score_scorers`, if the trace has a scorable span and required inputs are present, enqueue `scoring_task`. **Groundedness runs online** (needs only output+context). **Correctness runs online only when `traces.reference_answer` is set** (via `assay.reference.answer` or attached later); otherwise it's offline. Any scorer is also runnable on-demand over a selected trace set (`POST /v1/traces/score`).

### 10.2 Offline (dataset runs)
`POST /v1/runs` creates an `eval_run`. Modes:
- **`score_existing`** — dataset items already contain outputs (+ context/reference); just score.
- **`generate_then_score`** — call the application's **target endpoint** per item to produce output (+ context), then score.
- **`trace_selection`** — score a selected set of existing traces against a dataset's references (matched by `external_id`/`assay.unit.id`).

**Target endpoint config** (`applications.target_endpoint`, secrets encrypted):
```json
{
  "url": "https://my-rag.example.com/answer",
  "method": "POST",
  "headers": {"Authorization": "Bearer {{secret}}"},
  "request_template": {"query": "{{ item.input.question }}", "top_k": 5},
  "response_mapping": {
    "output": "$.answer",
    "context": "$.sources[*].text"
  },
  "timeout_ms": 30000
}
```
Request body rendered from `request_template` (Go `text/template` over the item); response parsed via **JSONPath** (`response_mapping`) to extract `output` + `context`. Per-item bounded concurrency, per-item timeout, retries; per-item failures recorded, run continues. On completion, compute `aggregates` (`mean`, `pass_rate`, `n` per scorer) and persist.

---

## 11. Security

- **API keys:** `asy_` + 32 bytes base62; stored as **SHA-256 hash** (`key_hash`), plus `key_prefix` for display; shown once. Multiple keys per project, revocable.
- **Admin auth:** management endpoints (create/delete projects, apps, keys, scorer configs, global settings) require `Authorization: Bearer $ASSAY_ADMIN_TOKEN` (from env). Ingest + read endpoints use project API keys.
- **Encryption:** judge API keys and target-endpoint secrets encrypted with **AES-256-GCM**; key from `ASSAY_ENCRYPTION_KEY` (32 bytes, base64). `crypto` package exposes `Encrypt/Decrypt`; secrets never logged, never returned in API responses (write-only fields).
- **Content privacy:** span content capture is client-opt-in; server never requires it.
- **Transport:** TLS terminated by the user's reverse proxy (documented); binary speaks HTTP.

---

## 12. REST API surface (huma v2 → OpenAPI 3.1)

All under `/v1`. Management routes require admin token; data routes accept project API key. huma auto-generates the OpenAPI doc served at `/openapi.json` + docs UI, and validates requests.

```
# health
GET    /healthz            liveness
GET    /readyz             readiness (DB reachable, migrations applied)

# OTLP ingest
POST   /v1/traces          OTLP/HTTP (protobuf|json, gzip)

# projects / keys (admin)
POST   /v1/projects        · GET /v1/projects · GET/PATCH/DELETE /v1/projects/{id}
POST   /v1/projects/{id}/keys   · GET .../keys · DELETE .../keys/{keyId}

# applications
POST   /v1/applications    · GET /v1/applications · GET/PATCH/DELETE /v1/applications/{id}
PATCH  /v1/applications/{id}/endpoint   (set/clear target endpoint; secrets write-only)

# traces / spans (read + on-demand score)
GET    /v1/traces          list (filter by app, time, status, min/max score)
GET    /v1/traces/{id}     trace + span tree + scores
POST   /v1/traces/score    on-demand score a selection {trace_ids[], scorers[]}
PATCH  /v1/traces/{id}/reference   attach a reference answer (enables online correctness)

# datasets
POST   /v1/datasets · GET /v1/datasets · GET/DELETE /v1/datasets/{id}
POST   /v1/datasets/{id}/items       (bulk; JSON) 
POST   /v1/datasets/{id}/import      (CSV/JSONL upload)
GET    /v1/datasets/{id}/items

# scorer configs
GET    /v1/applications/{id}/scorers · PUT /v1/applications/{id}/scorers/{scorer}

# eval runs
POST   /v1/runs            create (mode, dataset_id, scorers, params)
GET    /v1/runs · GET /v1/runs/{id}            (status + aggregates)
GET    /v1/runs/{id}/scores                    (per-item scores)
POST   /v1/runs/{id}/cancel

# scores / aggregates
GET    /v1/scores          query (filter by app, scorer, run, trace, time)
GET    /v1/applications/{id}/metrics           (trends: mean/pass-rate over time per scorer)
```

Errors use RFC 9457 problem+json (huma default). Pagination: cursor-based on `created_at`.

---

## 12.1 Web UI (embedded React SPA)

A minimal, single-user **test UI** now; it grows into the login-capable dashboard later **without changing the deployment shape**.

- **Stack:** React + Vite + TypeScript + **Tailwind + shadcn/ui**. ESM only, exact-pinned versions (verified current-stable when the frontend milestone is scaffolded, same discipline as the Go stack). Lint/format per house style (oxlint/oxfmt); tests with vitest.
- **Visual direction:** take inspiration from [Odysseus](https://github.com/odysseus-dev/odysseus), especially its restrained gray-blue palette, cool-toned atmosphere, and core visual idea. Use it as a reference rather than reproducing it 1:1; Assay should retain its own identity and adapt the design to tracing and evaluation workflows.
- **Brand mark:** use the transparent `assets/assay_gopher.png` for the README and wide brand placements. It pairs the gray-blue assay gopher with a navy Assay wordmark. Derive a square mascot-only variant from the same artwork for the final app icon.
- **Why not Next.js:** the UI is embedded as **static files** in the Go binary. Next.js is a full-stack framework whose value (SSR, server components, API routes) runs on its own Node server — which we don't have, our backend is Go. A static export would discard everything Next adds. A Vite SPA builds straight to embeddable static assets.
- **API client:** generated from the huma **OpenAPI 3.1** spec (`/openapi.json`) via `@hey-api/openapi-ts` into `web/src/api/` — **generated, never hand-edited**; stays in lockstep with the backend contract.
- **Delivery:** `vite build` → static assets embedded via Go `embed.FS`, served by `assayd` at `/` (SPA fallback). No extra container, no separate deploy — the UI rides inside `assayd`. Toggle with `ASSAY_UI_ENABLED`.
- **v1 scope (test-only, single-user):** list applications; list + inspect traces (span tree, attributes, scores + rationales); browse datasets/items; trigger an eval run and watch status/aggregates; view score trends. **Auth in v1 = the admin token entered once and kept in the browser (localStorage)** — deliberately not real login; it's a personal test tool.
- **Future (out of scope now):** real user login (OIDC/password), multi-user/roles, richer dashboards. Since the SPA already speaks the `/v1` API and is versioned independently, adding auth later = API auth + a login view, with no change to how it deploys (still embedded, or optionally split into its own container if SSR is ever wanted).

---

## 13. Python client `assay`

Distribution `assay-sdk` (import `assay`; uv project, ruff/ty/pytest). Built on the OTel SDK;
OTLP/HTTP exporter to `/v1/traces` with the API key header.

```python
import assay

assay.init(
    endpoint="http://localhost:8080",   # base URL; exporter targets {endpoint}/v1/traces
    api_key="asy_...",
    project="my-project",
    application="support-bot",           # -> resource attr assay.application.slug
)

@assay.trace                              # decorator: makes a span, auto-captures I/O
def answer(question: str) -> str:
    with assay.span("retrieval", kind="retrieval") as s:
        chunks = retriever(question)
        s.set_context(chunks)             # -> gen_ai.retrieval.documents + assay.context.chunks.*
    with assay.span("generation", kind="generation", scorable=True) as s:
        out = llm(question, chunks)
        s.set_input(question); s.set_output(out)   # marks the scorable answer
    return out
```

- **`assay.init(...)`** — configures a `TracerProvider` + `BatchSpanProcessor(OTLPSpanExporter(endpoint, headers={"x-api-key": key}))` and sets resource attributes (`assay.application.slug`, `service.name`). Env fallbacks (`ASSAY_ENDPOINT`, `ASSAY_API_KEY`, …).
- **`@assay.trace`** — wraps a function in a span; **auto-captures** args → input and return → output (JSON-serialized, size-capped, default cap configurable), with `capture=False` to disable and `redact=fn` to scrub sensitive fields. Async-aware.
- **`assay.span(name, kind=..., scorable=..., reference=...)`** — context manager; helpers `set_input/set_output/set_context(chunks)/set_reference(text)` write the correct `gen_ai.*` + `assay.*` attributes so users never hand-type conventions.
- **`assay.Client(endpoint, api_key|admin_token)`** — full API client: `projects`, `applications`, `datasets` (create, `import_csv`, `import_jsonl`), `runs` (`create`, `wait`, `scores`), `traces` (list, get, `score`), `scores`, `metrics`. Thin, typed, generated-friendly.

Privacy: capture off by default at the attribute level unless the user opts in per span/decorator (mirrors OTel norm).

---

## 14. Configuration & ops

**Config (env, parsed once via caarlos0/env):**
```
ASSAY_HTTP_ADDR            default :8080
ASSAY_DATABASE_URL         postgres DSN (app pool and goose migrations)
ASSAY_ADMIN_TOKEN          required; guards management endpoints
ASSAY_ENCRYPTION_KEY       required; base64 32 bytes (AES-256-GCM)
ASSAY_JUDGE_BASE_URL       global default judge (OpenAI-compatible)
ASSAY_JUDGE_API_KEY        global default judge key
ASSAY_JUDGE_MODEL          e.g. gpt-4o-mini / llama3.1 / ...
ASSAY_WORKER_CONCURRENCY   default GOMAXPROCS
ASSAY_JOB_MAX_ATTEMPTS     default 3
ASSAY_TRACE_RETENTION_DAYS default 0 (unlimited)
ASSAY_AUTO_CREATE_APPS     default false
ASSAY_LOG_FORMAT           json|text (default json)
ASSAY_UI_ENABLED           serve the embedded web UI at / (default true)
```

**Startup sequence (`cmd/assayd/main.go`):**
1. Parse config (fail fast on missing required).
2. Open `pgxpool` (app) + a `pgx/v5/stdlib` `*sql.DB` (migrations).
3. `goose.NewProvider(Postgres, sqlDB, embeddedFS)` with a `PostgresSessionLocker`; `provider.Up(ctx)` — **migrate before serving**. Fail fast on error.
4. Build `crypto.Cipher`, domain services, worker pool; start worker + reaper goroutines.
5. Mount huma REST + raw OTLP handler on the ServeMux; bind `ASSAY_HTTP_ADDR`.
6. `/readyz` returns 200 only after migrations applied + pool healthy.
7. SIGTERM → graceful drain (§8).

**Docker image:** multi-stage build — (1) `node` stage runs `vite build` for the embedded UI, (2) `go` stage builds `assayd` embedding those assets via `embed.FS`, producing a distroless/static image with a single binary (API + worker + UI).

**Deployment shapes (one binary, one storage engine, two ways to run it):**
- **Standalone / local (default):** `docker compose up` → `assayd` + `postgres:18.6-trixie` (see `docker-compose.yml`; note the PG18 volume path `/var/lib/postgresql`), env from a root `.env`. One command, one file; `docker run`-to-first-trace in < 2 minutes; restart auto-migrates. This is the "single deployable feel" without a second database engine. (Optional convenience: an all-in-one dev image that supervises Postgres + `assayd` in one container — documented as **dev/throwaway only**, never the scaling path.)
- **Scale / "production":** one or more `assayd` containers pointed at a managed/separate Postgres. Because the queue lives in Postgres, worker capacity scales by adding `assayd` replicas (or, later, a worker-only mode of the same binary) with **no code change**. Publish the honest ~1–5M spans/day Postgres ceiling for this segment.

There is deliberately **no SQLite build and no object store** (see §1 storage decision).

**Dogfooding:** `assayd` itself emits OTel traces via `log/slog` + OTel SDK (optional, off by default) — Assay can watch Assay.

---

## 15. CLI + Claude skill

**`assay` CLI** (ship in the Python package; thin wrapper over `assay.Client`):
```
assay projects create|list
assay keys create --project P            # prints the key once
assay apps create|list --project P
assay apps set-endpoint APP --file endpoint.json
assay datasets import APP --file data.csv|.jsonl
assay scorers set APP groundedness --threshold 0.6
assay run create APP --dataset D --scorers groundedness,correctness --mode generate_then_score
assay run watch RUN_ID                    # streams status → aggregates; exit non-zero if pass_rate < gate
assay traces list APP --min-score 0 --scorer groundedness
assay traces get TRACE_ID
assay traces score --scorer correctness TRACE_ID...
assay scores export APP --format jsonl
```
`assay run watch --gate groundedness:0.8` exits non-zero when the aggregate is below the gate → **CI regression gate** in one line.

**Claude Code skill** (`.claude/skills/assay/SKILL.md`): teaches an agent to (1) find failing traces, (2) turn a production failure into a dataset item (regression case), (3) run an eval and read aggregates, (4) gate a change. This is the production→eval loop, agent-operable — a differentiator no competitor ships.

---

## 16. Testing strategy

- **Test behavior, not implementation.** Each test must protect an observable contract, a meaningful error or boundary case, or a known regression. Do not test private call sequences, trivial accessors, or wrapper plumbing merely to increase the test count.
- **No line-coverage target or gate.** Coverage is optional diagnostic evidence for finding suspiciously untested areas, never a quality score, badge target, or merge threshold. Add a test only when it expresses behavior worth preserving.
- **Prioritize risk.** Exercise malformed and empty inputs, boundaries, cancellation, retries, concurrency, partial failures, and secret handling. A smaller suite that catches realistic failures is better than broad shallow assertions.
- **Mock boundaries, not logic.** Use real Postgres for storage and queue semantics, and deterministic fakes only for external HTTP/LLM boundaries or nondeterministic systems. Internal orchestration should be tested through its public outcome.
- **Go unit:** table-driven tests for scorer aggregation, OTLP mapping, attribute extraction, job retry/backoff math, config parsing, crypto round-trip.
- **Go integration:** **testcontainers-go Postgres** for store/queue (real `SKIP LOCKED` concurrency, migrations apply cleanly, reaper returns leases). `httptest` for huma handlers. **Fake OpenAI-compatible server** (httptest) returning canned judge JSON — asserts scorer math without real tokens; also test malformed-JSON retry path.
- **OTLP conformance:** feed real `ExportTraceServiceRequest` payloads (protobuf + JSON + gzip) and assert row mapping + partial_success behavior.
- **Python:** pytest with an **in-memory span exporter** to assert `@assay.trace`/`assay.span` emit the correct `gen_ai.*`/`assay.*` attributes; client tests against a fake backend.
- **Scorer quality (dogfood):** a small gold-set fixture with known groundedness/correctness labels; report judge↔label agreement to guard prompt regressions. Not a unit gate (judge is nondeterministic across models) but a tracked metric.
- **Verify tests catch failures:** for important logic, break the behavior or use targeted mutation testing, confirm the relevant test fails for the expected reason, then restore the implementation.

---

## 17. Repo layout

```
assay/
  README.md  LICENSE (Apache-2.0)  .gitignore
  assets/assay_gopher.png                     # transparent mascot and wordmark
  docker-compose.yml  .env.example
  docs/
    specs/2026-08-26-assay-design.md          # this file
    semantic-conventions.md                    # attribute contract (gen_ai.* + assay.*)
    ci-cd.md                                    # CI/CD plan + pinned versions
  .github/{workflows/,dependabot.yml}          # from docs/ci-cd.md (build task)
  assayd/
    go.mod  Dockerfile  sqlc.yaml
    cmd/assayd/main.go
    db/migrations/*.sql          # goose, embedded
    db/queries/*.sql             # sqlc
    internal/{config,crypto,store,domain,otlp,scoring,worker,api,ui}
    internal/ui/dist/            # vite build output, //go:embed'd (git-ignored, built in CI/Docker)
  web/                           # React + Vite + TS + Tailwind + shadcn/ui (embedded SPA)
    package.json  vite.config.ts  tailwind.config.ts
    src/{main.tsx,App.tsx,routes/,components/,api/}   # api/ = generated from OpenAPI
  clients/python/assay/
    pyproject.toml               # uv; ruff/ty/pytest; entry point `assay` CLI
    src/assay/{__init__.py,trace.py,client.py,cli.py,conventions.py}
    tests/
  .claude/skills/assay/SKILL.md
```

---

## 18. Build roadmap (phased, each phase ends green & demoable)

**M0 — Skeleton & rails** → *verify: `docker compose up` boots, `/readyz` 200, migrations auto-apply.*
- go.mod (Go 1.27), config, pgxpool + stdlib handle, goose embed + Provider.Up on boot, slog, health endpoints, sqlc wired, CI (build/lint/test), Apache-2.0 LICENSE.

**M1 — Domain + auth** → *verify: create project → key → app via API; admin token enforced; key hashing round-trips.*
- projects/apps/api_keys tables + services + huma routes; AES-GCM crypto; OpenAPI served.

**M2 — OTLP ingestion** → *verify: OTel Python SDK exports a trace; it appears via `GET /v1/traces/{id}` with correct span tree + attributes; partial_success works.*
- `/v1/traces` (protobuf+json+gzip), auth, app resolution, ResourceSpans→rows mapping, span partitioning.

**M3 — Scoring engine (offline first)** → *verify: `assay run create ... score_existing` on a seeded dataset yields groundedness + correctness scores with rationales against a fake judge; aggregates computed.*
- scorer interface, judge client, groundedness (2-call) + correctness (1-call), prompt templates, scorer_configs, eval_runs, worker pool + job queue + reaper.

**M4 — Online scoring + generate-then-score** → *verify: auto-score-on-ingest produces groundedness scores on live traces; a `generate_then_score` run calls a target endpoint and scores results.*
- ingest→enqueue path, on-demand `/v1/traces/score`, target-endpoint calling + JSONPath mapping, reference attach.

**M5 — Python client + CLI** → *verify: `@assay.trace` round-trips to a real assayd; CLI creates/imports/runs/watches; `run watch --gate` exits non-zero on failure.*
- `assay.init`/decorator/spans/helpers, `assay.Client`, CLI, in-memory-exporter tests.
- **Completed ahead of M5:** bootstrap the `assay-sdk` distribution at version 0.1.0 and add
  Trusted Publishing so the PyPI project name can be reserved. This bootstrap exposes only the
  `assay` import namespace and version; the functional SDK and CLI remain M5 work.

**M5.5 — Minimal web UI (embedded React SPA)** → *verify: `vite build` output embeds into the binary; visiting `/` lists apps, opens a trace's span tree + scores, and triggers a run + watches aggregates; admin-token stored in-browser; UI toggles off via `ASSAY_UI_ENABLED`.*
- React + Vite + TS + Tailwind + shadcn/ui in `web/`; OpenAPI-generated client; `embed.FS` serving + SPA fallback in `internal/ui`; Docker multi-stage (node build → go embed).

**M6 — Agent-native + polish** → *verify: Claude skill drives a full loop (find failing trace → make regression item → run → gate); metrics endpoint returns trends; retention job prunes old partitions.*
- `.claude/skills/assay/SKILL.md`, `/metrics` trends, retention/TTL job, docs (`semantic-conventions.md`, README quickstart), dogfooding traces.

**Post-v1 (deliberately deferred):** OTLP/gRPC; HHEM local verifier for groundedness; prompt playground/versioning UI; **login-capable multi-user web UI (OIDC/password, roles)** — the v1 UI is a single-user test tool (§12.1); more scorers (answer-relevancy, context precision/recall); local-model cost tracking; SSO/RBAC.

---

## 19. Open questions to resolve during M0–M1

1. UUID v7 app-side generation library vs `gen_random_uuid()` + app v7 — pick one in M0.
2. Exact JSONPath library for Go response mapping (`github.com/ohler55/ojg` vs alternatives) — choose in M4; verify current version then.
3. huma + raw OTLP handler coexistence on one ServeMux — validated in M2 spike (huma via `humago` adapter over the same mux the OTLP handler registers on).
4. Whether `eval_run` fans out to sub-jobs or scores inline — default inline with bounded concurrency (M3); revisit if datasets get large.

---

*This spec reflects decisions confirmed during design: single binary + **Postgres-only** (no SQLite, no object store) with two deployment shapes (standalone compose vs Postgres-backed scale); OTLP/HTTP (gRPC later); OTel GenAI semconv + minimal `assay.*` extensions; continuous 0–1 scores + per-scorer threshold; global→project→scorer judge config; built-in-overridable versioned prompts; claim-decomposition groundedness; auto reference-free online scoring + on-demand for all; keep-all + TTL retention; auto-capturing decorator with redaction; CLI + Claude skill; **minimal embedded React (Vite + Tailwind + shadcn/ui) test UI**, login deferred; Apache-2.0.*
