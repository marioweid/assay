# Session explorer and conversation-first UI — implementation plan

**Status:** Implemented on `feat/session-explorer-ui` (2026-09-27), pending manual
browser/visual signoff and explicit rollout approval. The approved standalone preview files were removed at
the user's request. Root-tagged session reads, generated clients, Sessions list/detail and
source inspector, task-local Python SDK session context, signed-cookie demo chat and a protected
offline-evaluation endpoint are in the branch. Trace detail leads with captured conversation.
The frontend passed synthetic and **isolated database-backed** embedded-browser acceptance;
the chat service passed signed-cookie, two-client, persisted-history, model-context and
service-recreation checks against disposable Assay/PostgreSQL. Actual deployed-chat process
restart and two-browser visual behavior and final human visual review are not yet verified. The user started Docker; the persisted Assay/Postgres containers auto-resumed and
were stopped with permission. Their named volumes were retained, **no migration was run on persistent data**, and the
isolated acceptance stack cleaned up. No SDK release was published.

## Visual fidelity correction — pending human signoff

The user-requested comparison recovered the original preview HTML and confirmed structural
and styling drift. The follow-up now restores recent sessions, a continuous conversation,
and the inspector together on desktop; clickable role-colored bubbles; summary chips;
duration/token tiles; execution/context evidence; and timing beneath the conversation.
The shell uses the approved midnight palette, system typography and spacing. Existing
application/theme/disconnect controls remain real controls, with aligned responsive sizing;
a global `font: inherit` rule had overridden button text sizes and now sets only font family.
The obsolete Plex Sans dependency is removed. Mobile stacks the inspector below the thread
and retains navigation through the drawer and All sessions link. No session activity,
scores or semantic titles are invented; partial counts explicitly say loaded.

97 focused component/unit tests and eight synthetic Playwright/axe checks pass, including
red/green composition assertions, 320–1440px toolbar bounds, long names/system theme,
320/390/1440px panel placement, both themes, keyboard timing zoom and navigation,
empty/error/retry and long messages. Lint, formatting, typecheck and build pass. Added
regressions cover scope-switch selection reset, partial counts, recent-list retry and rich
message controls outside the selection button. A read-only independent review failed at
its usage limit with no report; self-review is complete, not independent approval.

Reference: `%TEMP%/assay-approved-mockup-review.html`; before/after synthetic screenshots:
`%TEMP%/assay-ui-comparison/` and `%TEMP%/assay-ui-corrected/`. The deleted repository mockup
folder remains deleted. No persistent service/data changed. Human visual signoff, deployed
chat/browser checks and persistent-rollout approval remain separate gates.

## Safe frontend preview without Docker

From `web/`, run:

```bash
node_modules/.bin/playwright test --config playwright.sessions.config.ts
```

This launches a temporary Vite server on `127.0.0.1:5271`, intercepts **synthetic** Assay API
responses, and stops the server afterward. It exercises Sessions list/detail, source scores,
model/tool/context inspector, transcript isolation, zoomable timeline, keyboard scrolling,
empty/error/retry states, long text and IDs, light/dark themes, mobile/desktop and axe. It uses
no credentials or persistent stack and stores no preview screenshots or traces. To inspect the
synthetic UI interactively, run the same Playwright command with `--debug` and
`--grep "1440px in dark mode"`; use the Playwright Inspector to advance through the list,
conversation, source inspector and timeline. For component checks run
`node_modules/.bin/vitest run src/features/sessions` from `web/`. Neither command
proves migration, PostgreSQL isolation, signed-cookie integration or actual chat persistence.
Run those against **disposable** PostgreSQL only after the separate database gate; never use the
persisted local chat/Assay Compose project for this branch without explicit approval.

## Disposable database-backed acceptance

With Docker running and the persisted containers stopped, run `./tests/acceptance/run.sh` from
the repository root (`pnpm`, `uv` and Node must be on `PATH`). It creates a uniquely named
PostgreSQL/Assay/fake-judge stack, tests the **embedded** production UI against real API reads,
and removes its own containers, images, volumes and temporary credentials afterward. On this
branch all 10 browser checks (including a real two-turn, opaque-ID Sessions flow), docs smoke,
SDK product acceptance and a demo-chat TestClient acceptance check passed. The chat check
uses a fake external model but real Assay, PostgreSQL and signed cookies; it covers context
across turns, two independent cookie jars, service recreation and New Chat rotation. The
synthetic browser checks remain separate; they do not count as database acceptance. Disposable PostgreSQL migration/backfill, project isolation,
late-root and deletion checks passed with `CI=1 go test -p 1 ./... -count=1` in `assayd/`.
A 100k-trace fixture measured session summaries at about 19 ms; recent, first-page and late
cursor reads in a 10k-turn session used the start-time index, including a prepared generic
plan (~0.03 ms for the measured late page). This is fixture evidence, **not** a latency promise
for production-size histories. The migration holds an exclusive lock during backfill/index
creation; size its runtime/WAL on representative disposable data and approve a maintenance
window before touching persistent volumes. A deployed chat process/browser restart and
human review of populated UI remain separate gates. Windows-local Go race checks require a
CGO toolchain and were not run; ordinary Go tests, vet and build passed.

## Outcome and decisions

- **Traces remains complete:** tagged and untagged traces remain in the existing view. A new
  **Sessions** view shows only traces explicitly tagged with `session.id`; never infer a session
  from matching text, a user ID, or proximity in time. Existing data is not rewritten.
- The example chat's session lasts across refresh/reopen and changes only on **New Chat**. Its
  backend issues the session ID and returns a separate, signed first-party cookie binding this
  browser to its session. It also issues a stable, anonymous browser ID. These IDs correlate
  telemetry, **not verified human identity**; the cookie is a bearer capability for reading
  that browser's demo transcript and needs its own privacy/security checks.
- Adopt OpenTelemetry `session.id` for grouping spans/traces into a user interaction session.
  For this chat, also record `gen_ai.conversation.id` with the same value on GenAI spans because
  one chat session is one conversation. Other clients may have conversations with different
  lifetimes; do not silently equate these two attributes there. Record the browser identifier as
  `enduser.pseudo.id`, **not** an authenticated `user.id`. A trusted auth integration can set
  `user.id` later. IDs are linkable/sensitive metadata and need capture/privacy guidance.
- Dark-first **midnight slate + teal** is the agreed visual direction. Retain the existing light
  and system options, but make the default dark. The user approved the sample Sessions design
  on desktop/mobile; finalize other states during implementation before locking visual baselines.
  The example chat and Assay share a visual language, not a backend or authentication boundary.
- No new charting library or session table in the first slice. Reuse the existing timing layout
  and message parser; introduce storage/indexes only where needed for query correctness and
  measured scale. Keep the existing trace and score/evaluation workflows available.

OTel references: [session attributes](https://opentelemetry.io/docs/specs/semconv/registry/attributes/session/),
[GenAI conversation ID](https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/gen-ai-spans.md),
[pseudonymous end-user ID](https://opentelemetry.io/docs/specs/semconv/registry/attributes/enduser/).
These conventions are still marked Development: use their canonical keys without claiming their
names/semantics are permanently stable. Session ID is an attribute, **not** an OTel trace ID:
each turn remains its own trace, and one session correlates multiple traces.

## Incremental delivery

### 0. Approve the interaction and visual design

- Produce reviewable desktop and mobile screens for Sessions list, a multi-turn session, clicked
  trace/message detail, Gantt mode, and empty/missing-capture/error states. Include both themes.
- In the session view, show one user/assistant pair per turn (not the model's repeated history).
  Clicking either message selects its source trace; a side panel presents model/tool spans,
  captured context, token/timing/score information, and a deep link to full Trace detail.
- Make session timeline and per-trace waterfall alternate views of **the same** underlying
  timing data, with a readable time axis, nested/overlapping bars and text duration summaries.
  Never turn context chunks into chat messages.
- Gate: the sample Sessions hierarchy and palette were approved on desktop/mobile. Confirm
  remaining empty/error and light-theme states before broad UI refactoring; retain the old trace
  route for bookmarked links.

### 1. Persist and query explicit session membership

- On ingest, accept `session.id` only from the trace's root span (document this requirement for
  raw OTLP senders); partial OTLP exports can acquire a root later. Preserve existing traces
  without the attribute and keep child attributes intact. Validate IDs as bounded nonblank
  opaque strings; avoid accidental PII in URL/search/logs.
- Add a nullable indexed session ID projection on the trace row, backfilled from **existing
  root attributes** by a non-destructive migration or generated expression. Re-ingest/summary
  updates must keep membership correct. Scope all reads by project **and** application before
  comparing a session ID; matching IDs in other projects must never be visible.
- Add cursor-paginated session summaries ordered by latest activity, and paginated member turns
  ordered chronologically with deterministic ID tie-breaking. Return totals/time window, trace
  summaries and the root turn's captured messages, not entire span histories; get individual
  details through existing trace API. The chat backend must be able to fetch its last 19
  persisted turns so its model context survives refresh/reopen without trusting client-provided
  history. A session with no captured messages remains visible with a clear reason.
- Extend generated OpenAPI/TypeScript and Python management-client APIs, not UI-only private
  endpoints. Test duplicate IDs across apps/projects, late root spans, out-of-order exports,
  deletion, pagination boundaries, new turns during pagination, and untagged legacy traces.
- Gate: two sessions plus one untagged trace appear only in the right lists; wrong-project/app
  requests cannot enumerate or read another project's session. Query plan for large fixture is
  recorded before adding further indexes/materialization.

### 2. Add scoped Python SDK session context

- Offer a typed context manager, e.g.
  `with assay.session(session_id, pseudonymous_user_id=None, conversation_id=None):`, that
  attaches `session.id` to **new** `assay.span`/`@assay.trace` spans in that request. When
  explicitly supplied, attach `gen_ai.conversation.id` to GenAI work. Names and arguments are
  contract candidates, not finalized signatures; no bare `user_id` implying trusted identity.
- Context must be per-request and async-safe (no process-global active session); nested contexts
  restore the previous value, exceptions clean up, and independent concurrent calls cannot leak
  IDs. Do not automatically put identities in propagating baggage or a resource attribute.
- Provide a separate explicit pseudonymous-user helper/parameter for `enduser.pseudo.id`; do
  not silently convert a browser-provided string into trusted `user.id`. Document capture
  controls, redaction and implications of linking sessions.
- Gate: concurrent async requests with different sessions, nested scopes, missing context,
  decorated sync/async functions, and flush/export all assert correct attributes. No session
  helper may change scorable behavior or add a second auto-scoring span.

### 3. Wire the example chat end to end

- `GET /api/session` creates/resumes backend-issued IDs and returns the current session ID plus
  the persisted, paginated transcript for this cookie's session. `POST /api/session` (New Chat)
  rotates only the session ID. Use a separate **signed, HttpOnly, SameSite** browser capability
  cookie, not a raw public `session.id` cookie: otherwise anyone who learns a session ID from
  an Assay link could request its transcript. A dedicated persistent chat signing secret is
  required; use `Secure` behind HTTPS, a bounded cookie lifetime, and same-origin mutation
  checks. The anonymous browser ID is separate from the rotating session. Document that
  stateless signed capabilities remain usable until expiry if copied before New Chat; stronger
  revocation or real user identity requires a server-side store/authentication later.
- `/api/chat` derives identity only from the verified cookie. It accepts the **latest** user
  message, then loads recent persisted turns for model context, rather than trusting a JSON
  session/user ID or client-supplied history. A root turn span records **only the current user
  message and its assistant reply**. Child generation spans may contain bounded full model
  history for debugging; session UI must not repeat that history as new turns. Preserve trace
  links and general-question scoring eligibility rules.
- New Chat clears local visible history and rotates the server session before sending another
  turn. Refresh/reopen restores visible history and model context from Assay's session reads,
  paginating the transcript if long. A failed send must not create a ghost assistant reply;
  a persisted error trace can remain visible as an error turn. If a cookie is
  lost/expired, a new session starts; existing traces remain in Assay and visible to its admin.
- Gate: two turns share one session, reload/reopen restores them and keeps the model context,
  New Chat rotates the session but preserves the pseudonymous browser ID, and two browser
  contexts stay isolated. A different browser cannot fetch a transcript by supplying a known
  session ID or forged cookie; no provider call is made to create a session. General and Assay
  questions both yield source-linked turns.

### 4. Build Sessions, improve Trace detail, and align the two UIs

- Add Sessions navigation and deep-linkable list/detail routes beside Traces. Render paired
  messages in an accessible transcript with clear speaker alignment, collapsible tools/context,
  error and score states, and a selected-trace inspector. The backend supplies stable turn
  order; no client-side collection of every trace/page to reconstruct sessions.
- Reuse `conversation-model.ts`, `message-content.tsx`, `span-timing.ts` and the existing trace
  waterfall. Refine Trace detail so user/assistant content leads; source, retrieval evidence,
  events/raw JSON and score details are progressively disclosed instead of competing equal boxes.
  Preserve missing/malformed/empty capture diagnostics and non-GenAI trace readability.
- Provide a session-level Gantt/waterfall across turns plus a trace-level nested waterfall.
  Bars use actual timestamps (parallel spans overlap), visible time ticks, duration labels,
  scroll/zoom for long sessions, keyboard-selectable rows and a text/table alternative. Use a
  lightweight SVG/CSS implementation until measurements justify a chart dependency.
- Refresh shared Assay shell, typography, spacing, elevation and semantic color tokens with the
  approved dark palette; preserve light/system theme selection and check contrast. Bring the
  separate example chat stylesheet into visual alignment without importing broad demo CSS into
  the React app or exposing Assay admin credentials there.
- Gate: keyboard/screen-reader navigation, focus/selection, 360px and 1440px, 200% zoom,
  reduced motion, long prompts/IDs, tool calls, malformed capture and overlapping timing all
  remain usable. Deep links and existing trace actions/scores continue working.

### 5. Acceptance and rollout

- Run the existing isolated Compose/fake-judge/Playwright acceptance with **synthetic**
  multi-turn/multi-session OTLP data; no paid model or personal chat content. Test privacy and
  project isolation at API and UI levels, two independent browsers, restart persistence,
  trace↔session navigation, and untagged legacy data.
- Run focused Go tests/integration/race checks, SQL generation and migrations, OpenAPI/client
  drift, Python SDK pytest/Ruff/ty/package checks, web Vitest/oxlint/oxfmt/tsc/build,
  Playwright/axe both themes and real embedded-build smoke. Measure session list/detail on
  large fixtures and virtualize only if observed. Human-approve screenshots before baselines.
- Record compatibility for published `assay-sdk==0.3.0`: the new helper exists only in the
  checkout until a separately approved SDK release. For development, build the chat against
  the checkout SDK; before shipping, pin it to the verified new release rather than adding a
  dual SDK compatibility path. Keep the running local project and Docker volumes untouched;
  any publication, rollout, or migration on persistent data needs approval.

## Current code and sequencing notes

- OTLP mapping already merges resource/span attrs into each span and mirrors the root onto
  `traces.attributes`: `assayd/internal/otlp/map.go` and
  `assayd/db/queries/traces.sql`. This branch adds the session projection and endpoints.
- The SDK has a private tracer provider and explicit `span`/`trace` helpers in
  `clients/python/assay/src/assay/tracing.py`. Context must be added there, not by a mutable
  global default. Server list APIs use cursor pagination; follow project-scoped auth patterns.
- `web/src/features/traces/conversation-model.ts`, `trace-detail.tsx` and
  `span-waterfall.tsx` parse messages and show selectable timing. The app retains light/system
  themes with dark as the new default; the separate demo chat now shares its visual language.
- This explicitly **extends** earlier M7 plans that excluded cross-trace sessions and defers
  E4's screenshot lock until final visual acceptance. It does not imply a real login system or
  change existing trace retention, scoring, or dataset semantics.
