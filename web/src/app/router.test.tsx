import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, delay, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const storageKey = "assay.admin-token.v1";
const application = {
  id: "019d11d2-cbd3-7a5e-ae83-9b791c9329de",
  project_id: "019d11d2-cbd3-7a5e-ae83-9b791c9329df",
  name: "Support Bot",
  slug: "support-bot",
  auto_score_scorers: ["groundedness"],
  config: {},
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.clear());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("validates and persists a token while preserving a protected deep link", async () => {
  server.use(http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })));
  renderApp("/apps/019d11d2-cbd3-7a5e-ae83-9b791c9329de/traces");
  const user = userEvent.setup();

  await user.type(screen.getByLabelText("Admin token"), "admin-secret");
  await user.click(screen.getByRole("button", { name: "Connect" }));

  expect(await screen.findByRole("heading", { name: "Traces" })).toBeInTheDocument();
  expect(localStorage.getItem(storageKey)).toBe("admin-secret");
});

test("removes a rejected stored token", async () => {
  localStorage.setItem(storageKey, "expired-secret");
  server.use(
    http.get("*/v1/applications", () =>
      HttpResponse.json({ title: "Unauthorized", status: 401 }, { status: 401 }),
    ),
  );
  renderApp("/apps");

  expect(await screen.findByLabelText("Admin token")).toBeInTheDocument();
  expect(localStorage.getItem(storageKey)).toBeNull();
});

test("disconnect clears the stored token", async () => {
  localStorage.setItem(storageKey, "admin-secret");
  server.use(http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })));
  renderApp("/apps");
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Disconnect" }));

  expect(await screen.findByLabelText("Admin token")).toBeInTheDocument();
  expect(localStorage.getItem(storageKey)).toBeNull();
});

test("shows application loading and empty states", async () => {
  localStorage.setItem(storageKey, "admin-secret");
  server.use(
    http.get("*/v1/applications", async () => {
      await delay(40);
      return HttpResponse.json({ items: [] });
    }),
  );
  renderApp("/apps");

  expect(screen.getByRole("status")).toHaveTextContent("Connecting to Assay");
  expect(await screen.findByText("No applications yet", {}, { timeout: 3000 })).toBeInTheDocument();
  expect(screen.getByText(/CLI or API/)).toBeInTheDocument();
});

test("keeps request errors in the connection gate", async () => {
  server.use(
    http.get("*/v1/applications", () =>
      HttpResponse.json({ title: "Unavailable", status: 503 }, { status: 503 }),
    ),
  );
  renderApp("/apps");
  const user = userEvent.setup();

  await user.type(screen.getByLabelText("Admin token"), "admin-secret");
  await user.click(screen.getByRole("button", { name: "Connect" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("Unavailable");
  expect(screen.getByLabelText("Admin token")).toHaveValue("admin-secret");
});

test("manages focus and Escape in the mobile navigation drawer", async () => {
  localStorage.setItem(storageKey, "admin-secret");
  server.use(http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })));
  renderApp(`/apps/${application.id}/traces`);
  const user = userEvent.setup();

  const opener = await screen.findByRole("button", { name: "Open navigation" });
  await user.click(opener);

  const drawer = screen.getByRole("dialog", { name: "Application navigation" });
  await waitFor(() => expect(drawer).toContainElement(document.activeElement as HTMLElement));
  await user.tab({ shift: true });
  expect(screen.getByRole("button", { name: "Close navigation" })).toHaveFocus();
  await user.tab();
  expect(screen.getAllByRole("link", { name: "traces" }).at(-1)).toHaveFocus();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "Application navigation" })).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
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
