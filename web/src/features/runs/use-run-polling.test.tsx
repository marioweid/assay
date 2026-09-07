import { act, fireEvent, render, screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { vi } from "vitest";

import { configureClient } from "@/api/client";
import { useRunPolling } from "@/features/runs/use-run-polling";

const runID = "019d11d2-cbd3-7a5e-ae83-9b791c932922";
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  configureClient(() => "admin-secret");
  vi.useFakeTimers();
});
afterEach(() => {
  server.resetHandlers();
  vi.useRealTimers();
});
afterAll(() => server.close());

test("polls each second and stops after a terminal response", async () => {
  let requests = 0;
  server.use(
    http.get(`*/v1/runs/${runID}`, () => {
      requests++;
      return HttpResponse.json(runFixture(requests < 3 ? "running" : "succeeded"));
    }),
  );
  render(<PollingHarness />);

  await waitForText("running");
  expect(requests).toBe(1);
  await act(() => vi.advanceTimersByTimeAsync(1000));
  expect(requests).toBe(2);
  await act(() => vi.advanceTimersByTimeAsync(1000));
  expect(screen.getByText("succeeded")).toBeInTheDocument();
  await act(() => vi.advanceTimersByTimeAsync(3000));
  expect(requests).toBe(3);
});

test("pauses while hidden and resumes when visible", async () => {
  let requests = 0;
  server.use(
    http.get(`*/v1/runs/${runID}`, () => {
      requests++;
      return HttpResponse.json(runFixture("running"));
    }),
  );
  render(<PollingHarness />);
  await waitForText("running");
  setVisibility("hidden");
  await act(() => vi.advanceTimersByTimeAsync(3000));
  expect(requests).toBe(1);
  setVisibility("visible");
  await act(() => vi.advanceTimersByTimeAsync(1000));
  expect(requests).toBe(2);
});

test("stops after three failures and retries on demand", async () => {
  let requests = 0;
  server.use(
    http.get(`*/v1/runs/${runID}`, () => {
      requests++;
      if (requests <= 3) return HttpResponse.json({ title: "Unavailable" }, { status: 503 });
      return HttpResponse.json(runFixture("succeeded"));
    }),
  );
  render(<PollingHarness />);

  await act(() => vi.advanceTimersByTimeAsync(2000));
  await vi.waitFor(() =>
    expect(screen.getByRole("button", { name: "Retry polling" })).toBeInTheDocument(),
  );
  expect(requests).toBe(3);
  fireEvent.click(screen.getByRole("button", { name: "Retry polling" }));
  await waitForText("succeeded");
  expect(requests).toBe(4);
});

test("resets the failure count after a successful poll and aborts on unmount", async () => {
  let requests = 0;
  let aborted = false;
  server.use(
    http.get(`*/v1/runs/${runID}`, ({ request }) => {
      requests++;
      request.signal.addEventListener("abort", () => {
        aborted = true;
      });
      if (requests === 1 || requests === 3 || requests === 4) {
        return HttpResponse.json({ title: "Unavailable" }, { status: 503 });
      }
      return HttpResponse.json(runFixture("running"));
    }),
  );
  const view = render(<PollingHarness />);

  await act(() => vi.advanceTimersByTimeAsync(4000));
  expect(screen.queryByRole("button", { name: "Retry polling" })).not.toBeInTheDocument();
  view.unmount();
  expect(aborted).toBe(true);
});

function PollingHarness() {
  const polling = useRunPolling(runID);
  return (
    <div>
      <span>{polling.run?.status}</span>
      <span>{polling.error}</span>
      {polling.stopped && <button onClick={polling.retry}>Retry polling</button>}
    </div>
  );
}

async function waitForText(text: string): Promise<void> {
  await vi.waitFor(() => expect(screen.getByText(text)).toBeInTheDocument());
}

function setVisibility(state: DocumentVisibilityState): void {
  Object.defineProperty(document, "visibilityState", { configurable: true, value: state });
  document.dispatchEvent(new Event("visibilitychange"));
}

function runFixture(status: string) {
  return {
    id: runID,
    application_id: "app",
    dataset_id: "dataset",
    name: "Run",
    status,
    mode: "score_existing",
    params: {},
    scorers: [],
    aggregates: {},
    total_items: 1,
    succeeded_items: 0,
    failed_items: 0,
    canceled_items: 0,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}
