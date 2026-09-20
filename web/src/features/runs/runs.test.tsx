import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";
import { vi } from "vitest";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const appID = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const datasetID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const runID = "019d11d2-cbd3-7a5e-ae83-9b791c932922";
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => {
  server.resetHandlers();
  vi.useRealTimers();
});
afterAll(() => server.close());

test("lists runs with progress and aggregate summaries", async () => {
  server.use(
    ...baseHandlers(),
    http.get("*/v1/runs", ({ request }) => {
      expect(new URL(request.url).searchParams.get("application_id")).toBe(appID);
      return HttpResponse.json({ items: [runFixture("succeeded")] });
    }),
  );
  renderApp(`/apps/${appID}/runs`);

  expect(await screen.findByRole("link", { name: "Baseline run" })).toBeInTheDocument();
  expect(screen.getByRole("table").parentElement).toHaveClass("bg-surface");
  expect(screen.getByRole("link", { name: "Baseline run" })).toHaveClass("text-accent");
  expect(screen.getByText("Regression cases")).toBeInTheDocument();
  expect(screen.getByText("3 / 4")).toBeInTheDocument();
  expect(screen.getByText(/groundedness 0.82/)).toBeInTheDocument();
});

test("loads additional datasets before creating a run", async () => {
  server.use(
    ...baseHandlers(),
    http.get("*/v1/runs", () => HttpResponse.json({ items: [] })),
  );
  renderApp(`/apps/${appID}/runs`);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "New evaluation run" }));
  await user.click(screen.getByRole("button", { name: "Load more datasets" }));
  expect(await screen.findByRole("option", { name: "Later cases" })).toBeInTheDocument();
});

test.each([
  ["score_existing", "Score existing outputs"],
  ["generate_then_score", "Generate then score"],
] as const)("creates and navigates to a %s run", async (mode, modeLabel) => {
  let requestBody: unknown;
  server.use(
    ...baseHandlers(),
    http.get("*/v1/runs", () => HttpResponse.json({ items: [] })),
    http.post("*/v1/runs", async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json(runFixture("succeeded"), { status: 202 });
    }),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
  );
  renderApp(`/apps/${appID}/runs`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "New evaluation run" }));
  expect(screen.getByLabelText("Run name")).toHaveFocus();
  await user.click(screen.getByRole("button", { name: "Create run" }));
  expect(screen.getByRole("alert")).toHaveTextContent("name, dataset, and scorer");
  await user.type(screen.getByLabelText("Run name"), "Baseline run");
  await user.selectOptions(screen.getByLabelText("Dataset"), datasetID);
  await user.selectOptions(screen.getByLabelText("Mode"), mode);
  expect(screen.getByRole("option", { name: modeLabel })).toBeInTheDocument();
  await user.click(screen.getByLabelText("groundedness"));
  await user.click(screen.getByRole("button", { name: "Create run" }));

  expect(requestBody).toEqual({
    application_id: appID,
    dataset_id: datasetID,
    mode,
    name: "Baseline run",
    scorers: ["groundedness"],
  });
  expect(await screen.findByRole("heading", { name: "Baseline run" })).toBeInTheDocument();
});

test("keeps problem details inside the creation dialog", async () => {
  server.use(
    ...baseHandlers(),
    http.get("*/v1/runs", () => HttpResponse.json({ items: [] })),
    http.post("*/v1/runs", () =>
      HttpResponse.json({ title: "Invalid run", detail: "Dataset has no items" }, { status: 422 }),
    ),
  );
  renderApp(`/apps/${appID}/runs`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "New evaluation run" }));
  await user.type(screen.getByLabelText("Run name"), "Broken run");
  await user.selectOptions(screen.getByLabelText("Dataset"), datasetID);
  await user.click(screen.getByLabelText("correctness"));
  await user.click(screen.getByRole("button", { name: "Create run" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("Dataset has no items");
  expect(screen.getByRole("dialog", { name: "New evaluation run" })).toBeInTheDocument();
});

test("allows a pending run to be cancelled", async () => {
  server.use(
    ...baseHandlers(),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("pending"))),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);

  expect(await screen.findByRole("button", { name: "Cancel run" })).toBeInTheDocument();
  expect(screen.getByText("Total").parentElement).toHaveClass("bg-surface");
});

test("confirms and applies active-run cancellation", async () => {
  let canceled = false;
  server.use(
    ...baseHandlers(),
    http.get(`*/v1/runs/${runID}`, () =>
      HttpResponse.json(runFixture(canceled ? "canceled" : "running")),
    ),
    http.post(`*/v1/runs/${runID}/cancel`, () => {
      canceled = true;
      return HttpResponse.json(runFixture("canceled"));
    }),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Cancel run" }));
  expect(screen.getByRole("dialog", { name: "Cancel evaluation run" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Confirm cancellation" }));
  expect(await screen.findByText("canceled")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Cancel run" })).not.toBeInTheDocument();
});

test("shows scored and execution-failed run cases", async () => {
  server.use(
    http.get(`*/v1/runs/${runID}/items`, () =>
      HttpResponse.json({
        items: [
          runItemFixture("scored", "succeeded", [{ id: 1, value: 0, passed: false }]),
          { ...runItemFixture("network", "failed", []), error: "Target unavailable" },
        ],
      }),
    ),
    http.get(`*/v1/runs/${runID}/items/:itemId`, () =>
      HttpResponse.json(
        runItemFixture("scored", "succeeded", [{ id: 1, value: 0, passed: false }]),
      ),
    ),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  expect(await screen.findByRole("heading", { name: "Cases" })).toBeInTheDocument();
  expect(await screen.findByText("groundedness 0.00 · Fail")).toBeInTheDocument();
  expect(screen.getByText("Target unavailable")).toBeInTheDocument();
  expect(screen.getByText("Not scored")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "scored" }));
  expect(await screen.findByRole("heading", { name: "Case scored" })).toBeInTheDocument();
  expect(screen.getByText("Original input")).toBeInTheDocument();
  expect(screen.getByText("Evidence")).toBeInTheDocument();
});

test("prefills a new run from current dataset and configuration", async () => {
  let requestBody: unknown;
  server.use(
    http.post("*/v1/runs", async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json(runFixture("pending"), { status: 201 });
    }),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Run again" }));
  const dialog = screen.getByRole("dialog", { name: "Run evaluation again" });
  expect(dialog).toHaveTextContent("current dataset and current application/scorer configuration");
  expect(within(dialog).getByLabelText("Run name")).toHaveValue("Baseline run copy");
  expect(within(dialog).getByLabelText("Dataset")).toHaveValue(datasetID);
  expect(within(dialog).getByLabelText("groundedness")).toBeChecked();
  await user.click(within(dialog).getByRole("button", { name: "Create run" }));

  await vi.waitFor(() =>
    expect(requestBody).toEqual({
      application_id: appID,
      dataset_id: datasetID,
      mode: "score_existing",
      name: "Baseline run copy",
      scorers: ["groundedness"],
    }),
  );
});

test("prevents dismissing a rerun while creation is in flight", async () => {
  let finishRequest: () => void = () => undefined;
  const pendingRequest = new Promise<void>((resolve) => {
    finishRequest = resolve;
  });
  server.use(
    http.post("*/v1/runs", async () => {
      await pendingRequest;
      return HttpResponse.json(runFixture("pending"), { status: 201 });
    }),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Run again" }));
  const dialog = screen.getByRole("dialog", { name: "Run evaluation again" });
  const submission = user.click(within(dialog).getByRole("button", { name: "Create run" }));
  await vi.waitFor(() =>
    expect(within(dialog).getByRole("button", { name: "Close" })).toBeDisabled(),
  );
  finishRequest();
  await submission;
  await vi.waitFor(() =>
    expect(within(dialog).getByRole("button", { name: "Close" })).not.toBeDisabled(),
  );
});

test("blocks another rerun after an uncertain create outcome", async () => {
  server.use(
    http.post("*/v1/runs", () => HttpResponse.error()),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Run again" }));
  const dialog = screen.getByRole("dialog", { name: "Run evaluation again" });
  const create = within(dialog).getByRole("button", { name: "Create run" });
  await user.click(create);

  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Creation outcome is unknown");
  expect(create).toBeDisabled();
  expect(within(dialog).getByRole("button", { name: "Close" })).toBeDisabled();
  await user.click(within(dialog).getByRole("button", { name: "Refresh run list" }));
  expect(await screen.findByRole("heading", { name: "Evaluation runs" })).toBeInTheDocument();
});

test("deletes a terminal run after typed-name confirmation", async () => {
  let deleted = false;
  server.use(
    http.delete(`*/v1/runs/${runID}`, () => {
      deleted = true;
      return new HttpResponse(null, { status: 204 });
    }),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Delete run" }));
  const dialog = screen.getByRole("dialog", { name: "Delete evaluation run?" });
  const confirm = within(dialog).getByRole("button", { name: "Delete run" });
  expect(dialog).toHaveTextContent("case outcomes, and all scores");
  expect(confirm).toBeDisabled();
  const confirmation = within(dialog).getByLabelText(/to confirm/);
  await user.type(confirmation, " Baseline run ");
  expect(confirm).toBeDisabled();
  await user.clear(confirmation);
  await user.type(confirmation, "Baseline run");
  await user.click(confirm);

  expect(deleted).toBe(true);
  expect(await screen.findByRole("heading", { name: "Evaluation runs" })).toBeInTheDocument();
});

test("blocks another delete after an uncertain transport outcome", async () => {
  server.use(
    http.delete(`*/v1/runs/${runID}`, () => HttpResponse.error()),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Delete run" }));
  const dialog = screen.getByRole("dialog", { name: "Delete evaluation run?" });
  await user.type(within(dialog).getByLabelText(/to confirm/), "Baseline run");
  const confirm = within(dialog).getByRole("button", { name: "Delete run" });
  await user.click(confirm);

  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Deletion outcome is unknown");
  expect(confirm).toBeDisabled();
});

test("refreshes after a run deletion conflict", async () => {
  let runRequests = 0;
  server.use(
    http.delete(`*/v1/runs/${runID}`, () =>
      HttpResponse.json({ title: "Conflict" }, { status: 409 }),
    ),
    http.get(`*/v1/runs/${runID}`, () => {
      runRequests++;
      return HttpResponse.json(runFixture(runRequests === 1 ? "succeeded" : "running"));
    }),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Delete run" }));
  const dialog = screen.getByRole("dialog", { name: "Delete evaluation run?" });
  await user.type(within(dialog).getByLabelText(/to confirm/), "Baseline run");
  await user.click(within(dialog).getByRole("button", { name: "Delete run" }));

  expect(await screen.findByRole("button", { name: "Cancel run" })).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent(
    "Run state changed. Only terminal runs can be deleted. Run details were refreshed.",
  );
  expect(runRequests).toBe(2);
});

test("polls the visible case page while the parent timestamp is unchanged", async () => {
  vi.useFakeTimers();
  let runRequests = 0;
  let itemRequests = 0;
  server.use(
    http.get(`*/v1/runs/${runID}/items`, () => {
      itemRequests++;
      return HttpResponse.json({
        items: [
          runItemFixture(
            itemRequests === 1
              ? "pending-case"
              : itemRequests === 2
                ? "running-case"
                : "finished-case",
            "succeeded",
            [],
          ),
        ],
      });
    }),
    http.get(`*/v1/runs/${runID}`, () => {
      runRequests++;
      return HttpResponse.json(runFixture(runRequests < 3 ? "running" : "succeeded"));
    }),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);

  await vi.waitFor(() =>
    expect(screen.getByRole("button", { name: "pending-case" })).toBeInTheDocument(),
  );
  await act(() => vi.advanceTimersByTimeAsync(1000));
  await vi.waitFor(() =>
    expect(screen.getByRole("button", { name: "running-case" })).toBeInTheDocument(),
  );
  await act(() => vi.advanceTimersByTimeAsync(1000));
  await vi.waitFor(() =>
    expect(screen.getByRole("button", { name: "finished-case" })).toBeInTheDocument(),
  );
  expect(itemRequests).toBe(3);
});

test("pages run cases without accumulating prior pages", async () => {
  const cursors: Array<string | null> = [];
  server.use(
    http.get(`*/v1/runs/${runID}/items`, ({ request }) => {
      const cursor = new URL(request.url).searchParams.get("cursor");
      cursors.push(cursor);
      return HttpResponse.json({
        items: [runItemFixture(cursor === null ? "first" : "second", "succeeded", [])],
        ...(cursor === null ? { next_cursor: "next" } : {}),
      });
    }),
    http.get(`*/v1/runs/${runID}`, () => HttpResponse.json(runFixture("succeeded"))),
    ...baseHandlers(),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);
  const user = userEvent.setup();

  expect(await screen.findByRole("button", { name: "first" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Next cases" }));
  expect(await screen.findByRole("button", { name: "second" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "first" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Previous cases" }));
  expect(await screen.findByRole("button", { name: "first" })).toBeInTheDocument();
  expect(cursors).toEqual([null, "next", null]);
});

test("rejects a run from a different application", async () => {
  server.use(
    ...baseHandlers(),
    http.get(`*/v1/runs/${runID}`, () =>
      HttpResponse.json({ ...runFixture("succeeded"), application_id: datasetID }),
    ),
  );
  renderApp(`/apps/${appID}/runs/${runID}`);

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Run does not belong to this application",
  );
});

function renderApp(path: string): void {
  render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
}

function baseHandlers() {
  return [
    http.get("*/v1/runs/:runId/items", () => HttpResponse.json({ items: [] })),
    http.get("*/v1/applications", () =>
      HttpResponse.json({
        items: [
          {
            id: appID,
            project_id: appID,
            name: "Primary",
            slug: "primary",
            auto_score_scorers: [],
            config: {},
            created_at: "2026-09-01T10:00:00Z",
            updated_at: "2026-09-01T10:00:00Z",
          },
        ],
      }),
    ),
    http.get("*/v1/datasets", ({ request }) => {
      if (new URL(request.url).searchParams.get("cursor") === "next") {
        return HttpResponse.json({
          items: [
            {
              id: "019d11d2-cbd3-7a5e-ae83-9b791c932933",
              application_id: appID,
              name: "Later cases",
              created_at: "2026-09-01T10:00:00Z",
              updated_at: "2026-09-01T10:00:00Z",
            },
          ],
        });
      }
      return HttpResponse.json({
        items: [
          {
            id: datasetID,
            application_id: appID,
            name: "Regression cases",
            created_at: "2026-09-01T10:00:00Z",
            updated_at: "2026-09-01T10:00:00Z",
          },
        ],
        next_cursor: "next",
      });
    }),
  ];
}

function runItemFixture(
  externalID: string,
  status: string,
  scores: Array<{ id: number; value: number; passed: boolean }>,
) {
  return {
    eval_run_id: runID,
    dataset_item_id: `${runID.slice(0, -1)}${externalID === "scored" ? "3" : "4"}`,
    status,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    snapshot_origin: "creation",
    snapshot: {
      id: datasetID,
      dataset_id: datasetID,
      external_id: externalID,
      input: { question: externalID },
      context: [],
      metadata: {},
      created_at: "2026-09-01T10:00:00Z",
      updated_at: "2026-09-01T10:00:00Z",
    },
    scores: scores.map((score) => ({
      ...score,
      scorer: "groundedness",
      threshold: 0.5,
      rationale: "Evidence",
      details: {},
      prompt_template_id: "groundedness@v1",
      judge_model: "judge",
      judge_provider: "fake",
      judge_tokens: 1,
      created_at: "2026-09-01T10:00:00Z",
    })),
  };
}

function runFixture(status: string) {
  return {
    id: runID,
    application_id: appID,
    dataset_id: datasetID,
    name: "Baseline run",
    status,
    mode: "score_existing",
    params: {},
    scorers: ["groundedness"],
    aggregates: { groundedness: { mean: 0.82, pass_rate: 0.75, n: 4 } },
    total_items: 4,
    succeeded_items: 3,
    failed_items: 1,
    canceled_items: status === "canceled" ? 1 : 0,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}
