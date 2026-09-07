# M5.5 Embedded Web UI Design

**Status:** Approved design, 2026-09-01

**Source:** This document refines M5.5 from
[`2026-08-26-assay-design.md`](2026-08-26-assay-design.md). Where the older document assigns score
trends to both M5.5 and M6, this design keeps the metrics API and trend charts in M6.

## 1. Goal

M5.5 adds a minimal single-user web interface to the existing `assayd` binary. The interface lets a
developer enter the admin token, select an application, inspect traces and scores, browse datasets,
and create and watch evaluation runs. Vite builds static files that the Go binary embeds and serves;
the deployment remains one `assayd` binary plus Postgres.

The milestone is complete when one production binary can:

- Serve the SPA and preserve direct route navigation across refreshes.
- Validate an admin token and list applications.
- Use that admin token to list and inspect application traces.
- Browse datasets and their items.
- Create, watch, and cancel evaluation runs and show their aggregates.
- Disable all UI routes with `ASSAY_UI_ENABLED=false` without affecting API, OpenAPI, docs, or
  health routes.
- Remain usable on desktop and mobile widths.

## 2. Scope

### In scope

- Application list and application-first navigation.
- Trace list, cursor pagination, span tree, attributes, events, scores, rationales, and score details.
- Dataset list, dataset detail, and cursor-paginated item inspection.
- Evaluation run list, creation, live status and aggregate polling, detail, and cancellation.
- Admin-token connection gate and explicit disconnect.
- Admin read access for trace list and detail endpoints.
- Generated TypeScript API client and deterministic OpenAPI drift checks.
- Embedded static asset serving, SPA fallback, cache headers, and security headers.
- Frontend and Go tests, frontend CI, and a multi-stage production image.

### Out of scope

- Score trend charts, `/v1/scores`, and application metrics; these remain M6.
- Project, application, key, dataset, or dataset-item creation and editing forms.
- Trace reference mutation and on-demand trace scoring in the UI.
- Real login, users, sessions, OIDC, roles, or multi-tenant browser authorization.
- Dark mode, saved views, configurable dashboards, and live streaming transports.
- A separately deployed frontend or Node server at runtime.

## 3. Product Structure

The interface is an application workspace rather than a global dashboard. URLs are the source of
navigation state:

| Route | Purpose |
|---|---|
| `/` | Redirect to `/apps` after authentication, otherwise show the connection gate |
| `/apps` | List applications |
| `/apps/:appId/traces` | List traces for one application |
| `/apps/:appId/traces/:traceId` | Inspect one trace and its span tree |
| `/apps/:appId/datasets` | List datasets for one application |
| `/apps/:appId/datasets/:datasetId` | Browse one dataset and its items |
| `/apps/:appId/runs` | List evaluation runs for one application |
| `/apps/:appId/runs/:runId` | Watch one run and inspect aggregates |

React Router provides route matching, deep links, not-found handling, and navigation. The URL holds
the selected application and resource identifiers; these values are not duplicated in global state.

After authentication, the desktop shell has a persistent left rail containing the mascot,
application switcher, and Traces, Datasets, and Runs navigation. On small screens the same navigation
uses a drawer and route content becomes one column.

## 4. Screens

### Connection gate

The first unauthenticated view contains the Assay mark, one password-style admin-token field, a
Connect action, and a concise explanation that this is a local single-user interface. Connect calls
`GET /v1/applications`. A successful response stores the token and opens the originally requested
route, or `/apps` when there was no protected destination. A 401 leaves the token field in place with
an inline error. The token is never included in the error text.

### Applications

Applications use a compact table with name, slug, project identifier, automatic scorers, and target
endpoint state. Selecting a row opens its trace workspace. Empty state text points users to the CLI
or API because application creation is outside this UI milestone.

### Traces

The trace table shows start time, root operation, status, duration, span count, total tokens, and
available score badges. The list always sends `application_id`; cursor pagination uses an explicit
Load more action and rejects repeated cursors.

Trace detail is a split workbench. The left pane is a collapsible span tree. The right pane has
Overview, Attributes, Events, and Scores tabs for the selected span, plus trace-level scores when no
span is selected. Score rows display scorer, value, threshold, pass/fail text, rationale, judge model,
and structured details. Color reinforces but never replaces pass/fail text.

### Datasets

The dataset list shows name, description, and update time. Dataset detail pages inspect input,
output, expected output, context chunks, and metadata for each case. Large JSON values use a
read-only formatted viewer with wrapping and copy controls. Items use cursor pagination.

### Runs

The run list shows name, dataset, mode, status, progress counts, and available aggregate summaries.
Run evaluation opens a dialog with run name, dataset, mode, and groundedness/correctness checkboxes.
The current application is fixed by the route. At least one scorer is required.

Run detail polls once per second while queued or running. It displays total, succeeded, failed, and
canceled item counts plus mean, pass rate, and sample size for each scorer. Polling stops at
succeeded, failed, or canceled. An active run exposes Cancel with a confirmation dialog.

## 5. Visual Language

The UI is a calm technical workbench inspired by Odysseus without copying its layout. It uses a
light cool gray-blue canvas, navy navigation, blue-gray surfaces, thin separators, compact controls,
and restrained semantic accents. Self-hosted IBM Plex Sans is the primary UI face; identifiers and
JSON use self-hosted IBM Plex Mono. The font packages are exact-pinned with the other dependencies.

The design avoids card grids, gradients, oversized marketing headings, decorative charts, and large
empty regions. Tables and split panes are preferred because the primary work is comparison and
inspection. The square app icon is a mascot-only crop derived from `assets/assay_gopher.png`; wide
brand placements continue to use the existing image.

Every route defines a loading skeleton, an empty state with a next action, a retryable error state,
and a not-found state. Tables remain semantic and become horizontally scrollable on narrow screens.
Split panes stack vertically below the tablet breakpoint.

## 6. Frontend Architecture

The frontend is an ESM-only Node 22 project under `web/`:

```text
web/
  package.json
  pnpm-lock.yaml
  tsconfig.json
  vite.config.ts
  openapi.json
  src/
    main.tsx
    app/router.tsx
    api/client.ts
    api/errors.ts
    api/generated/          # generated and committed; never hand-edited
    auth/auth-context.tsx
    components/app-shell.tsx
    components/problem-state.tsx
    components/score-result.tsx
    features/applications/
    features/traces/
    features/datasets/
    features/runs/
    test/server.ts
```

The implementation uses React, Vite, TypeScript, Tailwind, shadcn/ui primitives, and React Router.
It does not add TanStack Query. Page-level hooks own requests, abort controllers, pagination, and the
single run-polling timer. Shared code is limited to auth, API error normalization, and components
used by multiple features.

TypeScript enables the repository strictness baseline: `strict`, `noUncheckedIndexedAccess`,
`exactOptionalPropertyTypes`, `noImplicitOverride`, `noPropertyAccessFromIndexSignature`,
`verbatimModuleSyntax`, and `isolatedModules`. Runtime and development dependencies are exact pinned.
Before implementation, current stable versions are resolved from authoritative package sources.

## 7. OpenAPI And Client Generation

`assayd/cmd/openapi` writes a deterministic OpenAPI 3.1 document to `web/openapi.json` without
opening Postgres or starting an HTTP server. API route registration becomes unconditional so the
document never changes based on optional runtime collaborators; production already supplies all
services. Request handlers may hold nil collaborators during document generation because no request
is executed.

`pnpm generate:api` runs the Go document command and then `@hey-api/openapi-ts`, writing
`web/src/api/generated/`. Both the snapshot and generated client are committed. CI reruns generation
and fails when `git diff --exit-code -- web/openapi.json web/src/api/generated` detects drift.

`web/src/api/client.ts` is the only handwritten transport adapter. It configures same-origin base
URLs, obtains the current token from auth context, and sends `Authorization: Bearer <token>`.
`web/src/api/errors.ts` maps RFC 9457 responses into operation, status, title, and detail without
including request headers or bodies.

## 8. Authentication Adjustment

Management, dataset, scorer, and run endpoints already accept the admin bearer token. M5.5 also
allows that token on these read-only trace operations:

- `GET /v1/traces`
- `GET /v1/traces/{id}`

Project API keys remain valid and preserve their existing project scope. Trace ingestion, reference
updates, and scoring mutations remain project-key-only.

For admin trace lists, `application_id` is required. The handler loads the application through the
admin service, resolves its project, and uses the existing project-scoped list query. This avoids a
new global trace-list query. For admin trace detail, the trace service adds an explicitly unscoped
admin read that loads a trace by ID and enriches it with the same spans, scores, and scoring tasks as
the project-scoped read. The repository's global trace lookup is named for its actual behavior and
shared by admin presentation and worker scoring rather than exposing a second duplicate query.

OpenAPI declares `adminBearer` as an alternative security scheme for these two reads and documents
the conditional `application_id` requirement. Unauthorized and cross-project project-key behavior
does not change.

## 9. Browser Security And Request Lifecycle

The admin token is stored only under the versioned key `assay.admin-token.v1` in `localStorage`.
Disconnect deletes it. It is never placed in route state, query strings, rendered output, telemetry,
or logs. A 401 deletes the stored value and returns to the connection gate.

The UI handler sets these headers on SPA HTML and static assets:

- `Content-Security-Policy` limited to same-origin scripts, styles, images, fonts, and connections;
  object and frame ancestors are denied.
- `X-Content-Type-Options: nosniff`.
- `X-Frame-Options: DENY`.
- `Referrer-Policy: no-referrer`.

Trace, dataset, and error content is inserted as text, never as HTML. Requests use `AbortController`
and are canceled when route inputs change or components unmount. Each response is associated with
the route key that created it so a stale response cannot replace newer route data.

Run polling has one active timer, pauses while the page is hidden, resets its transient failure count
after a successful response, and stops after a terminal state or unmount. Three consecutive transport
or 5xx failures stop automatic polling and show an explicit Retry action. Repeated cursor values are
treated as protocol errors instead of creating infinite pagination loops.

## 10. Embedded Asset Delivery

Vite writes production output to `assayd/internal/ui/dist/`. Generated assets are ignored, while a
tracked placeholder `dist/index.html` keeps ordinary Go compilation and tests possible before a
frontend build. The placeholder states that UI assets were not built; release and Docker builds
always run Vite first and replace it with the production index.

`assayd/internal/ui` embeds `dist/` and registers a GET/HEAD handler at `/`. Go's longest-pattern
matching keeps `/v1`, `/openapi.json`, `/docs`, `/healthz`, and `/readyz` ahead of the UI handler.
The handler behavior is:

- Serve existing static files with their detected content type.
- Apply `public, max-age=31536000, immutable` to hashed assets.
- Apply `no-cache` to `index.html`.
- Return `index.html` for extensionless SPA navigation paths.
- Return 404 for missing files that contain an extension and for non-GET/HEAD methods.
- Return 404 for all UI routes when `ASSAY_UI_ENABLED=false`.

The development server proxies API, OpenAPI, docs, and health paths to a local `assayd`. Production
uses only same-origin requests.

## 11. Build And CI

The production Dockerfile has three stages:

1. A pinned Node 22 image installs pnpm dependencies with the frozen lockfile and builds the SPA.
2. The Go stage copies `assayd/` plus the generated UI distribution and builds `assayd`.
3. The existing distroless runtime receives only the binary.

The frontend workflow runs for `web/**`, UI Go files, OpenAPI-affecting API files, and its own
workflow. It performs:

1. Frozen pnpm install with lifecycle scripts disabled unless an exact dependency requires one.
2. OpenAPI and generated-client drift check.
3. `oxlint`.
4. `oxfmt --check`.
5. `tsc --noEmit`.
6. Vitest.
7. Vite production build.
8. Relevant Go tests and final Go binary build with the generated assets.

GitHub Actions remain SHA-pinned. New package and action versions are verified as current stable at
implementation time and recorded exactly.

## 12. Testing

Frontend behavior tests use Vitest, Testing Library, and MSW. MSW mocks the external HTTP boundary;
tests do not mock feature logic or generated client internals. Required cases are:

- Token validation, persistence, 401 removal, and disconnect.
- Application list and application switching.
- Trace list pagination, repeated-cursor rejection, span-tree selection, attributes, events, and
  score rationale/detail rendering.
- Dataset list, item pagination, and empty values.
- Run form validation, creation, polling transitions, aggregates, cancellation, transient failure
  recovery, visibility pause, and terminal stop.
- Problem+JSON normalization, transport retry, aborted stale requests, and not-found routes.
- Keyboard/focus behavior for the navigation drawer, dialogs, tabs, and tree expanders.

Go tests cover:

- Admin and project-key trace read authorization, including the admin `application_id` requirement.
- Cross-project isolation for project keys after admin access is added.
- Static files, MIME types, cache headers, security headers, HEAD requests, and missing assets.
- SPA fallback and direct deep links.
- API, docs, OpenAPI, and health route precedence.
- `ASSAY_UI_ENABLED=false` behavior.

Tests target observable behavior. The release gate does not use a line-coverage percentage.

## 13. Accessibility And Responsive Behavior

All controls are keyboard reachable and retain visible focus. Dialogs trap focus and restore it to
their trigger. Span expanders are buttons with `aria-expanded`; tree nesting is represented
semantically. Tables retain headers and accessible names. Loading states announce progress without
repeated polling announcements. Errors use an alert region only when the user must act.

Status is always expressed by text and shape as well as color. The design supports browser zoom to
200%, a 360-pixel viewport, reduced motion, and horizontal table scrolling. Desktop split panes
become ordered stacked sections on mobile; no operation is desktop-only.

## 14. Documentation

M5.5 updates the root README with the embedded UI status, startup URL, token warning, build commands,
and `ASSAY_UI_ENABLED`. Architecture documentation records the `web` to generated-client to Go embed
pipeline. The original design status is updated to mark M5.5 complete only after the acceptance
checks pass.
