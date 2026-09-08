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
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("lists application traces and rejects a repeated cursor", async () => {
  let page = 0;
  server.use(
    applicationHandler(),
    http.get("*/v1/traces", ({ request }) => {
      const url = new URL(request.url);
      expect(url.searchParams.get("application_id")).toBe(appID);
      page++;
      return HttpResponse.json({
        items: [traceFixture(page === 1 ? "first operation" : "second operation")],
        next_cursor: "same-cursor",
      });
    }),
  );
  renderApp(`/apps/${appID}/traces`);
  const user = userEvent.setup();

  expect(await screen.findByText("first operation")).toBeInTheDocument();
  for (const heading of ["Start", "Operation", "Status", "Duration", "Spans", "Tokens"]) {
    expect(screen.getByRole("columnheader", { name: heading })).toBeInTheDocument();
  }
  await user.click(screen.getByRole("button", { name: "Load more" }));

  expect(await screen.findByText("second operation")).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent("repeated cursor");
  expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
  await user.type(screen.getByLabelText("Filter traces"), "second");
  expect(screen.queryByText("first operation")).not.toBeInTheDocument();
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
  expect(screen.getByText("Assay uses Postgres.")).toBeInTheDocument();
  expect(screen.getByText("demo-model")).toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("treeitem", { name: /child span/ }));
  expect(screen.getByText("Where are traces stored?")).toBeInTheDocument();
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

function traceFixture(rootName: string) {
  return {
    id: traceID,
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
