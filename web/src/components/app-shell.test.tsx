import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const primary = {
  id: "019d11d2-cbd3-7a5e-ae83-9b791c9329de",
  project_id: "019d11d2-cbd3-7a5e-ae83-9b791c9329df",
  name: "Support Bot",
  slug: "support-bot",
  auto_score_scorers: ["groundedness"],
  config: {},
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};
const secondary = {
  ...primary,
  id: "019d11d2-cbd3-7a5e-ae83-9b791c9329ee",
  name: "Review Bot",
  slug: "review-bot",
};
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => {
  server.resetHandlers();
  localStorage.clear();
  localStorage.removeItem("assay.theme.v1");
});
afterAll(() => server.close());

function useApplications(): void {
  server.use(
    http.get("*/v1/applications", () => HttpResponse.json({ items: [primary, secondary] })),
    http.get("*/v1/runs", () => HttpResponse.json({ items: [] })),
    http.get("*/v1/traces", () => HttpResponse.json({ items: [], next_cursor: "" })),
  );
}

function renderApp(path: string): void {
  render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
}

test("renders workspace and application navigation with the active section", async () => {
  useApplications();
  renderApp(`/apps/${primary.id}/runs`);

  const workspace = await screen.findByRole("navigation", { name: "Workspace" });
  expect(workspace).toHaveTextContent("Applications");
  expect(workspace).toHaveTextContent("Projects");

  const sectionNav = screen.getByRole("navigation", { name: "Application" });
  expect(screen.getByRole("link", { name: "Traces" })).toHaveAttribute(
    "href",
    `/apps/${primary.id}/traces`,
  );
  expect(screen.getByRole("link", { name: "Evaluations" })).toHaveAttribute(
    "href",
    `/apps/${primary.id}/runs`,
  );
  expect(screen.getByRole("link", { name: "Score trends" })).toHaveAttribute(
    "href",
    `/apps/${primary.id}/metrics`,
  );
  expect(screen.getByRole("link", { name: "Evaluations" })).toHaveAttribute("aria-current", "page");
  expect(sectionNav).toHaveTextContent("Datasets");
});

test("switches applications from the header switcher", async () => {
  useApplications();
  renderApp(`/apps/${primary.id}/traces`);
  const user = userEvent.setup();

  const switcher = await screen.findByRole("combobox", { name: "Application" });
  await user.selectOptions(switcher, secondary.id);

  await waitFor(() => expect(switcher).toHaveValue(secondary.id));
  expect(screen.getByRole("link", { name: "Review Bot" })).toBeInTheDocument();
});

test("offers a way back when the application is unknown", async () => {
  useApplications();
  renderApp("/apps/019d11d2-cbd3-7a5e-ae83-9b791c9329ff/traces");

  expect(await screen.findByText("Application not found")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Back to applications" })).toHaveAttribute(
    "href",
    "/apps",
  );
});

test("applies the selected theme to the document", async () => {
  useApplications();
  renderApp(`/apps/${primary.id}/traces`);
  const user = userEvent.setup();

  await user.selectOptions(await screen.findByRole("combobox", { name: "Theme" }), "dark");

  expect(document.documentElement["dataset"]["theme"]).toBe("dark");
  expect(localStorage.getItem("assay.theme.v1")).toBe("dark");
});

test("opens, traps, and closes the mobile navigation drawer", async () => {
  useApplications();
  renderApp(`/apps/${primary.id}/traces`);
  const user = userEvent.setup();

  const opener = await screen.findByRole("button", { name: "Open navigation" });
  await user.click(opener);

  const drawer = await screen.findByRole("dialog", { name: "Application navigation" });
  expect(drawer).toContainElement(screen.getAllByRole("link", { name: "Evaluations" }).at(-1)!);
  for (let press = 0; press < 6; press++) {
    await user.tab();
    expect(drawer.contains(document.activeElement)).toBe(true);
  }
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "Application navigation" })).not.toBeInTheDocument();
  await waitFor(() => expect(opener).toHaveFocus());
});

test("navigates globally to projects and back to applications", async () => {
  useApplications();
  renderApp(`/apps/${primary.id}/traces`);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("link", { name: "Projects" }));
  expect(await screen.findByRole("heading", { name: "Projects" })).toBeInTheDocument();

  await user.click(screen.getByRole("link", { name: "Applications" }));
  expect(await screen.findByRole("heading", { name: "Applications" })).toBeInTheDocument();
});
