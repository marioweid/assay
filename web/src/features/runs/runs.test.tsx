import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const appID = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const datasetID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const runID = "019d11d2-cbd3-7a5e-ae83-9b791c932922";
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
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
