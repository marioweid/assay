import { act, screen } from "@testing-library/react";
import { setupServer } from "msw/node";

import { tokenModeServerInfo } from "@/test/server-info";

const server = setupServer(tokenModeServerInfo);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterAll(() => server.close());

test("renders the Assay heading", async () => {
  document.body.innerHTML = '<div id="root"></div>';
  await act(async () => {
    await import("@/main");
  });
  expect(await screen.findByRole("heading", { name: "Assay" })).toBeInTheDocument();
});
