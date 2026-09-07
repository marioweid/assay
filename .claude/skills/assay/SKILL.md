---
name: assay
description: Drive the Assay eval+tracing platform from the terminal — inspect traces and scores, turn a failing production trace into a regression dataset item, run groundedness/correctness evals, and gate a change on score thresholds. Use when the user mentions Assay, an eval run, groundedness/correctness scoring, a failing/low-scoring trace, or wants to check whether a change regressed answer quality.
---

# Assay

Assay is a self-hosted GenAI eval + tracing platform (one Go binary + Postgres). Use the CLI to
find failed scores, import captured trace evidence into regression datasets, run evaluations,
and gate changes.

## Prerequisites

- Use the CLI from this checkout for M6: prefix commands with
  `uv run --project clients/python/assay` from the repository root.
  A published `assay-sdk` release may not yet contain these commands.
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

To turn a scored trace into a regression item, use an existing dataset in the same application:

```bash
assay scores list <APP_ID> --failed --scorer groundedness
assay datasets from-trace <DATASET_ID> <TRACE_ID> --scorer groundedness
```

Inspect the rationale before selecting a trace. Imports preserve the latest selected scorer's
captured input, output, context, and reference, even after spans expire. Use `--expected-output`
to supply a corrected reference. The external ID contains the trace ID and scorer; repeating the
import returns a conflict and preserves the original item. Import corrected outputs into a new
dataset, or use `generate_then_score` to evaluate a changed target endpoint.

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
```bash
assay traces score --scorer correctness <TRACE_ID> [<TRACE_ID> …]
```

### 6. Export failed evidence and inspect trends

```bash
assay scores export <APP_ID> --failed --format jsonl > failed-scores.jsonl
assay metrics <APP_ID> --scorer groundedness
```

Both use admin authentication. Score export follows every page and writes one JSON object per
line. Metrics and score filters default to 30 days; use `--start` and `--end` with timezone-bearing
timestamps for another range (at most 366 days, start inclusive, end exclusive). Metrics combine
online and offline scores into UTC daily means, pass rates, and counts; missing days are omitted.
Pass rates use the threshold recorded when each score was computed.

## Command reference (subset)

| Command | Purpose |
|---|---|
| `assay apps list` | list applications |
| `assay projects create/list` | manage projects |
| `assay keys create --project P` | create a project API key |
| `assay apps create/list/set-endpoint` | manage applications and generation targets |
| `assay datasets import <APP_ID> --file FILE` | ensure a dataset and import CSV/JSONL cases |
| `assay datasets from-trace <DATASET_ID> <TRACE_ID> --scorer S` | create a case from retained score evidence |
| `assay scores list/export <APP_ID> [--failed] [--scorer S]` | filter scores or export JSONL |
| `assay metrics <APP_ID> [--scorer S]` | daily score trends |
| `assay scorers set <APP_ID> <SCORER> --threshold N` | configure a scorer threshold |
| `assay traces list <APP_ID> [--status STATUS]` | list traces for an application |
| `assay traces get <TRACE_ID>` | full trace: spans, attributes, scores, rationales |
| `assay traces score --scorer S <TRACE_ID>…` | on-demand scoring |
| `assay run create <APP_ID> --dataset D --scorers … --mode …` | start an eval run |
| `assay run watch <RUN_ID> [--gate scorer:threshold]` | watch + gate |

## Guardrails

- Present low scores as signals to investigate and surface the rationale; the number alone does
  not establish that an answer is wrong.
- Prefer **cross-family judges** (a judge model from a different provider than the one that generated the answer) to avoid self-preference bias when configuring scorers.
- Before creating many runs, check the effective judge settings (scorer override, project override,
  then process defaults); runs cost judge tokens. `scorers set` configures thresholds only.
- Correctness needs a reference; include `expected_output` in imported dataset items or run groundedness only.
