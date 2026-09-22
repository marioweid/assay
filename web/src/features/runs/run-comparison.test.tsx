import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const appID = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const datasetID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const baselineID = "019d11d2-cbd3-7a5e-ae83-9b791c932922";
const candidateID = "019d11d2-cbd3-7a5e-ae83-9b791c932923";
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("renders signed paired deltas, exclusions, warnings, and keyboard evidence", async () => {
  server.use(...comparisonHandlers());
  renderApp(comparisonURL(baselineID, candidateID));
  const user = userEvent.setup();

  expect(await screen.findByRole("heading", { name: "Baseline vs Candidate" })).toBeInTheDocument();
  expect(screen.getByText("case-positive · matched · +0.50")).toBeInTheDocument();
  expect(screen.getByText("case-negative · matched · -0.60")).toBeInTheDocument();
  expect(screen.getByText("case-zero · matched · 0.00")).toBeInTheDocument();
  expect(screen.getByText("case-unscored · unscored · Excluded")).toBeInTheDocument();
  expect(screen.getByLabelText("Comparison warnings")).toHaveTextContent(
    "Judge configuration differs",
  );

  const summary = screen.getByText("case-positive · matched · +0.50");
  summary.focus();
  await user.keyboard("{Enter}");
  const evidence = summary.parentElement?.querySelector("div");
  expect(evidence).toHaveClass("lg:grid-cols-2");
  if (evidence === null || evidence === undefined)
    throw new Error("comparison evidence is missing");
  expect(within(evidence).getByRole("heading", { name: "Baseline: Baseline" })).toBeInTheDocument();
  expect(
    within(evidence).getByRole("heading", { name: "Candidate: Candidate" }),
  ).toBeInTheDocument();
});

test("swaps the pair while retaining the scorer filter and reversing deltas", async () => {
  server.use(...comparisonHandlers());
  renderApp(comparisonURL(baselineID, candidateID));
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Swap baseline and candidate" }));
  expect(await screen.findByRole("heading", { name: "Candidate vs Baseline" })).toBeInTheDocument();
  expect(screen.getByText("case-positive · matched · -0.50")).toBeInTheDocument();
  expect(screen.getAllByText("groundedness").length).toBeGreaterThan(0);
});

test("explains when no cases have paired scores", async () => {
  server.use(
    ...comparisonHandlers({
      items: [comparisonRow("unscored", null, "case-unscored")],
      summary: comparisonSummary(0),
      warnings: [],
    }),
  );
  renderApp(comparisonURL(baselineID, candidateID));

  expect(
    await screen.findByText("No unchanged cases have scores in both runs."),
  ).toBeInTheDocument();
});

test("rejects run IDs outside the current application before comparing", async () => {
  let comparisons = 0;
  server.use(
    ...applicationHandlers(),
    http.get(`*/v1/runs/${baselineID}`, () =>
      HttpResponse.json(runFixture(baselineID, "Baseline")),
    ),
    http.get(`*/v1/runs/${candidateID}`, () =>
      HttpResponse.json({ ...runFixture(candidateID, "Candidate"), application_id: datasetID }),
    ),
    http.get(`*/v1/runs/${baselineID}/comparison`, () => {
      comparisons++;
      return HttpResponse.json(comparisonFixture());
    }),
  );
  renderApp(comparisonURL(baselineID, candidateID));

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Both runs must belong to this application",
  );
  expect(comparisons).toBe(0);
});

test("selects explicit terminal baseline, candidate, and shared scorer", async () => {
  server.use(
    ...comparisonHandlers(),
    http.get("*/v1/runs", () =>
      HttpResponse.json({
        items: [
          runFixture(baselineID, "Baseline"),
          runFixture(candidateID, "Candidate"),
          { ...runFixture("019d11d2-cbd3-7a5e-ae83-9b791c932924", "Running"), status: "running" },
        ],
      }),
    ),
    http.get("*/v1/datasets", () => HttpResponse.json({ items: [] })),
  );
  renderApp(`/apps/${appID}/runs`);
  const user = userEvent.setup();

  await user.selectOptions(await screen.findByLabelText("Baseline run"), baselineID);
  await user.selectOptions(screen.getByLabelText("Candidate run"), candidateID);
  expect(screen.queryByRole("option", { name: "Running" })).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText("Shared scorer"), "groundedness");
  await user.click(screen.getByRole("button", { name: "Compare runs" }));

  expect(await screen.findByRole("heading", { name: "Baseline vs Candidate" })).toBeInTheDocument();
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

function comparisonURL(baseline: string, candidate: string): string {
  return `/apps/${appID}/runs/compare?baseline=${baseline}&candidate=${candidate}&scorer=groundedness`;
}

function comparisonHandlers(override?: Record<string, unknown>) {
  return [
    ...applicationHandlers(),
    http.get("*/v1/runs/:runId/comparison", ({ params, request }) => {
      const swapped = params["runId"] === candidateID;
      expect(new URL(request.url).searchParams.get("scorer")).toBe("groundedness");
      const fixture = comparisonFixture(swapped);
      return HttpResponse.json(override === undefined ? fixture : { ...fixture, ...override });
    }),
    http.get("*/v1/runs/:runId", ({ params }) => {
      const id = String(params["runId"]);
      return HttpResponse.json(runFixture(id, id === baselineID ? "Baseline" : "Candidate"));
    }),
  ];
}

function applicationHandlers() {
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
  ];
}

function comparisonFixture(swapped = false) {
  const sign = swapped ? -1 : 1;
  return {
    baseline_run_id: swapped ? candidateID : baselineID,
    candidate_run_id: swapped ? baselineID : candidateID,
    scorer: "groundedness",
    items: [
      comparisonRow("matched", 0.5 * sign, "case-positive"),
      comparisonRow("matched", -0.6 * sign, "case-negative"),
      comparisonRow("matched", 0, "case-zero"),
      comparisonRow("unscored", null, "case-unscored"),
    ],
    summary: { ...comparisonSummary(3), mean_delta: -0.033 * sign },
    warnings: ["Judge configuration differs between runs."],
  };
}

function comparisonSummary(n: number) {
  return {
    n,
    mean_delta: n === 0 ? null : -0.033,
    matched: n,
    changed_cases: 0,
    baseline_only: 0,
    candidate_only: 0,
    unscored: n === 0 ? 1 : 1,
  };
}

function comparisonRow(kind: string, delta: number | null, externalID: string) {
  const suffix =
    ["case-positive", "case-negative", "case-zero", "case-unscored"].indexOf(externalID) + 1;
  return {
    dataset_item_id: `${baselineID.slice(0, -1)}${suffix}`,
    kind,
    delta,
    baseline: runItemFixture(baselineID, externalID, delta === null ? [] : [0.4]),
    candidate: runItemFixture(candidateID, externalID, delta === null ? [] : [0.4 + delta]),
  };
}

function runItemFixture(runID: string, externalID: string, values: number[]) {
  return {
    eval_run_id: runID,
    dataset_item_id: `${runID.slice(0, -1)}1`,
    status: "succeeded",
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
    scores: values.map((value, index) => ({
      id: index + 1,
      scorer: "groundedness",
      value,
      threshold: 0.5,
      passed: value >= 0.5,
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

function runFixture(id: string, name: string) {
  return {
    id,
    application_id: appID,
    dataset_id: datasetID,
    name,
    status: "succeeded",
    mode: "score_existing",
    params: {},
    scorers: ["groundedness"],
    aggregates: {},
    total_items: 4,
    succeeded_items: 4,
    failed_items: 0,
    canceled_items: 0,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}
