---
name: assay
description: Drive the Assay eval+tracing platform from the terminal — inspect traces and scores, turn a failing production trace into a regression dataset item, run groundedness/correctness evals, and gate a change on score thresholds. Use when the user mentions Assay, an eval run, groundedness/correctness scoring, a failing/low-scoring trace, or wants to check whether a change regressed answer quality.
---

# Assay

Assay is a self-hosted GenAI eval + tracing platform (one Go binary + Postgres). This skill drives the implemented M5 CLI to inspect traces, import regression datasets, run evaluations, and gate changes.

> This skill currently documents the M5 CLI subset. The production-trace-to-dataset workflow and score export/filter commands remain M6 work.

## Prerequisites

- `assay` CLI installed (`uv tool install assay-sdk`).
- Environment: `ASSAY_ENDPOINT` (e.g. `http://localhost:8080`) and either `ASSAY_API_KEY` (project-scoped, for trace ops) or `ASSAY_ADMIN_TOKEN` (for management and evaluation ops).
- Never print full API keys / admin tokens in output. Refer to keys by their `asy_ab12…` prefix.

## Core workflows

### 1. Inspect traces
```
assay traces list <APP_ID> --status error
assay traces get <TRACE_ID>          # span tree, attributes, scores + rationales
```
Read the scorer `rationale` and the per-claim/per-fact `details` to explain why it scored low.

### 2. Import a regression dataset
```
assay datasets import <APP_ID> --name regressions --file regression.jsonl
```

### 3. Evaluate
```
# score an existing dataset (outputs already present)
assay run create <APP_ID> --dataset <DATASET_ID> --scorers groundedness,correctness --mode score_existing
# or generate fresh answers by calling the app's target endpoint, then score
assay run create <APP_ID> --dataset <DATASET_ID> --scorers groundedness,correctness --mode generate_then_score
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
| `assay apps list` | list applications |
| `assay projects create/list` | manage projects |
| `assay keys create --project P` | create a project API key |
| `assay apps create/list/set-endpoint` | manage applications and generation targets |
| `assay datasets import <APP_ID> --file FILE` | ensure a dataset and import CSV/JSONL cases |
| `assay scorers set <APP_ID> <SCORER> --threshold N` | configure a scorer threshold |
| `assay traces list <APP_ID> [--status STATUS]` | list traces for an application |
| `assay traces get <TRACE_ID>` | full trace: spans, attributes, scores, rationales |
| `assay traces score --scorer S <TRACE_ID>…` | on-demand scoring |
| `assay run create <APP_ID> --dataset D --scorers … --mode …` | start an eval run |
| `assay run watch <RUN_ID> [--gate scorer:threshold]` | watch + gate |

## Guardrails

- **Judges are triage + gates, not oracle truth** (≈70–90% precision). Present low scores as *signals to investigate*, and always surface the rationale — never assert a trace is "wrong" on the number alone.
- Prefer **cross-family judges** (a judge model from a different provider than the one that generated the answer) to avoid self-preference bias when configuring scorers.
- Before creating many runs, confirm the judge config (`assay scorers set …`) — runs cost judge tokens.
- Correctness needs a reference; include `expected_output` in imported dataset items or run groundedness only.
