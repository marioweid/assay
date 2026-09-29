import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";
import { tokenModeServerInfo } from "@/test/server-info";

const appId = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const first = "019d11d2-cbd3-7a5e-ae83-9b791c9329aa";
const second = "019d11d2-cbd3-7a5e-ae83-9b791c9329ab";
const time = "2026-09-01T10:00:00Z";
const server = setupServer(
  tokenModeServerInfo,
  http.get("*/v1/sessions", () => HttpResponse.json({ items: [] })),
  http.get(`*/v1/traces/${second}`, () => HttpResponse.json({ ...sourceTrace(), id: second })),
  http.get("*/v1/applications", () =>
    HttpResponse.json({
      items: [
        {
          id: appId,
          project_id: appId,
          name: "Demo",
          slug: "demo",
          config: {},
          auto_score_scorers: [],
          created_at: time,
          updated_at: time,
        },
      ],
    }),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderApp(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
}

test("session list shows only backend-tagged sessions and links to detail", async () => {
  server.use(
    http.get("*/v1/sessions", ({ request }) => {
      expect(new URL(request.url).searchParams.get("application_id")).toBe(appId);
      return HttpResponse.json({
        items: [
          {
            id: "private-session",
            start_time: time,
            end_time: time,
            first_operation: "Answer question",
            last_trace_id: first,
            turn_count: 2,
          },
        ],
      });
    }),
  );
  renderApp(`/apps/${appId}/sessions`);
  expect(await screen.findByRole("link", { name: "Answer question" })).toHaveAttribute(
    "href",
    `/apps/${appId}/sessions/detail?session_id=private-session`,
  );
  expect(screen.getByText("2 turns")).toBeInTheDocument();
});

test("root turns exclude child history while timeline selects a trace", async () => {
  server.use(
    http.get("*/v1/session-turns", ({ request }) => {
      expect(new URL(request.url).searchParams.get("session_id")).toBe("private-session");
      expect(new URL(request.url).searchParams.get("application_id")).toBe(appId);
      return HttpResponse.json({
        items: [
          turn(first, "Question one", "Reply one", time),
          turn(second, "Question two", "Reply two", "2026-09-01T10:00:02Z"),
        ],
      });
    }),
    http.get(`*/v1/traces/${second}`, () =>
      HttpResponse.json({
        id: second,
        application_id: appId,
        root_name: "answer",
        span_count: 2,
        total_tokens: 12,
        status: "ok",
        scores: [],
        spans: [
          {
            id: 1,
            otel_span_id: "a1",
            name: "root",
            kind: "internal",
            start_time: time,
            end_time: "2026-09-01T10:00:03Z",
            duration_ms: 3000,
            attributes: {},
            children: [
              {
                id: 2,
                otel_span_id: "a2",
                name: "generation",
                kind: "client",
                start_time: time,
                end_time: "2026-09-01T10:00:01Z",
                duration_ms: 1000,
                attributes: {
                  "gen_ai.input.messages": JSON.stringify([
                    { role: "user", content: "Repeated history" },
                  ]),
                },
                children: [],
                input_tokens: 8,
                output_tokens: 4,
              },
            ],
            input_tokens: 0,
            output_tokens: 0,
          },
        ],
      }),
    ),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  const transcript = await screen.findByRole("list", { name: "Session turns" });
  expect(within(transcript).getByText("Question one")).toBeInTheDocument();
  expect(within(transcript).getByText("Reply two")).toBeInTheDocument();
  expect(within(transcript).queryByText("Repeated history")).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Timeline" }));
  await user.click(screen.getByRole("button", { name: /Turn 2 · answer/ }));
  expect(await screen.findByRole("link", { name: "Open full trace and scores →" })).toHaveAttribute(
    "href",
    `/apps/${appId}/traces/${second}`,
  );
  expect(screen.getByRole("region", { name: "Span timing" })).toBeInTheDocument();
});

test("inspector reveals model tools, context and scores", async () => {
  server.use(
    http.get("*/v1/session-turns", () =>
      HttpResponse.json({ items: [turn(first, "Current question", "Current answer", time)] }),
    ),
    http.get(`*/v1/traces/${first}`, () => HttpResponse.json(sourceTrace())),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  const transcript = await screen.findByRole("list", { name: "Session turns" });
  expect(within(transcript).queryByText("Prior model context")).not.toBeInTheDocument();
  await userEvent
    .setup()
    .click(
      within(transcript).getByRole("button", { name: /Inspect source for assistant message/ }),
    );
  const inspector = await screen.findByRole("complementary", { name: "Selected trace inspector" });
  expect(inspector).toHaveFocus();
  expect(
    within(inspector).getByText("groundedness", { selector: "summary span" }),
  ).toBeInTheDocument();
  expect(within(inspector).getByText("0.93", { selector: "summary strong" })).toBeInTheDocument();
  await userEvent.setup().click(within(inspector).getByText("Model calls and captured context"));
  expect(within(inspector).getByText("Prior model context")).toBeInTheDocument();
  expect(within(inspector).getByText(/Tool call: lookup/)).toBeInTheDocument();
  expect(within(inspector).getByText("Retrieved context")).toBeInTheDocument();
  await userEvent.setup().click(within(inspector).getByText("Document 1"));
  expect(within(inspector).getByText(/Assay stores traces in PostgreSQL/)).toBeInTheDocument();
  await userEvent
    .setup()
    .click(within(inspector).getByText("groundedness", { selector: "summary span" }));
  expect(within(inspector).getByText("Evidence supported by retrieval")).toBeInTheDocument();
  await userEvent.setup().click(within(inspector).getByRole("button", { name: /generate/ }));
  expect(within(inspector).getByRole("region", { name: "Selected span" })).toHaveTextContent(
    "2000.0 ms",
  );
  expect(
    within(inspector).getByRole("heading", { name: "Scores for generate · 1" }),
  ).toBeInTheDocument();
  expect(within(inspector).getByRole("button", { name: /generate/ })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await userEvent
    .setup()
    .click(within(inspector).getByRole("button", { name: "Show all spans and scores" }));
  expect(
    within(inspector).queryByRole("region", { name: "Selected span" }),
  ).not.toBeInTheDocument();
  expect(within(inspector).getByRole("heading", { name: "Trace scores · 1" })).toBeInTheDocument();
});

test("refreshes asynchronous scores without leaving the session", async () => {
  let reads = 0;
  server.use(
    http.get("*/v1/session-turns", () =>
      HttpResponse.json({ items: [turn(first, "Current question", "Current answer", time)] }),
    ),
    http.get(`*/v1/traces/${first}`, () => {
      reads += 1;
      return HttpResponse.json(reads === 1 ? { ...sourceTrace(), scores: [] } : sourceTrace());
    }),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: /Inspect source for assistant message/ }));
  const inspector = screen.getByRole("complementary", { name: "Selected trace inspector" });
  expect(
    await within(inspector).findByText("No scores recorded for this trace."),
  ).toBeInTheDocument();
  await userEvent
    .setup()
    .click(within(inspector).getByRole("button", { name: "Refresh source trace" }));
  expect(
    await within(inspector).findByText("groundedness", { selector: "summary span" }),
  ).toBeInTheDocument();
  expect(reads).toBe(2);
});

test("long session timeline zooms, scrolls and exposes timing as text", async () => {
  server.use(
    http.get("*/v1/session-turns", () =>
      HttpResponse.json({
        items: [
          turn(first, "First question", "First answer", time),
          turn(second, "Second question", "Second answer", "2026-09-01T10:00:02Z"),
        ],
      }),
    ),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  await screen.findByRole("list", { name: "Session turns" });
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Timeline" }));
  const timeline = screen.getByRole("region", { name: "Session timeline" });
  const duration = (2000).toLocaleString();
  expect(
    within(timeline).getByText(`${duration} ms start, ${duration} ms duration`),
  ).toBeInTheDocument();
  expect(within(timeline).getByText("1×")).toBeInTheDocument();
  await user.click(within(timeline).getByRole("button", { name: "Zoom in" }));
  expect(within(timeline).getByText("2×")).toBeInTheDocument();
  expect(within(timeline).getByTestId("timeline-content").style.minWidth).toBe("68rem");
  await user.click(within(timeline).getByRole("button", { name: "Zoom in" }));
  expect(within(timeline).getByRole("button", { name: "Zoom in" })).toBeDisabled();
  await user.click(within(timeline).getByRole("button", { name: /Turn 2 · answer/ }));
  expect(screen.getByRole("complementary", { name: "Selected trace inspector" })).toHaveFocus();
});

test("a delayed trace read cannot overwrite a newer selected source", async () => {
  let resolveFirst: ((response: Response) => void) | undefined;
  const firstResponse = new Promise<Response>((resolve) => {
    resolveFirst = resolve;
  });
  let firstRequested = false;
  let firstSettled = false;
  server.use(
    http.get("*/v1/session-turns", () =>
      HttpResponse.json({
        items: [
          turn(first, "First", "Reply", time),
          turn(second, "Second", "Reply", "2026-09-01T10:00:02Z"),
        ],
      }),
    ),
    http.get(`*/v1/traces/${first}`, async () => {
      firstRequested = true;
      const response = await firstResponse;
      firstSettled = true;
      return response;
    }),
    http.get(`*/v1/traces/${second}`, () =>
      HttpResponse.json({ ...sourceTrace(), id: second, root_name: "Second source" }),
    ),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  const transcript = await screen.findByRole("list", { name: "Session turns" });
  const [firstButton, secondButton] = within(transcript).getAllByRole("button", {
    name: /Inspect source for assistant message/,
  });
  if (!firstButton || !secondButton) throw new Error("Missing turn selectors");
  const user = userEvent.setup();
  await user.click(firstButton);
  await waitFor(() => expect(firstRequested).toBe(true));
  await user.click(secondButton);
  const inspector = screen.getByRole("complementary", { name: "Selected trace inspector" });
  expect(
    await within(inspector).findByRole("heading", { name: "Second source" }),
  ).toBeInTheDocument();
  resolveFirst?.(HttpResponse.json(sourceTrace()));
  await waitFor(() => expect(firstSettled).toBe(true));
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(within(inspector).getByRole("heading", { name: "Second source" })).toBeInTheDocument();
  expect(
    within(inspector).getByRole("link", { name: "Open full trace and scores →" }),
  ).toHaveAttribute("href", `/apps/${appId}/traces/${second}`);
});

test("paginated turns retain timeline zoom and do not duplicate rows", async () => {
  server.use(
    http.get("*/v1/session-turns", ({ request }) => {
      const cursor = new URL(request.url).searchParams.get("cursor");
      return HttpResponse.json(
        cursor === null
          ? { items: [turn(first, "First", "Reply", time)], next_cursor: "page-two" }
          : {
              items: [
                turn(first, "First", "Reply", time),
                turn(second, "Second", "Reply", "2026-09-01T10:00:02Z"),
              ],
            },
      );
    }),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  await screen.findByRole("list", { name: "Session turns" });
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Timeline" }));
  await user.click(screen.getByRole("button", { name: "Zoom in" }));
  await user.click(screen.getByRole("button", { name: "Load more turns" }));
  expect(await screen.findByText("2 loaded turns")).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "Session timeline" })).toHaveTextContent("2×");
  expect(screen.getAllByRole("button", { name: /Turn [12] · answer/ })).toHaveLength(2);
});

test("pending and failed scoring tasks are visible before scores arrive", async () => {
  server.use(
    http.get("*/v1/session-turns", () =>
      HttpResponse.json({ items: [turn(first, "Question", "Answer", time)] }),
    ),
    http.get(`*/v1/traces/${first}`, () =>
      HttpResponse.json({
        ...sourceTrace(),
        scores: [],
        scoring_tasks: [
          { id: "task-1", trace_id: first, scorer: "groundedness", status: "pending" },
          {
            id: "task-2",
            trace_id: first,
            scorer: "correctness",
            status: "failed",
            error: "Reference answer missing",
          },
        ],
      }),
    ),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: /Inspect source for assistant message/ }));
  const scores = await screen.findByRole("region", { name: "Source scores" });
  expect(within(scores).getByText("Scoring groundedness: pending")).toBeInTheDocument();
  expect(
    within(scores).getByText(/Scoring correctness: failed.*Reference answer missing/),
  ).toBeInTheDocument();
});

test("malformed turn remains inspectable after a trace fetch failure", async () => {
  server.use(
    http.get("*/v1/session-turns", () =>
      HttpResponse.json({
        items: [
          {
            ...turn(first, "unused", "unused", time),
            status: "error",
            attributes: { "gen_ai.input.messages": "{broken" },
          },
        ],
      }),
    ),
    http.get(`*/v1/traces/${first}`, () =>
      HttpResponse.json({ title: "Trace unavailable" }, { status: 503 }),
    ),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  expect(await screen.findByText(/Captured messages could not be parsed/)).toBeInTheDocument();
  expect(screen.getByText("This turn ended with an error.")).toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "Inspect source trace" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Trace unavailable");
  expect(screen.getByRole("link", { name: /Full trace →/ })).toHaveAttribute(
    "href",
    `/apps/${appId}/traces/${first}`,
  );
});

test("opaque session IDs remain distinct in routes", async () => {
  const requested: string[] = [];
  server.use(
    http.get("*/v1/sessions", () =>
      HttpResponse.json({
        items: [
          {
            id: "a%2Fb",
            start_time: time,
            end_time: time,
            first_operation: "Literal percent",
            last_trace_id: first,
            turn_count: 1,
          },
          {
            id: "a/b",
            start_time: time,
            end_time: time,
            first_operation: "Slash",
            last_trace_id: second,
            turn_count: 1,
          },
          {
            id: "..",
            start_time: time,
            end_time: time,
            first_operation: "Dots",
            last_trace_id: first,
            turn_count: 1,
          },
        ],
      }),
    ),
    http.get("*/v1/session-turns", ({ request }) => {
      const id = new URL(request.url).searchParams.get("session_id") ?? "";
      requested.push(id);
      return HttpResponse.json({ items: [turn(first, id, "reply", time)] });
    }),
  );
  renderApp(`/apps/${appId}/sessions`);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("link", { name: "Literal percent" }));
  expect(await screen.findByRole("list", { name: "Session turns" })).toHaveTextContent("a%2Fb");
  expect(requested).toEqual(["a%2Fb"]);
  await user.click(screen.getByRole("link", { name: "← All sessions" }));
  await user.click(await screen.findByRole("link", { name: "Dots" }));
  expect(await screen.findByRole("list", { name: "Session turns" })).toHaveTextContent("..");
  expect(requested).toEqual(["a%2Fb", ".."]);
});

test("empty and failed session reads provide actionable states", async () => {
  server.use(http.get("*/v1/sessions", () => HttpResponse.json({ items: [] })));
  const view = renderApp(`/apps/${appId}/sessions`);
  expect(await screen.findByText("Your traces are still here.")).toBeInTheDocument();
  view.unmount();
  server.use(
    http.get("*/v1/sessions", () => HttpResponse.json({ title: "Unavailable" }, { status: 503 })),
  );
  renderApp(`/apps/${appId}/sessions`);
  expect(await screen.findByRole("alert")).toBeInTheDocument();
});

test("recent-session navigation clears the old source and keeps loaded counts honest", async () => {
  server.use(
    http.get("*/v1/sessions", () =>
      HttpResponse.json({
        items: [
          {
            id: "first-session",
            first_operation: "First session",
            start_time: time,
            end_time: time,
            last_trace_id: first,
            turn_count: 2,
          },
          {
            id: "second-session",
            first_operation: "Second session",
            start_time: time,
            end_time: time,
            last_trace_id: second,
            turn_count: 1,
          },
        ],
      }),
    ),
    http.get("*/v1/session-turns", ({ request }) =>
      HttpResponse.json(
        new URL(request.url).searchParams.get("session_id") === "first-session"
          ? { items: [turn(first, "First question", "First answer", time)], next_cursor: "more" }
          : { items: [turn(second, "Second question", "Second answer", time)] },
      ),
    ),
    http.get(`*/v1/traces/${first}`, () => HttpResponse.json(sourceTrace())),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=first-session`);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: /First answer/ }));
  const inspector = screen.getByRole("complementary", { name: "Selected trace inspector" });
  expect(await within(inspector).findByText("Tokens used")).toBeInTheDocument();
  const summary = screen.getByRole("region", { name: "Session summary" });
  expect(summary).toHaveTextContent("1 loaded turn");
  expect(summary).toHaveTextContent("2 loaded messages");
  expect(summary).toHaveTextContent("last loaded turn");
  await user.click(screen.getByRole("link", { name: "Second session" }));
  expect(await screen.findByRole("button", { name: /Second answer/ })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /First answer/ })).not.toBeInTheDocument();
  expect(screen.queryByText("Tokens used")).not.toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Second session" })).toHaveAttribute(
    "aria-current",
    "page",
  );
});

test("recent sessions can retry a failure without losing the selected conversation", async () => {
  let fail = true;
  server.use(
    http.get("*/v1/sessions", () =>
      fail
        ? HttpResponse.json({ title: "Recent sessions unavailable" }, { status: 503 })
        : HttpResponse.json({ items: [] }),
    ),
    http.get("*/v1/session-turns", () =>
      HttpResponse.json({
        items: [turn(first, "Keep this question", "Keep this answer", time)],
      }),
    ),
  );
  renderApp(`/apps/${appId}/sessions/detail?session_id=private-session`);
  expect(await screen.findByRole("alert")).toHaveTextContent("Recent sessions unavailable");
  fail = false;
  await userEvent.setup().click(screen.getByRole("button", { name: "Refresh sessions" }));
  expect(await screen.findByText(/No recent sessions/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Keep this question/ })).toBeInTheDocument();
});

function sourceTrace() {
  return {
    id: first,
    start_time: time,
    end_time: "2026-09-01T10:00:04Z",
    application_id: appId,
    root_name: "answer",
    status: "ok",
    span_count: 2,
    total_tokens: 18,
    attributes: {},
    spans: [
      {
        id: 1,
        otel_span_id: "root",
        name: "answer",
        kind: "internal",
        start_time: time,
        end_time: "2026-09-01T10:00:04Z",
        duration_ms: 4000,
        status_code: "ok",
        input_tokens: 0,
        output_tokens: 0,
        is_scorable: false,
        attributes: {},
        events: [],
        children: [
          {
            id: 2,
            otel_span_id: "model",
            name: "generate",
            kind: "client",
            start_time: "2026-09-01T10:00:01Z",
            end_time: "2026-09-01T10:00:03Z",
            duration_ms: 2000,
            status_code: "ok",
            input_tokens: 12,
            output_tokens: 6,
            is_scorable: true,
            events: [],
            children: [],
            attributes: {
              "gen_ai.request.model": "local-model",
              "gen_ai.input.messages": JSON.stringify([
                { role: "user", content: "Prior model context" },
              ]),
              "gen_ai.output.messages": JSON.stringify([
                {
                  role: "assistant",
                  parts: [
                    { type: "text", content: "Model answer" },
                    { type: "tool_call", name: "lookup", arguments: { source: "docs" } },
                  ],
                },
              ]),
              "gen_ai.retrieval.documents": JSON.stringify([
                { id: "postgres", text: "Assay stores traces in PostgreSQL." },
              ]),
            },
          },
        ],
      },
    ],
    scores: [
      {
        id: 10,
        span_id: 2,
        scorer: "groundedness",
        passed: true,
        value: 0.93,
        threshold: 0.7,
        rationale: "Evidence supported by retrieval",
        details: {},
        judge_provider: "local",
        judge_model: "mistral",
        judge_tokens: 20,
        prompt_template_id: "groundedness-v1",
        created_at: time,
      },
    ],
  };
}

function turn(id: string, question: string, answer: string, start: string) {
  return {
    id,
    root_name: "answer",
    start_time: start,
    end_time: "2026-09-01T10:00:04Z",
    status: "ok",
    span_count: 2,
    total_tokens: 12,
    attributes: {
      "gen_ai.input.messages": JSON.stringify([{ role: "user", content: question }]),
      "gen_ai.output.messages": JSON.stringify([{ role: "assistant", content: answer }]),
    },
  };
}
