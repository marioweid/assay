import { render, screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";

import { MetricsPage } from "@/features/metrics/metrics-page";
import { configureClient } from "@/api/client";

const server = setupServer();
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function showMetrics() {
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

test("shows an empty state", async () => {
  server.use(http.get("*/v1/applications/app-1/metrics", () => HttpResponse.json({ items: [] })));
  showMetrics();
  expect(await screen.findByText("No scores in this period.")).toBeInTheDocument();
});

test("shows request failures", async () => {
  server.use(
    http.get("*/v1/applications/app-1/metrics", () => new HttpResponse(null, { status: 500 })),
  );
  showMetrics();
  expect(await screen.findByRole("alert")).toHaveTextContent("Unable to load metrics");
});
