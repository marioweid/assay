import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const server = setupServer(
  http.get("*/v1/server-info", () => HttpResponse.json({ local_mode: true })),
  http.get("*/v1/applications", ({ request }) => {
    expect(request.headers.has("Authorization")).toBe(false);
    return HttpResponse.json({ items: [] });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.clear());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderApp() {
  return render(
    <MemoryRouter initialEntries={["/apps"]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
}

test("local mode opens without a token and discards a stale stored credential", async () => {
  localStorage.setItem("assay.admin-token.v1", "stale-token");
  renderApp();
  expect(await screen.findByText("No applications yet")).toBeInTheDocument();
  expect(screen.getByText("Local mode")).toBeVisible();
  expect(screen.queryByLabelText("Admin token")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Disconnect" })).not.toBeInTheDocument();
  expect(localStorage.getItem("assay.admin-token.v1")).toBeNull();
});

test("token mode still requires a real admin credential", async () => {
  server.use(http.get("*/v1/server-info", () => HttpResponse.json({ local_mode: false })));
  renderApp();
  expect(await screen.findByLabelText("Admin token")).toBeVisible();
  expect(screen.queryByText("Local mode")).not.toBeInTheDocument();
});

test("discovery failure is actionable and cannot silently enable local mode", async () => {
  server.use(
    http.get("*/v1/server-info", () =>
      HttpResponse.json({ title: "Unavailable" }, { status: 503 }),
    ),
  );
  renderApp();
  expect(await screen.findByRole("alert")).toHaveTextContent("Unavailable");
  expect(screen.queryByText("No applications yet")).not.toBeInTheDocument();
  server.use(http.get("*/v1/server-info", () => HttpResponse.json({ local_mode: true })));
  await userEvent.setup().click(screen.getByRole("button", { name: "Retry connection" }));
  expect(await screen.findByText("No applications yet")).toBeInTheDocument();
});

test("a rejected admin token shows an actionable error and can be replaced", async () => {
  server.use(
    http.get("*/v1/server-info", () => HttpResponse.json({ local_mode: false })),
    http.get("*/v1/applications", ({ request }) =>
      request.headers.get("Authorization") === "Bearer valid-token"
        ? HttpResponse.json({ items: [] })
        : HttpResponse.json({ title: "Unauthorized" }, { status: 401 }),
    ),
  );
  renderApp();
  const user = userEvent.setup();
  await user.type(await screen.findByLabelText("Admin token"), "invalid-token");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("token was rejected");
  expect(localStorage.getItem("assay.admin-token.v1")).toBeNull();
  await user.clear(screen.getByLabelText("Admin token"));
  await user.type(screen.getByLabelText("Admin token"), "valid-token");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(await screen.findByText("No applications yet")).toBeVisible();
});

test("local management failure remains tokenless and supports retry", async () => {
  server.use(
    http.get("*/v1/applications", () =>
      HttpResponse.json({ title: "Storage unavailable" }, { status: 503 }),
    ),
  );
  renderApp();
  expect(await screen.findByRole("alert")).toHaveTextContent("Storage unavailable");
  expect(screen.queryByLabelText("Admin token")).not.toBeInTheDocument();
  server.resetHandlers();
  await userEvent.setup().click(screen.getByRole("button", { name: "Retry connection" }));
  expect(await screen.findByText("No applications yet")).toBeVisible();
});

test("malformed discovery data does not enable anonymous access", async () => {
  server.use(http.get("*/v1/server-info", () => HttpResponse.json({ local_mode: "true" })));
  renderApp();
  expect(await screen.findByRole("alert")).toHaveTextContent("authentication mode");
  expect(screen.queryByText("No applications yet")).not.toBeInTheDocument();
});
