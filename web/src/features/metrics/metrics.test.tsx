import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";

import { configureClient } from "@/api/client";
import { MetricsPage } from "@/features/metrics/metrics-page";

const server = setupServer();
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function showMetrics(): void {
  configureClient(() => "admin");
  render(
    <MemoryRouter initialEntries={["/apps/app-1/metrics"]}>
      <Routes>
        <Route path="/apps/:appId/metrics" element={<MetricsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

test("shows daily score averages and recorded pass rates", async () => {
  server.use(
    http.get("*/v1/applications/app-1/metrics", () =>
      HttpResponse.json({
        items: [
          {
            date: "2026-09-01T00:00:00Z",
            scorer: "groundedness",
            mean: 0.75,
            pass_rate: 0.5,
            n: 2,
          },
        ],
      }),
    ),
  );
  showMetrics();
  expect(await screen.findByText("75.0%")).toBeInTheDocument();
  expect(screen.getByText("50.0%")).toBeInTheDocument();
  expect(screen.getByRole("table")).toHaveAccessibleName("Daily score trends");
});

test("shows an empty state without zero-filled missing days", async () => {
  server.use(http.get("*/v1/applications/app-1/metrics", () => HttpResponse.json({ items: [] })));
  showMetrics();
  expect(
    await screen.findByRole("heading", { name: "No scores in this period" }),
  ).toBeInTheDocument();
});

test("shows request failures and recovers with the retry action", async () => {
  let failed = true;
  server.use(
    http.get("*/v1/applications/app-1/metrics", () => {
      if (failed) {
        return new HttpResponse(null, { status: 500 });
      }
      return HttpResponse.json({ items: [] });
    }),
  );
  showMetrics();
  const user = userEvent.setup();

  expect(await screen.findByRole("heading", { name: "Metrics unavailable" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Score trends" })).toBeInTheDocument();
  failed = false;
  await user.click(screen.getByRole("button", { name: "Try again" }));

  expect(
    await screen.findByRole("heading", { name: "No scores in this period" }),
  ).toBeInTheDocument();
});
