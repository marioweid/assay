import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { JsonView } from "@/components/json-view";

test("reports a denied copy operation without exposing a second payload", async () => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
  });
  render(<JsonView value={{ secret: "do-not-repeat" }} />);

  await userEvent.click(screen.getByRole("button", { name: "Copy JSON" }));
  const alert = await screen.findByRole("alert");
  expect(alert).toHaveTextContent("Copy failed");
  expect(alert).not.toHaveTextContent("do-not-repeat");
});
