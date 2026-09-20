## Now

- M7D2 is in progress on `feat/m7d-run-review-actions`: run detail loads paginated outcomes and URL-backed direct case evidence while distinguishing zero-valued failed scores from execution failures.

## Next

- Add active-run item polling, rerun/delete actions, and bounded JSONL export.

## Done

- 2026-09-20 M7D1 shipped: admin-scoped run-item lookup, non-null score arrays, set-based page enrichment, repeatable-read consistency, generated OpenAPI/TypeScript, and regressions for source deletion, 101-score boundaries, and concurrent completion.

- 2026-09-11 PR #9 passed Docker-backed PostgreSQL integration tests, Go race detection, frontend production checks, and Python checks in CI.
- 2026-09-11 closed `findings.md`: scoped deletion locking, single-source application loading with error presentation, protected one-time keys, blank expected-output normalization, narrowed docs ignore, and populated migration upgrade coverage.

- 2026-09-11 M7C5 and M7C6 merged to `main` in PR #8 (`41e3a6d`). M7C6 shipped eligibility, reference/score/save-to-dataset trace-detail actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.

- Frontend trace filters now drop `passed` without a valid scorer before serializing or requesting.
- Trace pagination tests use distinct IDs, assert filters on both load-more requests, and verify reset results.
