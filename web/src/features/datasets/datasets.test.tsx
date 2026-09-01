import { render, screen } from "@testing-library/react";
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
  await user.click(screen.getByRole("link", { name: "Regression cases" }));
  expect(await screen.findByRole("heading", { name: "Regression cases" })).toBeInTheDocument();
  expect(screen.getByText("No dataset items yet.")).toBeInTheDocument();
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
