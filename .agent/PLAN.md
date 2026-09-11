## Now

- Post-merge findings are implemented, verified with available local gates, and independently reviewed with no blockers or should-fix items; ready for user review.
- Docker-backed PostgreSQL integration tests remain locally skipped because Docker is unavailable. Go race detection remains blocked because CGO is disabled and no C compiler is installed.
- Preserve the user-owned untracked `Default.merchant-rules-v36.json` and `nul` paths.

## Next

- Run the Docker-backed migration/concurrency tests and Go race suite in CI or once the required local services/toolchain are available.

## Done

- 2026-09-11 closed `findings.md`: scoped deletion locking, single-source application loading with error presentation, protected one-time keys, blank expected-output normalization, narrowed docs ignore, and populated migration upgrade coverage.

- 2026-09-11 M7C5 and M7C6 merged to `main` in PR #8 (`41e3a6d`). M7C6 shipped eligibility, reference/score/save-to-dataset trace-detail actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.

- Frontend trace filters now drop `passed` without a valid scorer before serializing or requesting.
- Trace pagination tests use distinct IDs, assert filters on both load-more requests, and verify reset results.
