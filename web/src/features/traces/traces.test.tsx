import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, delay, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const appID = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const otherAppID = "019d11d2-cbd3-7a5e-ae83-9b791c9329df";
const traceID = "019d11d2-cbd3-7a5e-ae83-9b791c9329aa";
const server = setupServer(
  http.get(`*/v1/traces/${traceID}/scoring-eligibility`, () =>
    HttpResponse.json({
      items: [
        { scorer: "groundedness", eligible: true, reasons: [] },
        {
          scorer: "correctness",
          eligible: false,
          reasons: [{ code: "missing_reference", message: "Trace is missing a reference answer." }],
        },
      ],
    }),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("keeps filters on every page and resets results after a search change", async () => {
  const paginatedRequests: URL[] = [];
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", ({ request }) => {
      const url = new URL(request.url);
      const cursor = url.searchParams.get("cursor");
      expect(url.searchParams.get("application_id")).toBe(appID);
      if (url.searchParams.get("q") === "needle") {
        expect(url.searchParams.get("start")).toBe("2026-09-01T00:00:00.000Z");
        expect(url.searchParams.get("end")).toBe("2026-09-02T00:00:00.000Z");
        expect(url.searchParams.get("status")).toBe("error");
        expect(url.searchParams.get("scorer")).toBe("groundedness");
        expect(url.searchParams.get("passed")).toBe("false");
        if (cursor !== null) paginatedRequests.push(url);
        const page = cursor === null ? 1 : cursor === "cursor-one" ? 2 : 3;
        return HttpResponse.json({
          items: [
            traceFixture(`needle operation ${page}`, `019d11d2-cbd3-7a5e-ae83-9b791c9329a${page}`),
          ],
          next_cursor: page === 1 ? "cursor-one" : page === 2 ? "cursor-two" : "cursor-two",
        });
      }
      expect(cursor).toBeNull();
      return HttpResponse.json({
        items: [traceFixture("unfiltered operation", "019d11d2-cbd3-7a5e-ae83-9b791c9329af")],
      });
    }),
  );
  renderApp(
    `/apps/${appID}/traces?q=needle&scorer=groundedness&passed=false&status=error&start=2026-09-01T00%3A00%3A00.000Z&end=2026-09-02T00%3A00%3A00.000Z`,
  );
  const user = userEvent.setup();

  expect(await screen.findByText("needle operation 1")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Load more" }));
  expect(await screen.findByText("needle operation 2")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Load more" }));

  expect(await screen.findByText("needle operation 3")).toBeInTheDocument();
  expect(paginatedRequests).toHaveLength(2);
  expect(paginatedRequests.map((url) => url.searchParams.get("cursor"))).toEqual([
    "cursor-one",
    "cursor-two",
  ]);
  expect(screen.getByRole("alert")).toHaveTextContent("repeated cursor");
  await user.clear(screen.getByLabelText("Search"));

  expect(await screen.findByText("unfiltered operation")).toBeInTheDocument();
  expect(screen.queryByText("needle operation 1")).not.toBeInTheDocument();
  expect(screen.queryByText("needle operation 2")).not.toBeInTheDocument();
  expect(screen.queryByText("needle operation 3")).not.toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

test("canonicalizes an orphaned pass filter before requesting traces", async () => {
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", ({ request }) => {
      const url = new URL(request.url);
      expect(url.searchParams.has("passed")).toBe(false);
      expect(url.searchParams.has("scorer")).toBe(false);
      return HttpResponse.json({ items: [traceFixture("canonical operation")] });
    }),
  );
  renderApp(`/apps/${appID}/traces?passed=false`);

  expect(await screen.findByText("canonical operation")).toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

test("keeps newer filters when a search debounce completes", async () => {
  const searches: URL[] = [];
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", ({ request }) => {
      const url = new URL(request.url);
      if (url.searchParams.get("q") === "needle") searches.push(url);
      return HttpResponse.json({ items: [] });
    }),
  );
  renderApp(`/apps/${appID}/traces`);
  const user = userEvent.setup();

  await user.type(await screen.findByLabelText("Search"), "needle");
  await user.selectOptions(screen.getByRole("combobox", { name: "Status" }), "error");

  await waitFor(() =>
    expect(searches.some((url) => url.searchParams.get("status") === "error")).toBe(true),
  );
  expect(searches.every((url) => url.searchParams.get("status") === "error")).toBe(true);
});

test("links score summaries to their captured trace evidence", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => HttpResponse.json(traceDetailFixture())),
    http.get("*/v1/traces", ({ request }) => {
      const url = new URL(request.url);
      expect(url.searchParams.get("q")).toBe("needle");
      expect(url.searchParams.get("scorer")).toBe("groundedness");
      expect(url.searchParams.get("passed")).toBe("false");
      return HttpResponse.json({
        items: [
          {
            ...traceFixture("needle operation"),
            score_summaries: [
              {
                scorer: "groundedness",
                value: 0.2,
                threshold: 0.7,
                passed: false,
                created_at: "2026-09-01T10:00:01Z",
              },
            ],
          },
        ],
      });
    }),
  );
  renderApp(
    `/apps/${appID}/traces?q=needle&scorer=groundedness&passed=false&range=custom&start=2026-09-01T00%3A00%3A00.000Z&end=2026-09-02T00%3A00%3A00.000Z`,
  );
  const user = userEvent.setup();

  expect(await screen.findByText("needle operation")).toBeInTheDocument();
  const evidence = screen.getByRole("link", { name: "View groundedness score evidence" });
  expect(evidence).toHaveAttribute(
    "href",
    `/apps/${appID}/traces/${traceID}?tab=scores&scorer=groundedness`,
  );
  expect(screen.getByRole("combobox", { name: "Pass" })).toHaveValue("false");
  await user.click(evidence);

  expect(await screen.findByRole("tab", { name: "Scores" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  expect(screen.getByText("Captured evidence")).toBeInTheDocument();
  expect(screen.getByText(/Where are traces stored/)).toBeInTheDocument();
});

test("aborts stale trace requests when the application changes", async () => {
  let firstAborted = false;
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", async ({ request }) => {
      const selected = new URL(request.url).searchParams.get("application_id");
      if (selected === appID) {
        request.signal.addEventListener("abort", () => {
          firstAborted = true;
        });
        await delay(80);
        return HttpResponse.json({ items: [traceFixture("stale operation")] });
      }
      return HttpResponse.json({ items: [traceFixture("current operation")] });
    }),
  );
  renderApp(`/apps/${appID}/traces`);
  const user = userEvent.setup();

  await user.selectOptions(
    await screen.findByRole("combobox", { name: "Application" }),
    otherAppID,
  );

  expect(await screen.findByText("current operation")).toBeInTheDocument();
  expect(screen.queryByText("stale operation")).not.toBeInTheDocument();
  await waitFor(() => expect(firstAborted).toBe(true));
});

test("aborts an active load-more request on unmount", async () => {
  let loadMoreStarted = false;
  let loadMoreAborted = false;
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", async ({ request }) => {
      if (new URL(request.url).searchParams.get("cursor") === "cursor-one") {
        loadMoreStarted = true;
        request.signal.addEventListener("abort", () => {
          loadMoreAborted = true;
        });
        await delay(80);
        return HttpResponse.json({ items: [traceFixture("late operation")] });
      }
      return HttpResponse.json({
        items: [traceFixture("first operation")],
        next_cursor: "cursor-one",
      });
    }),
  );
  const { unmount } = render(
    <MemoryRouter initialEntries={[`/apps/${appID}/traces`]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Load more" }));
  await waitFor(() => expect(loadMoreStarted).toBe(true));
  unmount();

  await waitFor(() => expect(loadMoreAborted).toBe(true));
});

test("shows loading and empty trace states", async () => {
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", async () => {
      await delay(40);
      return HttpResponse.json({ items: [] });
    }),
  );
  renderApp(`/apps/${appID}/traces`);

  expect(await screen.findByText("Loading traces...")).toBeInTheDocument();
  expect(await screen.findByText("No traces found.")).toBeInTheDocument();
});

test("does not present a failed trace request as an empty result", async () => {
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", () =>
      HttpResponse.json({ title: "Storage unavailable" }, { status: 500 }),
    ),
  );
  renderApp(`/apps/${appID}/traces`);

  expect(await screen.findByRole("alert")).toHaveTextContent("Storage unavailable");
  expect(screen.queryByText("No traces found.")).not.toBeInTheDocument();
});

test("inspects nested spans, scores, and JSON as text", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => HttpResponse.json(traceDetailFixture())),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  const root = await screen.findByRole("treeitem", { name: /root span/ });
  expect(root).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("tree").parentElement).toHaveClass("bg-surface");
  const overview = screen.getByRole("tab", { name: "Overview" });
  overview.focus();
  await user.keyboard("{End}");
  expect(screen.getByRole("tab", { name: "Scores" })).toHaveFocus();
  expect(screen.getByText("Whole trace score")).toBeInTheDocument();
  expect(screen.getByText("Supported by context")).toBeInTheDocument();
  const child = screen.getByRole("treeitem", { name: /child span/ });
  root.focus();
  await user.keyboard("{ArrowDown}");
  expect(child).toHaveFocus();
  await user.keyboard("{Enter}");
  expect(screen.getByText("0.92")).toBeInTheDocument();
  expect(screen.queryByText("Whole trace score")).not.toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "Attributes" }));

  expect(screen.getByText(/<img src=x onerror=alert/)).toBeInTheDocument();
  expect(document.querySelector('img[src="x"]')).toBeNull();
  await user.click(screen.getByRole("tab", { name: "Events" }));
  expect(screen.getByText(/<script>unsafe event/)).toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "Scores" }));
  expect(screen.getByText("Passed")).toBeInTheDocument();
  expect(screen.getByText("Supported by context")).toBeInTheDocument();
  expect(screen.getByText("Threshold 0.7")).toBeInTheDocument();
  expect(screen.getByText("openai / judge")).toBeInTheDocument();
  expect(screen.getByText(/"citations"/)).toBeInTheDocument();
});

test("shows captured child-span content in the trace overview", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => HttpResponse.json(traceDetailFixture())),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  expect(await screen.findByText("Where are traces stored?")).toBeInTheDocument();
  expect(screen.getByText("In Postgres.")).toBeInTheDocument();
  expect(screen.getByText(/Assay uses Postgres/)).toBeInTheDocument();
  expect(screen.getByText(/demo-model/)).toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("treeitem", { name: /child span/ }));
  expect(screen.getByText("Where are traces stored?")).toBeInTheDocument();
});

test("selects a conversation source and renders its timing row", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => HttpResponse.json(traceDetailFixture())),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  await user.click(
    await screen.findByRole("button", { name: "View source for assistant message" }),
  );
  expect(screen.getByRole("img", { name: "Timeline for child span" })).toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "Scores" }));
  expect(screen.getByText("0.92")).toBeInTheDocument();
  expect(screen.queryByText("Whole trace score")).not.toBeInTheDocument();
});

test("edits a reference and refreshes trace actions", async () => {
  let reads = 0;
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}/scoring-eligibility`, () =>
      HttpResponse.json({
        items: [
          { scorer: "groundedness", eligible: true, reasons: [] },
          { scorer: "correctness", eligible: true, reasons: [] },
        ],
      }),
    ),
    http.get(`*/v1/traces/${traceID}`, () => {
      reads += 1;
      return HttpResponse.json({
        ...traceDetailFixture(),
        reference_answer: reads > 1 ? "Expected answer" : undefined,
      });
    }),
    http.patch(`*/v1/traces/${traceID}/reference`, async ({ request }) => {
      expect(await request.json()).toEqual({ reference_answer: "Expected answer" });
      return HttpResponse.json(traceDetailFixture());
    }),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Edit reference" }));
  await user.type(screen.getByLabelText("Reference answer"), "Expected answer");
  await user.click(screen.getByRole("button", { name: "Save reference" }));

  await waitFor(() => expect(reads).toBeGreaterThan(1));
});

test("keeps accepted scoring tasks visible and refreshable", async () => {
  let reads = 0;
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => {
      reads += 1;
      return HttpResponse.json({
        ...traceDetailFixture(),
        scoring_tasks:
          reads > 1
            ? [
                {
                  id: "019d11d2-cbd3-7a5e-ae83-9b791c9329ab",
                  trace_id: traceID,
                  scorer: "groundedness",
                  status: "pending",
                },
              ]
            : [],
      });
    }),
    http.post("*/v1/traces/score", async ({ request }) => {
      expect(await request.json()).toEqual({ trace_ids: [traceID], scorers: ["groundedness"] });
      return HttpResponse.json(
        {
          items: [
            {
              id: "019d11d2-cbd3-7a5e-ae83-9b791c9329ab",
              trace_id: traceID,
              scorer: "groundedness",
              status: "pending",
            },
          ],
        },
        { status: 202 },
      );
    }),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Score" }));
  await user.click(screen.getByRole("button", { name: "Queue score" }));

  expect(await screen.findByRole("status")).toHaveTextContent("Queued scoring tasks");
  expect(screen.getByRole("status")).toHaveTextContent("groundedness — pending");
  await user.click(screen.getByRole("button", { name: "Close" }));
  expect(await screen.findByRole("heading", { name: "Scoring tasks" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Refresh trace" }));
  await waitFor(() => expect(reads).toBeGreaterThan(2));
});

test("keeps accepted tasks visible when their initial refresh fails", async () => {
  let reads = 0;
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => {
      reads += 1;
      return reads === 1
        ? HttpResponse.json(traceDetailFixture())
        : HttpResponse.json(
            { title: "Unavailable", detail: "The trace could not refresh." },
            { status: 503 },
          );
    }),
    http.post("*/v1/traces/score", () =>
      HttpResponse.json(
        {
          items: [
            {
              id: "019d11d2-cbd3-7a5e-ae83-9b791c9329ab",
              trace_id: traceID,
              scorer: "groundedness",
              status: "pending",
            },
          ],
        },
        { status: 202 },
      ),
    ),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Score" }));
  await user.click(screen.getByRole("button", { name: "Queue score" }));

  expect(await screen.findByRole("status")).toHaveTextContent("groundedness — pending");
  expect(screen.getByRole("alert")).toHaveTextContent("Scoring was queued");
  expect(screen.queryByRole("button", { name: "Queue score" })).not.toBeInTheDocument();
});

test("shows the chronologically latest score for each scorer", async () => {
  const trace = traceDetailFixture();
  const groundedness = trace.scores[0];
  if (groundedness === undefined) throw new Error("missing groundedness fixture");
  trace.scores = [
    { ...groundedness, id: 99, value: 0.9, created_at: "2026-09-01T10:00:00Z" },
    { ...groundedness, id: 1, value: 0.2, created_at: "2026-09-01T10:01:00Z" },
  ];
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => HttpResponse.json(trace)),
    http.get("*/v1/datasets", () =>
      HttpResponse.json({ items: [datasetFixture("dataset-1", "First")] }),
    ),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Save to dataset" }));

  expect(screen.getByRole("option", { name: "groundedness (0.2)" })).toBeInTheDocument();
  expect(screen.queryByRole("option", { name: "groundedness (0.9)" })).not.toBeInTheDocument();
});

test("paginates application datasets and saves retained evidence", async () => {
  const datasetRequests: URL[] = [];
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => HttpResponse.json(traceDetailFixture())),
    http.get("*/v1/datasets", ({ request }) => {
      const url = new URL(request.url);
      datasetRequests.push(url);
      expect(url.searchParams.get("application_id")).toBe(appID);
      if (url.searchParams.get("cursor") === null) {
        return HttpResponse.json({
          items: [datasetFixture("dataset-1", "First")],
          next_cursor: "next-page",
        });
      }
      expect(url.searchParams.get("cursor")).toBe("next-page");
      return HttpResponse.json({ items: [datasetFixture("dataset-2", "Second")] });
    }),
    http.post("*/v1/datasets/dataset-2/from-trace", async ({ request }) => {
      expect(await request.json()).toEqual({ trace_id: traceID, scorer: "groundedness" });
      return HttpResponse.json(datasetItemFixture("dataset-2"), { status: 201 });
    }),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Save to dataset" }));
  expect(await screen.findByRole("option", { name: "Second" })).toBeInTheDocument();
  expect(datasetRequests).toHaveLength(2);
  await user.selectOptions(screen.getByRole("combobox", { name: "Dataset" }), "dataset-2");
  await user.selectOptions(screen.getByRole("combobox", { name: "Latest score" }), "groundedness");
  await user.click(screen.getByRole("button", { name: "Save evidence" }));

  expect(await screen.findByRole("link", { name: "Open dataset" })).toHaveAttribute(
    "href",
    `/apps/${appID}/datasets/dataset-2`,
  );
});

test("shows score import conflicts in the save dialog", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () => HttpResponse.json(traceDetailFixture())),
    http.get("*/v1/datasets", () =>
      HttpResponse.json({ items: [datasetFixture("dataset-1", "First")] }),
    ),
    http.post("*/v1/datasets/dataset-1/from-trace", () =>
      HttpResponse.json(
        { title: "Conflict", detail: "Evidence is already imported" },
        { status: 409 },
      ),
    ),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Save to dataset" }));
  await user.click(screen.getByRole("button", { name: "Save evidence" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("Evidence is already imported");
  expect(screen.queryByRole("link", { name: "Open dataset" })).not.toBeInTheDocument();
});

test("rejects a trace from a different application", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/traces/${traceID}`, () =>
      HttpResponse.json({ ...traceDetailFixture(), application_id: otherAppID }),
    ),
  );
  renderApp(`/apps/${appID}/traces/${traceID}`);

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Trace does not belong to this application",
  );
  expect(screen.queryByRole("tree", { name: "Spans" })).not.toBeInTheDocument();
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

function applicationHandler() {
  return http.get("*/v1/applications", () =>
    HttpResponse.json({
      items: [applicationFixture(appID, "Primary"), applicationFixture(otherAppID, "Other")],
    }),
  );
}

function applicationFixture(id: string, name: string) {
  return {
    id,
    project_id: id,
    name,
    slug: name.toLowerCase(),
    auto_score_scorers: [],
    config: {},
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}

function traceFixture(rootName: string, id = traceID) {
  return {
    id,
    application_id: appID,
    otel_trace_id: "01",
    root_name: rootName,
    start_time: "2026-09-01T10:00:00Z",
    end_time: "2026-09-01T10:00:01Z",
    status: "ok",
    span_count: 2,
    total_tokens: 42,
    attributes: {},
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:01Z",
  };
}

function datasetFixture(id: string, name: string) {
  return {
    id,
    application_id: appID,
    name,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}

function datasetItemFixture(datasetID: string) {
  return {
    id: "019d11d2-cbd3-7a5e-ae83-9b791c9329ac",
    dataset_id: datasetID,
    input: { question: "question" },
    output: "answer",
    context: [],
    metadata: {},
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}

function traceDetailFixture() {
  return {
    ...traceFixture("root span"),
    spans: [
      {
        id: 1,
        otel_span_id: "01",
        name: "root span",
        kind: "server",
        start_time: "2026-09-01T10:00:00Z",
        end_time: "2026-09-01T10:00:01Z",
        duration_ms: 1000,
        status_code: "ok",
        is_scorable: false,
        attributes: {},
        events: [],
        input_tokens: 0,
        output_tokens: 0,
        children: [
          {
            id: 2,
            otel_span_id: "02",
            name: "child span",
            kind: "client",
            start_time: "2026-09-01T10:00:00Z",
            end_time: "2026-09-01T10:00:01Z",
            duration_ms: 500,
            status_code: "ok",
            is_scorable: true,
            attributes: {
              unsafe: "<img src=x onerror=alert(1)>",
              "gen_ai.input.messages": JSON.stringify([
                { role: "user", content: "Where are traces stored?" },
              ]),
              "gen_ai.output.messages": [{ role: "assistant", content: "In Postgres." }],
              "gen_ai.retrieval.documents": JSON.stringify([
                { id: "storage", text: "Assay uses Postgres." },
              ]),
              "gen_ai.request.model": "demo-model",
            },
            events: [
              {
                name: "<script>unsafe event</script>",
                time: "2026-09-01T10:00:00Z",
                attributes: {},
                dropped_attributes_count: 0,
              },
            ],
            input_tokens: 20,
            output_tokens: 22,
            children: [],
          },
        ],
      },
    ],
    scores: [
      {
        id: 1,
        scorer: "groundedness",
        value: 0.92,
        threshold: 0.7,
        passed: true,
        rationale: "Supported by context",
        details: { citations: 2 },
        prompt_template_id: "groundedness-v1",
        judge_model: "judge",
        judge_provider: "openai",
        judge_tokens: 10,
        span_id: 2,
        judged_input: "Where are traces stored?",
        judged_output: "In Postgres.",
        judged_context: [{ id: "storage", text: "Assay uses Postgres." }],
        created_at: "2026-09-01T10:00:01Z",
      },
      {
        id: 2,
        scorer: "correctness",
        value: 0.75,
        threshold: 0.7,
        passed: true,
        rationale: "Whole trace score",
        details: {},
        prompt_template_id: "correctness-v1",
        judge_model: "judge",
        judge_provider: "openai",
        judge_tokens: 10,
        created_at: "2026-09-01T10:00:01Z",
      },
    ],
  };
}
