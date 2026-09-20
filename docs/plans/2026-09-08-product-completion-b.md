# M7B Visual Workspace and Management Implementation Plan

> **For agentic workers:** Use `executing-plans` task-by-task, or the available
> `superpowers:subagent-driven-development` skill. Do not build a generic CRUD/form framework.

**Goal:** Make the existing application look consistent and let users manage it without leaving the UI.
**Architecture:** Tokenized light/dark styles and installed Radix primitives underpin focused feature
forms using generated requests. A2's catalog handles app changes; A4's APIs handle safe case editing.
**Tech Stack:** React/Vite/TypeScript/Tailwind/Radix/Lucide; Vitest/Testing Library/MSW.
**Spec:** [`../specs/2026-09-08-product-completion-design.md`](../specs/2026-09-08-product-completion-design.md), §§3–5,7.

## Global Constraints

Apply the [master constraints](2026-09-08-product-completion.md#global-constraints).
Use Odysseus only as visual inspiration; no AGPL code/assets copied. Preserve self-hosted IBM Plex.
B1 requires A1; B2–B4 require A2; B5–B6 require A3/A4. All forms use generated operation types.

## B1. Establish tested theme tokens and accessible UI primitives

**Files**
- Modify: `web/src/styles.css`, `web/src/main.tsx`, `web/src/components/modal.tsx`.
- Create: `web/src/app/theme-provider.tsx`, `theme-provider.test.tsx`.
- Create: `web/src/components/ui/button.tsx`, `field.tsx`, `dialog.tsx`, `status-badge.tsx`,
  `web/src/components/ui/primitives.test.tsx`.
- Create: `web/src/components/problem-state.tsx`, `empty-state.tsx`, `loading-state.tsx`.
- Modify: existing dialog consumers in dataset/run features as needed to remove the old Modal.

**Interfaces:** `ThemePreference = "system" | "light" | "dark"`; `useTheme()` returns
`{preference, setPreference}`. Store only preference at `assay.theme.v1`. Root `data-theme` holds the
resolved light/dark theme. Components use semantic tokens rather than hardcoded `bg-white`/blue classes.

- [x] Add tests for saved/system preference, live system change only in system mode, storage throwing,
  and no storage value becoming executable markup. For dialogs, test Tab/Shift+Tab trap, Escape,
  focus restoration, accessible title/description, background inertness, disabled submit, and 360px
  scroll containment. A small harness exercises outcomes:

```tsx
function DialogHarness() {
  const [open, setOpen] = useState(false);
  return <>
    <Button onClick={() => setOpen(true)}>Edit case</Button>
    <Dialog open={open} onOpenChange={setOpen} title="Edit dataset case">
      <label>Question<input /></label>
      <Button onClick={() => setOpen(false)}>Cancel</Button>
    </Dialog>
  </>;
}
```

  `Button` consumes ordinary button props plus `variant: primary|secondary|ghost|danger` (optional,
  default secondary); `Dialog` consumes the exact props above plus optional description. Use current
  installed Radix API documentation, not guessed import exports.
- [x] Run new tests and observe missing implementation/old focus behavior failures.
- [x] Add original theme tokens. Use these as starting colors, then measure contrast, adjusting them
  before acceptance rather than claiming the values are accessible without verification:

```css
:root, :root[data-theme="light"] {
  --color-canvas: #f3f5f8; --color-surface: #ffffff; --color-rail: #e9eef5;
  --color-ink: #172033; --color-muted: #52627a; --color-line: #cbd5e1;
  --color-accent: #275f91; --color-accent-strong: #194d79;
  --color-success: #216447; --color-danger: #a32f40; --color-warning: #805900;
}
:root[data-theme="dark"] {
  --color-canvas: #20252e; --color-surface: #282f3a; --color-rail: #171c24;
  --color-ink: #edf2f8; --color-muted: #acb9cb; --color-line: #465368;
  --color-accent: #91c8ed; --color-accent-strong: #b5ddf7;
  --color-success: #8bd2ac; --color-danger: #ffa6af; --color-warning: #efcb80;
}
```

  Integrate with Tailwind's existing theme mechanism so runtime overrides actually work. Use solid
  `accent` button background with a separately contrast-checked foreground token. Add spacing/radius,
  `min-width:0` and wrapped long strings where necessary; avoid global overflow hiding.
- [x] Implement theme detection in external bundled JS (no inline scripts that violate CSP). If
  rendering briefly waits for theme resolution, show the correctly themed root, not a white flash.
  Preference access may fail: continue in system mode without swallowing unrelated errors.
- [x] Replace manual Modal implementation with the new Radix-backed Dialog at all existing call
  sites, then delete `modal.tsx`. Do not keep two dialog stacks. Danger confirmation focuses Cancel
  first. Dirty form close asks “Discard changes?”; a submitted request abort is not a server rollback.
- [x] Run primitive/theme + existing dataset/run/router tests; lint/format/types; build and inspect
  under real embedded CSP, not only Vite. Commit `Establish accessible Assay workspace styling`.

**Gate:** A text-heavy dialog, table, code block, and all status variants are readable in both themes;
no scripts/styles need an unsafe CSP exception. Browser-wide overflow hiding is not an acceptable fix.

## B2. Refresh navigation and shared page states

**Files**
- Modify: `web/src/components/app-shell.tsx`, `web/src/app/router.tsx`, `router.test.tsx`,
  `web/src/auth/connection-gate.tsx`, `web/src/features/applications/applications-page.tsx`.
- Create: `web/src/components/workspace-header.tsx`, `page-heading.tsx`,
  `web/src/components/app-shell.test.tsx`.
- Modify: `web/src/features/metrics/metrics-page.tsx` to consume tokens/shared states.

**Interfaces:** Preserve all existing trace/dataset/run/metrics deep links. Add `/projects`,
`/projects/:projectId`, `/apps/:appId/settings`. Navigation labels are “Evaluations” and “Score trends”
while paths remain `/runs` and `/metrics`. `ProblemState` takes `{title, detail?, onRetry?}`;
`EmptyState` takes `{title, description, action?: ReactNode}`.

- [x] Test global Applications/Projects navigation, application switcher, active route indication,
  unknown app fallback link, preserved deep link after connect, theme selector, and mobile drawer
  keyboard/close/focus. Test heading hierarchy and a retryable failed metrics request.
- [x] Implement 224px desktop rail and 56px header, compact Lucide+text links, Assay-owned small icon,
  breadcrumbs and visible workspace exit. Keep content as `minmax(0,1fr)` and give tables their own
  horizontal scroller. Replace the hand-built drawer with the B1 dialog primitive.
- [x] Distinguish these states in route components:

```text
loading: labeled skeleton, aria-busy on section, no repeated live announcements
empty: no resources yet, action to create/connect SDK
filtered-empty: no matching records, clear filters action
error: preserve prior content if available, actionable problem text and retry
not-found: resource missing or deleted, link to its owning list
```

- [x] Apply B1 tokens to connection, app list, metrics and shell; do not redesign metrics semantics
  (they currently combine online/offline UTC daily scores). Show sample counts and leave missing days
  as missing, not zero. Remove unsupported operational claims in empty copy.
- [x] Run router/auth/metrics tests, full frontend lint/format/types, production build. Capture only
  synthetic 1440px and 360px shell screenshots for review in the final browser harness. Commit
  `Unify workspace navigation and page states`.

**Gate:** No primary workflow is mobile-only/desktop-only; keyboard users can open and exit the drawer.
The interface remains a workbench, not a grid of new dashboards.

## B3. Implement project management and key rotation

**Files**
- Create: `web/src/features/projects/projects-page.tsx`, `project-detail.tsx`, `project-form.tsx`,
  `api-keys-panel.tsx`, `projects.test.tsx`, `api-keys.test.tsx`.
- Modify: `web/src/app/router.tsx`; consume B1 components and A2 catalog refresh.

**Consumes:** Existing generated `listProjects`, `createProject`, `getProject`, `updateProject`,
`deleteProject`, `createApiKey`/`listApiKeys`/`revokeApiKey` (verify exact generated export casing).
No new backend endpoints. Key plaintext exists only in the creation response's local component state.

- [x] Write an MSW flow for empty list → New project → name validation → create 201 → project detail
  → rename → delete confirmation. Cover duplicate 409 without losing form input; 401 disconnect;
  delete failure keeps item/list state intact. No optimistic deletion.
- [x] Write key tests: secret displayed exactly once on create, copy success/failure announced, dismiss
  clears it, list shows prefix/name/revocation/last use only, revoked key not offered as reusable.
  Assert neither localStorage nor sessionStorage contains the synthetic key.
- [x] Run new tests to establish missing routes/forms. Implement focused forms using existing APIs;
  prefer project names over raw IDs where available. Projects contain Applications and API keys;
  judge override editor is shared with B4 settings. Refresh catalog after project deletion.
- [x] Implement irreversible confirmation. Whole-project delete requires typing its exact name and
  states “Deletes this project, its applications, traces, datasets, evaluation runs, and scores.”
  Disable duplicate submit, abort/fence stale callbacks, close only after server success.
- [x] Implement key rotation instructions: create replacement, update emitters, then revoke old key.
  Do not offer Edit secret or recover plaintext. A copy failure leaves the one-time display visible
  with manual-select instructions; never log the value in error messages.
- [x] Run project/key/router tests and shared lint/types; commit `Manage projects and ingest keys in UI`.

**Gate:** A fresh database can reach an application-ready project without terminal management commands.
A user understands losing a key means creating a new one, not revealing the hash.

## B4. Implement application setup and scorer/endpoint settings

**Files**
- Modify: `web/src/features/applications/applications-page.tsx`.
- Create: `web/src/features/applications/application-form.tsx`, `application-settings.tsx`,
  `endpoint-form.tsx`, `scorers-panel.tsx`, `sdk-setup.tsx`, `applications.test.tsx`, `settings.test.tsx`.
- Create: `web/src/features/projects/judge-config-form.tsx`; use in project detail.
- Modify: `web/src/app/router.tsx`, consume A2 catalog.

**Consumes:** Existing application PATCH/endpoint/scorer/project contracts in
`assayd/internal/api/{applications,scorer_configs,types,projects}.go`. Do not invent a “test connection”
endpoint or send paid judge requests just because a settings page loads.

- [ ] Test create/edit/delete application with project selection, slug conflict and stale catalog
  refresh; slug change warns about emitter configuration; owning project cannot change. Form retains
  advanced config JSON fields on edit; do not replace a config object with only the visible fields.
- [ ] Test settings body contents at the HTTP boundary:

```json
{
  "endpoint": {
    "url": "http://target:8090/answer",
    "method": "POST",
    "headers": {"Authorization": "Bearer {{ .secret }}"},
    "request_template": {"question": "{{ .item.input.question }}"},
    "response_mapping": {"output": "$.answer", "context": "$.sources[*].text"},
    "timeout_ms": 30000
  }
}
```

  Secret omitted means preserve; explicit “Replace secret” sends `secret`, and “Remove endpoint”
  sends `{clear:true}` with confirmation. Match current template syntax/validation from
  `assayd/internal/target/template_test.go` before saving this example as UI copy.
- [ ] Test invalid URL/timeout surfaced by settings validation and malformed JSON rejected locally;
  JSONPath/template execution errors are surfaced by run item review in D2. A successful settings
  save does not prove the endpoint can execute. Test threshold 0 and 1 accepted, values outside [0,1]
  rejected, enabled versus auto-score controls distinct, and blank password
  input never overwrites a stored key accidentally. Responses only indicate `has_api_key`/`has_secret`.
- [ ] Implement General, Evaluation, and SDK setup sections. Judge resolution copy: process defaults →
  project override → per-scorer override. Show only what API reports; do not claim an effective process
  model that the API does not expose. For scorer PUT, send the full **current displayed** override when
  changing another field, because current semantics may replace its non-secret fields; cover this
  with request-body tests. Project clear-override uses its existing explicit flag.
- [ ] Keep all bearer material in write-only secret fields; ordinary endpoint headers should contain
  `{{ .secret }}`, not entered plaintext tokens. UI rejects a literal Authorization credential with
  guidance to use the secret field. No provider credentials in preview output or copied setup code.
- [ ] Implement setup showing actual app slug and endpoint, with key placeholder and env instructions:

```python
import assay

assay.init(capture=True)  # ASSAY_ENDPOINT, ASSAY_API_KEY, ASSAY_APPLICATION

@assay.trace
def answer(question: str) -> str:
    return "Assay evaluates AI systems."

answer("What is Assay?")
assay.shutdown()
```

  Explain capture is optional and potentially sensitive. Key creation is a separate explicit action,
  not automatic whenever the setup page opens. Empty traces link here; settings link to project keys.
- [ ] Run application/settings/project tests, frontend gates; commit
  `Add application onboarding and evaluation settings`.

**Gate:** Create app, configure target/scorers, and obtain minimal tracing instructions entirely in UI.
Settings never expose saved credential values or mutate them on a mere read.

## B5. Complete dataset metadata and case CRUD without losing data

**Files**
- Modify: `web/src/features/datasets/{datasets-page,dataset-detail,create-dataset-dialog}.tsx`.
- Replace/delete: `web/src/features/datasets/add-item-dialog.tsx` with focused
  `dataset-item-editor.tsx` used for create and replace.
- Create: `web/src/features/datasets/dataset-metadata-dialog.tsx`, `dataset-item-editor.test.tsx`.
- Extend: `web/src/features/datasets/datasets.test.tsx`.

**Consumes:** A4 PATCH/GET/PUT/DELETE contracts; B1 Dialog and fields. Item editor uses one complete
local editable value with advanced JSON fields, not destructive conversions from existing items.

- [ ] Write create → inspect → edit → clear nullable output → delete tests. Edit a case with custom
  `input.language`, two context chunks, external ID and metadata; changing only question must retain
  all other fields. Assert complete PUT body, preserving explicit nulls. Unknown/wrong-parent 404
  offers back to dataset; 409 duplicate external ID retains the draft and does not overwrite a row.
- [ ] Test blank question, malformed input/metadata JSON, array input, duplicate chunk IDs, empty
  context, reference-only case, and absent recorded output. Item identity/created_at are read-only.
  Distinguish “Recorded answer” from “Expected answer” in labels and help text.
- [ ] Implement a compact dataset table/detail with metadata Edit/Delete, case Add/Edit/Delete, and
  clear action hierarchy. Editor fields: question, recorded answer, expected answer, context chunk
  rows (id/text), external ID, advanced input JSON and metadata JSON. Keep question synchronized
  into the full input object; do not discard non-question keys.
- [ ] Reset pagination to a fresh first page after a successful mutation and deduplicate by ID. Do not
  append edited/created objects in arbitrary order to an ascending cursor list. Preserve pagination
  on failed mutation. If the dataset disappears while a dialog is open, show not-found and discard
  callbacks belonging to its old route.
- [ ] Item delete confirmation says old evaluation evidence remains. Dataset delete confirmation says
  its cases and evaluation runs/scores are removed. Dirty navigation/back/close prompts before losing
  unsaved input; do not persist sensitive drafts in browser storage.
- [ ] Run dataset/item editor tests and frontend gates; verify against A4 API tests, commit
  `Complete dataset and case management UI`.

**Gate:** Every editable field can be inspected and changed without replacing hidden data. Historical
run evidence survives item mutations by A3, not by UI assumptions.

## B6. Add bounded JSONL import/export and dependable pagination

**Files**
- Create: `web/src/features/datasets/dataset-jsonl.ts`, `dataset-jsonl.test.ts`,
  `import-dataset-dialog.tsx`, `import-dataset-dialog.test.tsx`.
- Modify: `web/src/features/datasets/dataset-detail.tsx`, `web/src/features/runs/runs-page.tsx`,
  `create-run-dialog.tsx` (dataset selection pagination).
- Reference: `clients/python/assay/src/assay/importers.py` for existing JSONL field semantics.

**Interfaces:** `parseDatasetJsonl(text: string): DatasetItemInput[]` uses generated creation type;
`serializeDatasetJsonl(items: readonly DatasetItemResponse[]): string` exports writable source fields
only. Max UI upload 5MiB/1,000 items; one atomic existing bulk POST. CSV remains the existing SDK/CLI
import path, explicitly linked; do not add a second CSV parser dependency.

- [ ] Add complete parser vectors before UI code:

```ts
test("reports the failing JSONL line without echoing content", () => {
  const source = '{"input":{"question":"ok"}}\n{private-invalid';
  expect(() => parseDatasetJsonl(source)).toThrow("Line 2: invalid JSON");
});
test("exports editable fields without database identifiers", () => {
  const item = {
    id: "item", dataset_id: "dataset", external_id: "case-1",
    input: {question: "Why?"}, context: [], metadata: {},
    created_at: "2026-09-08T00:00:00Z", updated_at: "2026-09-08T00:00:00Z",
  } satisfies DatasetItemResponse;
  const text = serializeDatasetJsonl([item]);
  expect(JSON.parse(text)).toEqual({
    external_id: "case-1", input: {question: "Why?"}, context: [], metadata: {},
  });
});
```

- [ ] Test BOM, CRLF, blank lines, empty file, malformed objects, invalid context, Unicode byte-size
  limit, duplicate external IDs in file, and server 409 with zero rows committed. Explicit null optional
  values in exported source normalize to omission for the existing POST contract; do not copy run IDs.
- [ ] Implement preview with count and first ten cases; require explicit Import. On conflict keep file
  and show “No cases imported”; there is no silent skip/upsert. For larger imports link to CLI and its
  documented chunked partial-commit behavior rather than claiming atomicity across chunks.
- [ ] Export follows all item cursors with one fixed dataset ID, AbortController, duplicate-ID/repeated-
  cursor guards, and an explicit busy/cancel state. Do not label current-page export “all cases.” Warn
  to avoid concurrent dataset edits during export; this is not a database backup. Cap browser export
  at 10,000 cases/50MiB and fail with CLI guidance instead of silently truncating.
- [ ] Dataset selection in New run must reach datasets beyond page 1: add load-more in the selector,
  scoped to current application with the same cursor guards. A missing endpoint disables
  generate-then-score with a Settings link. Do not load every application dataset on page mount.
- [ ] Run JSONL/dialog/dataset/run tests, frontend gates; commit
  `Add bounded dataset transfer and complete dataset selection`.

**M7B demo:** Fresh workspace → project/app/key → configure scorers → create/import/edit/delete case;
light/dark, keyboard-only, narrow viewport, server validation errors, and unsaved-change handling.
Ask for visual review before applying this design to the entire tracing/evaluation workbench.
