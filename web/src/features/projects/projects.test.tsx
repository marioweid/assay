import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const projectID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const project = {
  id: projectID,
  name: "Trust Platform",
  judge_config: { has_api_key: true },
  created_at: "2026-09-01T10:00:00Z",
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

function catalogHandler(): void {
  server.use(http.get("*/v1/applications", () => HttpResponse.json({ items: [] })));
}

test("creates a project from an empty list and opens its detail", async () => {
  let requestBody: unknown;
  catalogHandler();
  server.use(
    http.get("*/v1/projects", () => HttpResponse.json({ items: [] })),
    http.post("*/v1/projects", async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json(project, { status: 201 });
    }),
    http.get(`*/v1/projects/${projectID}`, () => HttpResponse.json(project)),
  );
  renderApp("/projects");
  const user = userEvent.setup();

  expect(await screen.findByRole("heading", { name: "Projects" })).toBeInTheDocument();
  const createButton = screen.getByRole("button", { name: "New project" });
  expect(createButton).toBeInTheDocument();
  await user.click(createButton);

  const dialog = screen.getByRole("dialog", { name: "New project" });
  const submit = within(dialog).getByRole("button", { name: "Create project" });
  expect(submit).toBeDisabled();
  await user.type(within(dialog).getByLabelText("Project name"), "Trust Platform");
  await user.click(submit);

  expect(await screen.findByRole("heading", { name: "Trust Platform" })).toBeInTheDocument();
  expect(requestBody).toEqual({ name: "Trust Platform" });
});

test("keeps form input when a duplicate project name is rejected", async () => {
  catalogHandler();
  server.use(
    http.get("*/v1/projects", () => HttpResponse.json({ items: [] })),
    http.post("*/v1/projects", () =>
      HttpResponse.json(
        { title: "Conflict", detail: "Project name already exists" },
        { status: 409 },
      ),
    ),
  );
  renderApp("/projects");
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "New project" }));
  const dialog = screen.getByRole("dialog", { name: "New project" });
  await user.type(within(dialog).getByLabelText("Project name"), "Duplicate");
  await user.click(within(dialog).getByRole("button", { name: "Create project" }));

  expect(await within(dialog).findByRole("alert")).toHaveTextContent("already exists");
  expect(within(dialog).getByLabelText("Project name")).toHaveValue("Duplicate");
});

test("lists projects with judge state and opens the detail page", async () => {
  catalogHandler();
  server.use(
    http.get("*/v1/projects", () => HttpResponse.json({ items: [project] })),
    http.get(`*/v1/projects/${projectID}`, () => HttpResponse.json(project)),
  );
  renderApp("/projects");
  const user = userEvent.setup();

  expect(await screen.findByRole("link", { name: "Trust Platform" })).toBeInTheDocument();
  expect(screen.getByText("Configured")).toBeInTheDocument();
  await user.click(screen.getByRole("link", { name: "Trust Platform" }));

  expect(await screen.findByRole("heading", { name: "Trust Platform" })).toBeInTheDocument();
  expect(screen.getByText("Applications")).toBeInTheDocument();
});

test("renames a project from its detail page", async () => {
  let current = project;
  catalogHandler();
  server.use(
    http.get(`*/v1/projects/${projectID}`, () => HttpResponse.json(current)),
    http.patch(`*/v1/projects/${projectID}`, async ({ request }) => {
      const body = (await request.json()) as { name: string };
      current = { ...current, name: body.name };
      return HttpResponse.json(current);
    }),
  );
  renderApp(`/projects/${projectID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Rename" }));
  const dialog = screen.getByRole("dialog", { name: "Rename project" });
  const input = within(dialog).getByLabelText("Project name");
  await user.clear(input);
  await user.type(input, "Trust Platform Prod");
  await user.click(within(dialog).getByRole("button", { name: "Save name" }));

  expect(await screen.findByRole("heading", { name: "Trust Platform Prod" })).toBeInTheDocument();
});

test("deletes a project only after typing its exact name", async () => {
  catalogHandler();
  server.use(
    http.get(`*/v1/projects/${projectID}`, () => HttpResponse.json(project)),
    http.delete(`*/v1/projects/${projectID}`, () => new HttpResponse(null, { status: 204 })),
    http.get("*/v1/projects", () => HttpResponse.json({ items: [] })),
  );
  renderApp(`/projects/${projectID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Delete project" }));
  const dialog = screen.getByRole("dialog", { name: "Delete project?" });
  const confirm = within(dialog).getByRole("button", { name: "Delete project" });
  expect(confirm).toBeDisabled();
  await user.type(within(dialog).getByLabelText(/to confirm/), "wrong");
  expect(confirm).toBeDisabled();
  await user.clear(within(dialog).getByLabelText(/to confirm/));
  await user.type(within(dialog).getByLabelText(/to confirm/), project.name);
  await user.click(confirm);

  expect(await screen.findByRole("heading", { name: "Projects" })).toBeInTheDocument();
});

test("keeps the project when deletion fails", async () => {
  catalogHandler();
  server.use(
    http.get(`*/v1/projects/${projectID}`, () => HttpResponse.json(project)),
    http.delete(`*/v1/projects/${projectID}`, () =>
      HttpResponse.json({ title: "Conflict", status: 409 }, { status: 409 }),
    ),
  );
  renderApp(`/projects/${projectID}`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Delete project" }));
  const dialog = screen.getByRole("dialog", { name: "Delete project?" });
  await user.type(within(dialog).getByLabelText(/to confirm/), project.name);
  await user.click(within(dialog).getByRole("button", { name: "Delete project" }));

  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Conflict");
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(screen.getByRole("heading", { name: project.name })).toBeInTheDocument();
});

test("shows an unavailable state and retries the project list", async () => {
  catalogHandler();
  let failed = true;
  server.use(
    http.get("*/v1/projects", () => {
      if (failed) {
        return new HttpResponse(null, { status: 500 });
      }
      return HttpResponse.json({ items: [project] });
    }),
  );
  renderApp("/projects");
  const user = userEvent.setup();

  expect(await screen.findByRole("heading", { name: "Projects unavailable" })).toBeInTheDocument();
  failed = false;
  await user.click(screen.getByRole("button", { name: "Try again" }));

  await waitFor(() =>
    expect(screen.getByRole("link", { name: "Trust Platform" })).toBeInTheDocument(),
  );
});
