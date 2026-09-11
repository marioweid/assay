---
gate: yes
milestone: M7C6
status: final-implementation-review
---

# M7C6 Trace Actions

**Goal.** Add truthful scoring eligibility, reference editing, on-demand scoring, and
score-evidence import to a dataset from trace detail. Keep run/dataset CRUD, comparisons, and all
other trace actions out of M7C6.

**Current facts.** `POST /v1/traces/score` and `PATCH /v1/traces/{id}/reference` already accept admin
credentials, queue transactionally, and preserve project ownership. `BuildTraceScoreInput` is shared
by queueing and the worker, but queueing does not yet validate the resolved judge. The Python
`datasets.from_trace` method currently performs GET dataset + GET trace + POST items itself. The
existing `(dataset_id, external_id)` unique index supplies duplicate protection, online scores retain
captured evidence independently of spans, and trace detail has generated-client/MSW coverage but no
action controls or mutation refresh.

**API contract.** Add admin-only `GET /v1/traces/{id}/scoring-eligibility`, returning both supported
scorers in deterministic order as `{items:[{scorer,eligible,reasons:[{code,message}]}]}`. Reasons are
deterministically ordered and use only `scorer_disabled`, `missing_judge`, `missing_scorable_span`,
`multiple_scorable_spans`, `missing_input`, `missing_output`, `missing_context`, `missing_reference`,
or `malformed_content`; no judge setting or key is returned. Add admin-only
`POST /v1/datasets/{id}/from-trace` with `{trace_id,scorer,expected_output?}`, returning one
`DatasetItemResponse` with 201. It uses the latest persisted `(created_at,id)` score evidence,
`trace:<trace_id>:<scorer>`, and metadata `{trace_id,score_id,scorer}`; an explicit expected output
overrides judged reference. Unknown resources are 404, invalid scorer/content, blank override,
missing evidence, or application mismatch are 422, and duplicate import is 409 without mutation.
Eligibility is informational; `POST /v1/traces/score` revalidates and rejects the whole batch with 422.

**Approach.** Refactor the existing trace-content checks into one structured eligibility evaluator
used by eligibility, queueing, and `BuildTraceScoreInput`; inject the existing evaluation scorer
resolver plus process `JudgeDefaults` into `TraceService` so queueing and the read endpoint agree.
Disabled scorers and invalid/missing URL or model become eligibility reasons, while storage or secret
decryption failures remain fatal; an absent API key alone is valid for a configured local compatible
endpoint. Add one repository transaction that loads dataset, trace, and latest score evidence,
checks same-application ownership, inserts through existing dataset normalization/uniqueness, and
touches the dataset. The UI uses only generated calls, refetches detail and eligibility after
mutations, and reports every failure in the relevant dialog/section. No new architectural pattern or
layer is needed; these extend the existing trace and evaluation services.

**Not doing.**
- No run or dataset CRUD UI, run comparison, trace deletion action, empty “More” menu, or unscored
  import; the ordinary case form already handles unscored traces.
- No schema migration, upsert, duplicate override, bulk import, synchronous judge call, polling
  subsystem, or new dependency.
- No compatibility endpoint or retained client-side Python orchestration; the public Python method
  name stays, but its old multi-request implementation and now-dead helper are deleted.

## Steps

1. Add domain eligibility result/reason types and refactor trace input validation so all stable content
   codes are produced in fixed order; inject the existing scorer resolver/defaults into `TraceService`
   and make queueing fail atomically from that same result — verify:
   `cd assayd && go test ./internal/domain -run 'TraceScoringEligibility|QueueScores|BuildTraceScoreInput'`.
2. Add the evaluation repository/domain import operation and one Postgres transaction selecting the
   latest score without joining spans, enforcing same application, normalizing the item, and relying
   on the existing unique index for 409 — verify:
   `cd assayd && go test -race -count=1 ./internal/store -run 'DatasetItemFromTrace'`.
3. Register both admin-only Huma routes in the existing trace/dataset API files, map their responses
   and errors, add route-level Postgres tests, regenerate sqlc if the import query requires it, then
   regenerate OpenAPI and TypeScript — verify: `cd assayd && go test -race -count=1 ./internal/api -run
   'ScoringEligibility|DatasetItemFromTrace|TraceScore|TraceReference'`; `cd web && pnpm generate:api`.
4. Replace Python `DatasetsResource.from_trace` orchestration with one POST to the new route, preserve
   its signature/return model and CLI behavior, delete `_regression_item`, and update transport tests
   for exact body/error propagation — verify: `cd clients/python/assay && uv run pytest -q
   tests/test_analytics.py tests/test_cli.py tests/test_live_workflow.py`.
5. Extend trace detail with accessible Score and Save-to-dataset dialogs plus a separate inline
   reference editor. Show eligibility reasons, queue only selected eligible scorers, list only the
   current application's existing datasets, offer only latest persisted score choices, allow an
   optional reference override, link success to the dataset, and link score summaries to their
   captured/span evidence; refresh detail and eligibility after reference/score/import success —
   verify: `cd web && pnpm test src/features/traces/traces.test.tsx`.
6. Run generated-file drift checks and all repository gates, including the embedded production UI
   path — verify: every command under “Verification evidence” passes with zero warnings.

## Verification evidence

- Eligibility and queue parity — evidence: the step 1 test exercises every stable reason code,
  multiple simultaneous reasons, disabled/missing judge, process defaults, a local endpoint without
  an API key, and proves the queue accepts exactly the eligibility-valid cases.
- Transactional import — evidence: the step 2/3 tests call the Huma route against Postgres and prove
  admin auth, same-app enforcement, latest `(created_at,id)` evidence, import after span deletion,
  expected-output override/default, exact provenance, 404/422 mapping, and duplicate 409 with the
  original row unchanged.
- Public/generated contract — evidence: `cd assayd && sqlc generate && git diff --exit-code --
  internal/store/sqlc`; `cd web && pnpm generate:api && git diff --exit-code -- openapi.json
  src/api/generated`; generated operations expose only the approved request/response fields.
- Production client paths — evidence: focused Python tests observe one POST and no orchestration
  GETs; focused Vitest/MSW tests invoke generated clients and cover ineligible states, reference save,
  accepted scoring, import success/conflict, application scoping, mutation refresh, and visible errors.
- Repository gates — evidence: `cd assayd && go build ./... && golangci-lint run && go test -race
  -count=1 ./...`; `cd web && pnpm lint && pnpm format:check && pnpm typecheck && pnpm test && pnpm
  build`; `cd clients/python/assay && uv run ruff check . && uv run ruff format --check . && uv run ty
  check && uv run pytest -q`; finally `cd assayd && go test ./internal/ui ./internal/app && go build
  ./cmd/assayd`.

## Risks

- Structured eligibility can drift from execution if parsing remains duplicated; make the shared
  evaluator the only source and prove queue/worker parity before exposing the endpoint.
- Import can accidentally use stale evidence or race ownership checks; order by both timestamp and ID
  and keep all reads, ownership validation, insertion, and dataset touch in one transaction.
- Async scoring does not imply immediate results; show accepted task state and explicit refresh rather
  than adding polling before it is requested.

## Open questions

- None; the API shape, authentication, error semantics, provenance, and deferred scope are approved.

## Implementation and verification

M7C6 Trace Actions is implemented, passed independent review, and passed focused recheck repairs. The
approved scope shipped: trace scoring eligibility, reference/score/save-to-dataset trace-detail
actions, transactional score-evidence import, generated contract/client, and Python one-POST
delegation.

Go verification passed: `go build ./...`, `golangci-lint run`, `go test -race -count=1 ./...`, and
UI/app production checks. Web verification passed: lint, format check, typecheck, full Vitest (139
tests), and build. pnpm emitted known Node 24 engine warnings even though shell Node reports 22.
Generation comparisons for OpenAPI/TypeScript passed, sqlc regenerated successfully, and `git diff
--check` passed.

All normal `uv run` Python gates could not start because the project `.venv` and fresh uv build
executables hit NixOS's dynamic-linker stub. Source lines were manually line-length checked.
M7C5 remains uncommitted alongside M7C6.
