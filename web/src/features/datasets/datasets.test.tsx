import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const appID = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const otherAppID = "019d11d2-cbd3-7a5e-ae83-9b791c9329df";
const datasetID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("lists application datasets and navigates to a dataset", async () => {
  server.use(
    applicationHandler(),
    http.get("*/v1/datasets", ({ request }) => {
      expect(new URL(request.url).searchParams.get("application_id")).toBe(appID);
      return HttpResponse.json({ items: [datasetFixture()] });
    }),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [] })),
  );
  renderApp(`/apps/${appID}/datasets`);
  const user = userEvent.setup();

  expect(await screen.findByText("No description")).toBeInTheDocument();
  expect(screen.getByRole("table").parentElement).toHaveClass("bg-surface");
  expect(screen.getByRole("link", { name: "Regression cases" })).toHaveClass("text-accent");
  await user.click(screen.getByRole("link", { name: "Regression cases" }));
  expect(await screen.findByRole("heading", { name: "Regression cases" })).toBeInTheDocument();
  expect(screen.getByText("No dataset items yet.")).toBeInTheDocument();
});

test("creates a dataset in the selected application and opens it", async () => {
  server.use(
    applicationHandler(),
    http.get("*/v1/datasets", () => HttpResponse.json({ items: [] })),
    http.post("*/v1/datasets", async ({ request }) => {
      expect(await request.json()).toEqual({
        application_id: appID,
        name: "Regression cases",
        description: "Chat checks",
      });
      return HttpResponse.json(datasetFixture(), { status: 201 });
    }),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [] })),
  );
  renderApp(`/apps/${appID}/datasets`);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Create dataset" }));
  expect(screen.getByRole("button", { name: "Save dataset" })).toBeDisabled();
  await user.type(screen.getByLabelText("Dataset name"), "  Regression cases  ");
  await user.type(screen.getByLabelText("Description (optional)"), "Chat checks");
  await user.click(screen.getByRole("button", { name: "Save dataset" }));
  expect(await screen.findByRole("heading", { name: "Regression cases" })).toBeInTheDocument();
});

test("preserves dataset input after a server error and allows cancellation", async () => {
  server.use(
    applicationHandler(),
    http.get("*/v1/datasets", () => HttpResponse.json({ items: [] })),
    http.post("*/v1/datasets", () =>
      HttpResponse.json({ title: "Storage unavailable" }, { status: 500 }),
    ),
  );
  renderApp(`/apps/${appID}/datasets`);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Create dataset" }));
  await user.type(screen.getByLabelText("Dataset name"), "My cases");
  await user.click(screen.getByRole("button", { name: "Save dataset" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Storage unavailable");
  expect(screen.getByLabelText("Dataset name")).toHaveValue("My cases");
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.queryByLabelText("Dataset name")).not.toBeInTheDocument();
});

test("browses item fields and stops on a repeated cursor", async () => {
  let page = 0;
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, ({ request }) => {
      page++;
      if (page === 2) expect(new URL(request.url).searchParams.get("cursor")).toBe("same-cursor");
      return HttpResponse.json({
        items: [itemFixture(page === 1 ? "case-one" : "case-two")],
        next_cursor: "same-cursor",
      });
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByText("case-one"));
  for (const label of ["Input", "Output", "Expected output", "Context", "Metadata"]) {
    expect(screen.getByRole("heading", { name: label })).toBeInTheDocument();
  }
  expect(screen.getAllByText("Not provided")).toHaveLength(2);
  expect(screen.getByText(/<script>dataset input/)).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
  await user.click(screen.getAllByRole("button", { name: "Copy JSON" })[0]!);
  expect(await screen.findByText("Copied")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Load more items" }));

  expect(await screen.findByText("case-two")).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent("repeated cursor");
  expect(screen.queryByRole("button", { name: "Load more items" })).not.toBeInTheDocument();
});

test("updates dataset metadata without changing its cases", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [] })),
    http.patch(`*/v1/datasets/${datasetID}`, async ({ request }) => {
      expect(await request.json()).toEqual({ clear_description: true, name: "Updated cases" });
      return HttpResponse.json({
        ...datasetFixture(),
        description: undefined,
        name: "Updated cases",
      });
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Edit dataset" }));
  await user.clear(screen.getByLabelText("Dataset name"));
  await user.type(screen.getByLabelText("Dataset name"), "Updated cases");
  await user.clear(screen.getByLabelText("Description (optional)"));
  await user.click(screen.getByRole("button", { name: "Save dataset" }));

  expect(await screen.findByRole("heading", { name: "Updated cases" })).toBeInTheDocument();
  expect(screen.getByText("No description")).toBeInTheDocument();
});

test("adds an evaluation item and displays it without reloading", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [] })),
    http.post(`*/v1/datasets/${datasetID}/items`, async ({ request }) => {
      expect(await request.json()).toEqual({
        items: [
          {
            input: { question: "Where are traces stored?" },
            output: "Postgres",
            expected_output: "Postgres",
            context: [{ id: "context-1", text: "Assay stores traces in Postgres." }],
          },
        ],
      });
      return HttpResponse.json(
        {
          items: [{ ...itemFixture("new-case"), input: { question: "Where are traces stored?" } }],
        },
        { status: 201 },
      );
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Add item" }));
  expect(screen.getByLabelText("Question")).toHaveFocus();
  await user.tab({ shift: true });
  expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();
  await user.tab();
  expect(screen.getByLabelText("Question")).toHaveFocus();
  expect(screen.getByRole("button", { name: "Save item" })).toBeDisabled();
  await user.type(screen.getByLabelText("Question"), "Where are traces stored?");
  await user.type(screen.getByLabelText("Recorded answer (optional)"), "Postgres");
  await user.type(screen.getByLabelText("Expected answer (optional)"), "Postgres");
  await user.type(
    screen.getByLabelText("Supporting context (optional)"),
    "Assay stores traces in Postgres.",
  );
  await user.click(screen.getByRole("button", { name: "Save item" }));
  await user.click(await screen.findByText("new-case"));
  expect(screen.getByText(/Where are traces stored/)).toBeInTheDocument();
  expect(screen.queryByText("No dataset items yet.")).not.toBeInTheDocument();
});

test("edits a case without losing its other editable fields", async () => {
  const item = {
    ...itemFixture("case-one"),
    context: [{ id: "doc-1", text: "Supporting context" }],
    expected_output: "Expected answer",
    external_id: "case-one",
    input: { language: "en", question: "Original question" },
    metadata: { priority: 1 },
    output: "Recorded answer",
  };
  let requestBody: unknown;
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [item] })),
    http.put(`*/v1/datasets/${datasetID}/items/${item.id}`, async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json({ ...item, input: { ...item.input, question: "Updated question" } });
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Edit case-one" }));
  const dialog = screen.getByRole("dialog", { name: "Edit dataset case" });
  const question = within(dialog).getByLabelText("Question");
  await user.clear(question);
  await user.type(question, "Updated question");
  await user.click(within(dialog).getByRole("button", { name: "Save item" }));

  expect(requestBody).toEqual({
    context: [{ id: "doc-1", text: "Supporting context" }],
    expected_output: "Expected answer",
    external_id: "case-one",
    input: { language: "en", question: "Updated question" },
    metadata: { priority: 1 },
    output: "Recorded answer",
  });
});

test("edits advanced input and metadata JSON without discarding question", async () => {
  const item = { ...itemFixture("case-one"), input: { language: "en", question: "Question" } };
  let requestBody: unknown;
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [item] })),
    http.put(`*/v1/datasets/${datasetID}/items/${item.id}`, async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json(item);
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Edit case-one" }));
  fireEvent.change(screen.getByLabelText("Input JSON"), { target: { value: '{"language":"fr"}' } });
  fireEvent.change(screen.getByLabelText("Metadata JSON"), { target: { value: '{"priority":2}' } });
  await user.click(screen.getByRole("button", { name: "Save item" }));

  expect(requestBody).toMatchObject({
    input: { language: "fr", question: "Question" },
    metadata: { priority: 2 },
  });
});

test("clears a recorded answer while preserving the rest of a case", async () => {
  const item = {
    ...itemFixture("case-one"),
    input: { question: "Question" },
    output: "Recorded answer",
  };
  let requestBody: unknown;
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [item] })),
    http.put(`*/v1/datasets/${datasetID}/items/${item.id}`, async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json({ ...item, output: undefined });
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Edit case-one" }));
  await user.clear(screen.getByLabelText("Recorded answer (optional)"));
  await user.click(screen.getByRole("button", { name: "Save item" }));

  expect(requestBody).toMatchObject({ input: { question: "Question" }, output: null });
});

test("deletes a case only after explaining that run evidence remains", async () => {
  const item = itemFixture("case-one");
  let deleted = false;
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [item] })),
    http.delete(`*/v1/datasets/${datasetID}/items/${item.id}`, () => {
      deleted = true;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Delete case-one" }));
  const dialog = screen.getByRole("dialog", { name: "Delete dataset case?" });
  expect(dialog).toHaveTextContent("Historical evaluation evidence remains");
  await user.click(within(dialog).getByRole("button", { name: "Delete item" }));

  expect(deleted).toBe(true);
  expect(screen.queryByText("case-one")).not.toBeInTheDocument();
});

test("keeps an unsaved item after an API failure", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () => HttpResponse.json(datasetFixture())),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [] })),
    http.post(`*/v1/datasets/${datasetID}/items`, async ({ request }) => {
      expect(await request.json()).toEqual({ items: [{ input: { question: "A question" } }] });
      return HttpResponse.json({ title: "Unable to save" }, { status: 500 });
    }),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Add item" }));
  await user.type(screen.getByLabelText("Question"), "A question");
  await user.click(screen.getByRole("button", { name: "Save item" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Unable to save");
  expect(screen.getByLabelText("Question")).toHaveValue("A question");
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.queryByLabelText("Question")).not.toBeInTheDocument();
});

test("rejects a dataset from a different application", async () => {
  server.use(
    applicationHandler(),
    http.get(`*/v1/datasets/${datasetID}`, () =>
      HttpResponse.json({ ...datasetFixture(), application_id: otherAppID }),
    ),
    http.get(`*/v1/datasets/${datasetID}/items`, () => HttpResponse.json({ items: [] })),
  );
  renderApp(`/apps/${appID}/datasets/${datasetID}`);

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Dataset does not belong to this application",
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

function applicationHandler() {
  return http.get("*/v1/applications", () =>
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
  );
}

function datasetFixture() {
  return {
    id: datasetID,
    application_id: appID,
    name: "Regression cases",
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T11:00:00Z",
  };
}

function itemFixture(externalID: string) {
  return {
    id: `${externalID}-id`,
    dataset_id: datasetID,
    external_id: externalID,
    input: { unsafe: "<script>dataset input</script>" },
    output: undefined,
    expected_output: undefined,
    context: [{ id: "doc-1", text: "Supporting context" }],
    metadata: { priority: 1 },
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}
