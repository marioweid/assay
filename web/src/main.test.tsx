import { screen } from "@testing-library/react";

test("renders the Assay heading", async () => {
  document.body.innerHTML = '<div id="root"></div>';

  await import("./main");

  expect(await screen.findByRole("heading", { name: "Assay" })).toBeInTheDocument();
});
