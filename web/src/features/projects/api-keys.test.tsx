import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";

import { configureClient } from "@/api/client";
import type { ApiKeyResponse } from "@/api/generated/types.gen";
import { ApiKeysPanel } from "@/features/projects/api-keys-panel";

const projectID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const server = setupServer();

const activeKey: ApiKeyResponse = {
  id: "019d11d2-cbd3-7a5e-ae83-9b791c932922",
  project_id: projectID,
  name: "CI",
  key_prefix: "assay_ci_8f3a",
  last_used_at: "2026-09-01T08:00:00Z",
  created_at: "2026-09-01T07:00:00Z",
  updated_at: "2026-09-01T07:00:00Z",
};
const revokedKey: ApiKeyResponse = {
  ...activeKey,
  id: "019d11d2-cbd3-7a5e-ae83-9b791c932933",
  name: "Old CI",
  key_prefix: "assay_old_11b2",
  revoked_at: "2026-09-02T07:00:00Z",
  updated_at: "2026-09-02T07:00:00Z",
};

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  configureClient(() => "admin");
});
afterEach(() => {
  server.resetHandlers();
  localStorage.clear();
});
afterAll(() => server.close());

function renderPanel(): void {
  render(<ApiKeysPanel projectID={projectID} />);
}

test("lists key metadata without exposing the plaintext", async () => {
  server.use(
    http.get(`*/v1/projects/${projectID}/keys`, () =>
      HttpResponse.json({ items: [activeKey, revokedKey] }),
    ),
  );
  renderPanel();

  expect(await screen.findByText("CI")).toBeInTheDocument();
  expect(screen.getByText("Old CI")).toBeInTheDocument();
  expect(screen.getByText("assay_ci_8f3a")).toBeInTheDocument();
  expect(screen.getByText("Active")).toBeInTheDocument();
  expect(screen.getByText("Revoked")).toBeInTheDocument();
  expect(document.body).not.toHaveTextContent("assay_ci_8f3a-full");
});

test("revoked keys are not offered for reuse", async () => {
  server.use(
    http.get(`*/v1/projects/${projectID}/keys`, () =>
      HttpResponse.json({ items: [activeKey, revokedKey] }),
    ),
  );
  renderPanel();

  await screen.findByText("CI");
  const rows = screen.getAllByRole("row");
  const revokedRow = rows.find((row) => row.textContent?.includes("Old CI"));
  expect(revokedRow).toBeDefined();
  expect(within(revokedRow!).queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
  expect(
    within(rows.find((row) => row.textContent?.includes("CI"))!).getByRole("button", {
      name: "Revoke",
    }),
  ).toBeInTheDocument();
});

test("creates a key, shows it once, and clears it on dismiss", async () => {
  const secret = "assay_live_019d-secret-value";
  server.use(
    http.get(`*/v1/projects/${projectID}/keys`, () => HttpResponse.json({ items: [activeKey] })),
    http.post(`*/v1/projects/${projectID}/keys`, () =>
      HttpResponse.json(
        {
          ...activeKey,
          name: "Local emitters",
          key_prefix: "assay_live_019d",
          key: secret,
        },
        { status: 201 },
      ),
    ),
  );
  renderPanel();
  const user = userEvent.setup();
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });

  await user.click(await screen.findByRole("button", { name: "New key" }));
  const dialog = screen.getByRole("dialog", { name: "New API key" });
  await user.type(within(dialog).getByLabelText("Key name"), "Local emitters");
  await user.click(within(dialog).getByRole("button", { name: "Create key" }));

  const copyDialog = await screen.findByRole("dialog", { name: "Copy your key" });
  expect(within(copyDialog).getByText(secret)).toBeInTheDocument();
  expect(screen.getAllByText(secret)).toHaveLength(1);
  expect(storedValues(localStorage)).not.toContain(secret);
  expect(storedValues(sessionStorage)).not.toContain(secret);

  await user.click(within(copyDialog).getByRole("button", { name: "Copy key" }));
  expect(await screen.findByText("Key copied to clipboard.")).toBeInTheDocument();
  expect(writeText).toHaveBeenCalledWith(secret);

  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog", { name: "Copy your key" })).toBeInTheDocument();
  await user.click(screen.getByTestId("dialog-overlay"));
  expect(screen.getByRole("dialog", { name: "Copy your key" })).toBeInTheDocument();

  await user.click(within(copyDialog).getByRole("button", { name: "Done" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name: "Copy your key" })).not.toBeInTheDocument(),
  );
  expect(screen.queryByText(secret)).not.toBeInTheDocument();
});

test("announces a copy failure and keeps the key visible", async () => {
  const secret = "assay_live_fail-secret";
  server.use(
    http.get(`*/v1/projects/${projectID}/keys`, () => HttpResponse.json({ items: [] })),
    http.post(`*/v1/projects/${projectID}/keys`, () =>
      HttpResponse.json(
        { ...activeKey, key_prefix: "assay_live_fail", key: secret },
        { status: 201 },
      ),
    ),
  );
  renderPanel();
  const user = userEvent.setup();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
  });

  await user.click(await screen.findByRole("button", { name: "New key" }));
  await user.type(screen.getByLabelText("Key name"), "Local");
  await user.click(screen.getByRole("button", { name: "Create key" }));

  const dialog = await screen.findByRole("dialog", { name: "Copy your key" });
  await user.click(within(dialog).getByRole("button", { name: "Copy key" }));
  expect(await screen.findByText(/Copy failed/)).toBeInTheDocument();
  expect(within(dialog).getByText(secret)).toBeInTheDocument();
});

test("revokes a key after confirmation and refreshes the list", async () => {
  let list: ApiKeyResponse[] = [activeKey];
  server.use(
    http.get(`*/v1/projects/${projectID}/keys`, () => HttpResponse.json({ items: list })),
    http.delete(`*/v1/projects/${projectID}/keys/${activeKey.id}`, () => {
      list = [revokedKey];
      return new HttpResponse(null, { status: 204 });
    }),
  );
  renderPanel();
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Revoke" }));
  const dialog = screen.getByRole("dialog", { name: "Revoke CI?" });
  await user.click(within(dialog).getByRole("button", { name: "Revoke key" }));

  expect(await screen.findByText("Old CI")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
});

function storedValues(storage: Storage): string {
  let values = "";
  for (let index = 0; index < storage.length; index++) {
    const key = storage.key(index);
    if (key !== null) values += storage.getItem(key) ?? "";
  }
  return values;
}
