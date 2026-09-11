import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const projectID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const applicationID = "019d11d2-cbd3-7a5e-ae83-9b791c932912";
const application = {
  auto_score_scorers: [],
  config: { region: "eu" },
  created_at: "2026-09-01T10:00:00Z",
  id: applicationID,
  name: "Support bot",
  project_id: projectID,
  slug: "support-bot",
  updated_at: "2026-09-01T10:00:00Z",
};
const project = {
  created_at: "2026-09-01T10:00:00Z",
  id: projectID,
  name: "Support",
  updated_at: "2026-09-01T10:00:00Z",
};
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderApp(path: string): void {
  render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
}

test("creates an application for a selected project", async () => {
  let requestBody: unknown;
  let applications: (typeof application)[] = [];
  server.use(
    http.get("*/v1/applications", () => HttpResponse.json({ items: applications })),
    http.get("*/v1/projects", () => HttpResponse.json({ items: [project] })),
    http.post("*/v1/applications", async ({ request }) => {
      requestBody = await request.json();
      applications = [application];
      return HttpResponse.json(application, { status: 201 });
    }),
  );
  renderApp("/apps");
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "New application" }));
  const dialog = screen.getByRole("dialog", { name: "New application" });
  await user.type(within(dialog).getByLabelText("Application name"), application.name);
  await user.type(within(dialog).getByLabelText("Slug"), application.slug);
  await user.selectOptions(within(dialog).getByLabelText("Project"), projectID);
  await user.click(within(dialog).getByRole("button", { name: "Create application" }));

  expect(requestBody).toEqual({
    config: {},
    name: application.name,
    project_id: projectID,
    slug: application.slug,
  });
  expect(await screen.findByRole("link", { name: application.name })).toBeInTheDocument();
});

test("deletes an application only after confirming its name", async () => {
  let deleted = false;
  server.use(
    http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })),
    http.delete(`*/v1/applications/${applicationID}`, () => {
      deleted = true;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  renderApp("/apps");
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: `Delete ${application.name}` }));
  const dialog = screen.getByRole("dialog", { name: `Delete ${application.name}?` });
  const confirm = within(dialog).getByRole("button", { name: "Delete application" });
  expect(confirm).toBeDisabled();
  await user.type(within(dialog).getByLabelText(/to confirm/), application.name);
  await user.click(confirm);

  expect(deleted).toBe(true);
});

test("keeps slug edits open after a conflict", async () => {
  server.use(
    http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })),
    http.patch(`*/v1/applications/${applicationID}`, () =>
      HttpResponse.json({ title: "Conflict", detail: "Slug already exists" }, { status: 409 }),
    ),
  );
  renderApp("/apps");
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: `Edit ${application.name}` }));
  const dialog = screen.getByRole("dialog", { name: "Edit application" });
  const slug = within(dialog).getByDisplayValue(application.slug);
  await user.clear(slug);
  await user.type(slug, "duplicate");
  await user.click(within(dialog).getByRole("button", { name: "Save application" }));

  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Slug already exists");
  expect(slug).toHaveValue("duplicate");
});

test("keeps advanced configuration when editing an application", async () => {
  let requestBody: unknown;
  server.use(
    http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })),
    http.get("*/v1/projects", () => HttpResponse.json({ items: [project] })),
    http.patch(`*/v1/applications/${applicationID}`, async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json({ ...application, name: "Support assistant" });
    }),
  );
  renderApp("/apps");
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: `Edit ${application.name}` }));
  const dialog = screen.getByRole("dialog", { name: "Edit application" });
  const name = within(dialog).getByLabelText("Application name");
  await user.clear(name);
  await user.type(name, "Support assistant");
  await user.click(within(dialog).getByRole("button", { name: "Save application" }));

  expect(requestBody).toEqual({
    config: { region: "eu" },
    name: "Support assistant",
    slug: application.slug,
  });
});
