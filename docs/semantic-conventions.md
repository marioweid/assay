# Assay Semantic Conventions

*The attribute contract for traces Assay ingests. Both the `assay` Python client and any third-party OpenTelemetry emitter (incl. Claude Code) target this. Companion to `docs/specs/2026-08-26-assay-design.md` §7.*

## Principles

1. **OTel `gen_ai.*` is the baseline.** Never invent a key OTel already defines. Everything downstream (Datadog, Grafana, Phoenix) reads these — always co-populate them.
2. **`assay.*` covers only the gaps** OTel doesn't standardize: marking the *scorable* span, grading-specific context extras, and the *reference answer*.
3. **The GenAI semconv is Development-status and moves fast.** Pin the mapping to a spec commit; version it (`ASSAY_SEMCONV_MAPPING_VERSION`); tolerate `OTEL_SEMCONV_STABILITY_OPT_IN`. Treat `gen_ai.response.finish_reasons` as an **array**, not a scalar.
4. **Content is opt-in.** Prompts/outputs/context text are captured only when the client opts in (mirrors OTel's default-off privacy norm). Assay never *requires* content — but scorers need output+context/reference to run.
5. **Attribute typing** follows the OTel attribute type system (string, bool, int, double, and their arrays). OTel attributes cannot hold nested objects, so lists are **flattened with zero-based `<i>` indices** (OpenInference's pattern) or stored as a JSON string in a documented key.

## Application resolution (which app owns the trace)

On ingest, Assay resolves the owning `Application` from **resource attributes**, then validates it against the API key's project:

| Resource attribute | Meaning | Precedence |
|---|---|---|
| `assay.application.slug` | Assay application slug | 1 (preferred) |
| `service.name` | standard OTel service name | 2 (fallback) |

If unresolved and `ASSAY_AUTO_CREATE_APPS=true`, the app is auto-created; otherwise those spans are rejected via OTLP `partial_success`. The API key (header `Authorization: Bearer asy_…` or `x-api-key`) determines the project; the resolved app must belong to it.

## Baseline: `gen_ai.*` attributes Assay reads

**LLM / generation spans** (span name `"{gen_ai.operation.name} {gen_ai.request.model}"`, kind CLIENT):

| Attribute | Type | Notes |
|---|---|---|
| `gen_ai.operation.name` | string | `chat`, `text_completion`, `embeddings`, `retrieval`, `execute_tool`, `invoke_agent`, … |
| `gen_ai.provider.name` | string | `openai`, `anthropic`, `aws.bedrock`, … |
| `gen_ai.request.model` | string | requested model |
| `gen_ai.response.model` | string | responding model |
| `gen_ai.response.finish_reasons` | **string[]** | array — never a scalar |
| `gen_ai.usage.input_tokens` | int | + `cache_read.input_tokens`, `cache_creation.input_tokens` |
| `gen_ai.usage.output_tokens` | int | + `reasoning.output_tokens` |
| `gen_ai.request.{temperature,top_p,top_k,max_tokens,seed,…}` | number | request params |

**Retrieval / RAG spans** (`gen_ai.operation.name = "retrieval"`) — already defined by OTel (Development):

| Attribute | Type | Notes |
|---|---|---|
| `gen_ai.retrieval.query.text` | string | the retrieval query |
| `gen_ai.retrieval.documents` | any | list of retrieved docs (JSON) |
| `gen_ai.data_source.id` | string | source id |
| `gen_ai.embeddings.dimension.count` | int | |

**Tool / agent spans:** `gen_ai.tool.{name,call.id,call.arguments,call.result,type}`, `gen_ai.agent.{id,name,version}`, `gen_ai.workflow.name`.

**Evaluation results** — already defined by OTel; **Assay writes its scores here** so any OTel backend renders them:

| Attribute | Type | Notes |
|---|---|---|
| `gen_ai.evaluation.name` | string | `groundedness`, `answer_correctness` |
| `gen_ai.evaluation.score.value` | double | 0..1 |
| `gen_ai.evaluation.score.label` | string | `pass` / `fail` (after thresholding) |
| `gen_ai.evaluation.explanation` | string | rationale |

## `assay.*` extensions

### (a) Mark the scorable unit — the span the scorers grade

| Attribute | Type | Notes |
|---|---|---|
| `assay.scorable` | bool | `true` on **exactly one** span per eval unit (the answer to grade) |
| `assay.scorable.kind` | string | `rag_answer` \| `generation` \| `agent_final` \| `tool_result` |
| `assay.unit.id` | string | stable eval-unit id (question/row id) — used to match `trace_selection` runs to dataset items |
| `assay.dataset.id` | string | dataset this unit belongs to (optional) |
| `assay.run.id` | string | eval run id (optional; ties to `eval_runs`) |

*Rationale: OTel has no "this is the answer to grade" marker; a boolean lets the ingest/scorer locate the target span deterministically instead of guessing from span kind.*

### (b) Retrieved context (grading extras beyond `gen_ai.retrieval.documents`)

Prefer `gen_ai.retrieval.documents` + `gen_ai.retrieval.query.text`. Add `assay.*` only for grading fields OTel omits, flattened zero-based:

| Attribute | Type | Notes |
|---|---|---|
| `assay.context.chunk.count` | int | number of chunks (also enables quick filtering) |
| `assay.context.chunks.<i>.id` | string | chunk id |
| `assay.context.chunks.<i>.text` | string | chunk text (opt-in capture) |
| `assay.context.chunks.<i>.score` | double | retriever similarity / rerank score |
| `assay.context.chunks.<i>.source` | string | uri / doc id |
| `assay.context.chunks.<i>.used` | bool | set *after* scoring: supported a grounded claim |

*If you'd rather not flatten, put the JSON array in `gen_ai.retrieval.documents` and keep only `assay.context.chunk.count` flat.*

### (c) Reference answer — the real OTel gap

| Attribute | Type | Notes |
|---|---|---|
| `assay.reference.answer` | string | gold answer text (enables **online correctness**) |
| `assay.reference.id` | string | id of the reference record |
| `assay.reference.source` | string | `human` \| `synthetic` \| `curated` |
| `assay.reference.facts` | string | optional: JSON array of pre-extracted key facts |

### (d) Judge audit trail (alongside `gen_ai.evaluation.*`)

| Attribute | Type | Notes |
|---|---|---|
| `assay.evaluation.judge.model` | string | judge model id (self-preference audit) |
| `assay.evaluation.judge.provider` | string | judge provider |
| `assay.evaluation.verifier` | string | `llm` \| `hhem` (groundedness verifier) |

## Span events (drill-down without attribute bloat)

Assay emits per-item scoring detail as **span events** on the scored span (or an evaluator child span):

- `assay.claim.verdict` — groundedness, one per claim: `{id, verdict: supported|contradicted|unsupported, supporting_chunk_ids, reason}`
- `assay.fact.status` — correctness, one per reference fact: `{fact, status: correct|contradicted|missing}`

These mirror the `scores.details` JSONB persisted in Postgres.

## Worked example — a RAG answer trace

Conceptual (attribute keys shown; the `assay` client sets these for you):

```
Trace (resource: assay.application.slug="support-bot")
└─ span "answer"                          # root
   ├─ span "retrieval"    gen_ai.operation.name=retrieval
   │     gen_ai.retrieval.query.text="how do I reset my password?"
   │     assay.context.chunk.count=2
   │     assay.context.chunks.0.id="kb#12"  .text="Go to Settings > Security…"  .score=0.83
   │     assay.context.chunks.1.id="kb#47"  .text="Reset links expire in 1h."   .score=0.71
   └─ span "generation"   gen_ai.operation.name=chat  gen_ai.request.model="gpt-4o-mini"
         gen_ai.usage.input_tokens=812  gen_ai.usage.output_tokens=64
         assay.scorable=true  assay.scorable.kind="rag_answer"
         assay.unit.id="q-001"
         # (optional, enables online correctness:)
         assay.reference.answer="Open Settings > Security and click Reset password."
         # (written back after scoring:)
         gen_ai.evaluation.name="groundedness"  gen_ai.evaluation.score.value=1.0
         gen_ai.evaluation.score.label="pass"   assay.evaluation.judge.model="gpt-4o-mini"
```

## Client helper → attribute mapping (`assay` Python lib)

| Helper | Sets |
|---|---|
| `assay.init(application=…)` | resource `assay.application.slug`, `service.name` |
| `assay.span(kind="retrieval")` + `s.set_context(chunks)` | `gen_ai.operation.name=retrieval`, `gen_ai.retrieval.documents`, `assay.context.chunks.*`, `assay.context.chunk.count` |
| `assay.span(scorable=True)` + `s.set_input/s.set_output` | `assay.scorable=true`, `assay.scorable.kind`, input/output content (opt-in) |
| `s.set_reference(text)` | `assay.reference.answer`, `assay.reference.source` |

Users never hand-type these keys; the helpers own the mapping so the convention can evolve in one place.
