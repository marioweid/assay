---
name: assay
description: Drive the Assay eval+tracing platform from the terminal — inspect traces and scores, turn a failing production trace into a regression dataset item, run groundedness/correctness evals, and gate a change on score thresholds. Use when the user mentions Assay, an eval run, groundedness/correctness scoring, a failing/low-scoring trace, or wants to check whether a change regressed answer quality.
---

# Assay

Assay is a self-hosted GenAI eval + tracing platform (one Go binary + Postgres). This skill drives it through the `assay` CLI to close the **production → eval loop**: find a bad answer in real traces, capture it as a regression test, and gate future changes on it.

> Design stub — the CLI is specified in `docs/specs/2026-08-26-assay-design.md` §15. Keep this file in sync with the CLI as it is implemented.

## Prerequisites

- `assay` CLI installed (`pip install assay` / `uv pip install assay`).
- Environment: `ASSAY_ENDPOINT` (e.g. `http://localhost:8080`) and either `ASSAY_API_KEY` (project-scoped, for data ops) or `ASSAY_ADMIN_TOKEN` (for management ops). Confirm with `assay whoami` before acting.
- Never print full API keys / admin tokens in output. Refer to keys by their `asy_ab12…` prefix.

## Core workflows

### 1. Triage: find low-scoring or failing traces
```
assay traces list <APP> --scorer groundedness --max-score 0.5 --since 24h
assay traces get <TRACE_ID>          # span tree, attributes, scores + rationales
```
Read the scorer `rationale` and the per-claim/per-fact `details` to explain *why* it scored low (unsupported claims → retrieval gap; contradictions → hallucination or stale context).

### 2. Capture a regression: production trace → dataset item
When a trace is a genuine failure worth locking in:
```
assay datasets ensure <APP> --name regressions
assay traces to-item <TRACE_ID> --dataset regressions \
    --reference "<the correct answer>"        # sets expected_output; input+context pulled from the trace
```
This makes the failure a permanent, re-runnable test case (`assay.unit.id` preserved for matching).

### 3. Evaluate
```
# score an existing dataset (outputs already present)
assay run create <APP> --dataset regressions --scorers groundedness,correctness --mode score_existing
# or generate fresh answers by calling the app's target endpoint, then score
assay run create <APP> --dataset regressions --scorers groundedness,correctness --mode generate_then_score
assay run watch <RUN_ID>                       # streams status → per-scorer aggregates (mean, pass_rate, n)
```

### 4. Gate a change (CI / pre-merge)
```
assay run watch <RUN_ID> --gate groundedness:0.8 --gate correctness:0.7
# exits non-zero if any aggregate is below its gate — use as a CI/pre-commit check
```

### 5. On-demand re-score (e.g. after tuning a scorer prompt or judge model)
```
assay traces score --scorer correctness <TRACE_ID> [<TRACE_ID> …]
```

## Command reference (subset)

| Command | Purpose |
|---|---|
| `assay whoami` | show endpoint + which token/key is active (prefix only) |
| `assay apps list` | list applications |
| `assay traces list <APP> [--scorer S --min-score/--max-score N --since D --status]` | filter traces |
| `assay traces get <TRACE_ID>` | full trace: spans, attributes, scores, rationales |
| `assay traces score --scorer S <TRACE_ID>…` | on-demand scoring |
| `assay traces to-item <TRACE_ID> --dataset D --reference "…"` | regression capture |
| `assay datasets ensure/import <APP> …` | manage datasets (CSV/JSONL import) |
| `assay run create <APP> --dataset D --scorers … --mode …` | start an eval run |
| `assay run watch <RUN_ID> [--gate scorer:threshold]` | watch + gate |
| `assay scores export <APP> --format jsonl` | export scores |

## Guardrails

- **Judges are triage + gates, not oracle truth** (≈70–90% precision). Present low scores as *signals to investigate*, and always surface the rationale — never assert a trace is "wrong" on the number alone.
- Prefer **cross-family judges** (a judge model from a different provider than the one that generated the answer) to avoid self-preference bias when configuring scorers.
- Before creating many runs, confirm the judge config (`assay scorers set …`) — runs cost judge tokens.
- Correctness needs a reference; if a trace has none, either attach one (`assay traces to-item … --reference`) or run groundedness only.
