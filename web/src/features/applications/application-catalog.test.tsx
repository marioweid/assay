import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";

import { AuthProvider } from "@/auth/auth-context";
import {
  ApplicationCatalogProvider,
  useApplicationCatalog,
} from "@/features/applications/application-catalog";

const storageKey = "assay.admin-token.v1";
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem(storageKey, "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("refreshes applications without reconnecting", async () => {
  let applications = [application("A")];
  server.use(http.get("*/v1/applications", () => HttpResponse.json({ items: applications })));
  render(
    <AuthProvider>
      <ApplicationCatalogProvider>
        <CatalogHarness />
      </ApplicationCatalogProvider>
    </AuthProvider>,
  );
  const user = userEvent.setup();

  expect(await screen.findByText("A")).toBeInTheDocument();
  applications = [application("A"), application("B")];
  await user.click(screen.getByRole("button", { name: "Refresh applications" }));

  expect(await screen.findByText("B")).toBeInTheDocument();
});

function CatalogHarness() {
  const { applications, refresh } = useApplicationCatalog();
  return (
    <>
      <button onClick={() => void refresh()}>Refresh applications</button>
      {applications.map((application) => (
        <p key={application.id}>{application.name}</p>
      ))}
    </>
  );
}

function application(name: string) {
  return {
    id: `019d11d2-cbd3-7a5e-ae83-9b791c9329${name === "A" ? "de" : "df"}`,
    project_id: "019d11d2-cbd3-7a5e-ae83-9b791c9329ff",
    name,
    slug: name.toLowerCase(),
    auto_score_scorers: [],
    config: {},
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}
