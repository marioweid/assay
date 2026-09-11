## Now

- M7C6 Trace Actions is implemented, independently reviewed, and repaired in focused recheck; ready for user review and commit. Python `uv run` gates were blocked by the NixOS dynamic-linker stub affecting the project `.venv` and fresh uv build executables; source lines were manually line-length checked.
- M7C5 remains uncommitted alongside M7C6.

## Next

- Select the next scoped milestone.

## Done

- 2026-09-11 M7C6 Trace Actions shipped: eligibility, reference/score/save-to-dataset trace-detail actions, transactional score-evidence import, generated contract/client, and Python one-POST delegation.

- Frontend trace filters now drop `passed` without a valid scorer before serializing or requesting.
- Trace pagination tests use distinct IDs, assert filters on both load-more requests, and verify reset results.
