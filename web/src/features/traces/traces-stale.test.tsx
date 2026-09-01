import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { vi } from "vitest";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const appID = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const otherAppID = "019d11d2-cbd3-7a5e-ae83-9b791c9329df";
const api = vi.hoisted(() => ({
  getTrace: vi.fn(),
  listApplications: vi.fn(),
  listTraces: vi.fn(),
}));

vi.mock("@/api/generated/sdk.gen", () => api);

test("ignores a stale response even when cancellation is not honored", async () => {
  localStorage.setItem("assay.admin-token.v1", "admin-secret");
  api.listApplications.mockResolvedValue({
    data: {
      items: [applicationFixture(appID, "Primary"), applicationFixture(otherAppID, "Other")],
    },
  });
  let resolveFirst: (response: TraceListResponse) => void = () => undefined;
  const firstResponse = new Promise<TraceListResponse>((resolve) => {
    resolveFirst = resolve;
  });
  api.listTraces.mockImplementation((options: TraceListOptions) =>
    options.query.application_id === appID
      ? firstResponse
      : Promise.resolve({ data: { items: [traceFixture("current operation")] } }),
  );
  render(
    <MemoryRouter initialEntries={[`/apps/${appID}/traces`]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
  const user = userEvent.setup();

  await user.selectOptions(
    await screen.findByRole("combobox", { name: "Application" }),
    otherAppID,
  );
  expect(await screen.findByText("current operation")).toBeInTheDocument();
  await act(() => resolveFirst({ data: { items: [traceFixture("stale operation")] } }));
  expect(screen.queryByText("stale operation")).not.toBeInTheDocument();
});

type TraceListOptions = { query: { application_id: string } };
type TraceListResponse = { data: { items: ReturnType<typeof traceFixture>[] } };

function applicationFixture(id: string, name: string) {
  return {
    id,
    project_id: id,
    name,
    slug: name.toLowerCase(),
    auto_score_scorers: [],
    config: {},
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  };
}

function traceFixture(rootName: string) {
  return {
    id: `${rootName}-id`,
    application_id: otherAppID,
    otel_trace_id: "01",
    root_name: rootName,
    start_time: "2026-09-01T10:00:00Z",
    end_time: "2026-09-01T10:00:01Z",
    status: "ok",
    span_count: 1,
    total_tokens: 1,
    attributes: {},
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:01Z",
  };
}
