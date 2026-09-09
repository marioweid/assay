import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";

const projectID = "019d11d2-cbd3-7a5e-ae83-9b791c932911";
const applicationID = "019d11d2-cbd3-7a5e-ae83-9b791c932912";
const application = {
  auto_score_scorers: ["groundedness"],
  config: {},
  created_at: "2026-09-01T10:00:00Z",
  id: applicationID,
  name: "Support bot",
  project_id: projectID,
  slug: "support-bot",
  target_endpoint: {
    has_secret: true,
    headers: { Authorization: "Bearer {{ .secret }}" },
    method: "POST",
    request_template: { question: "{{ .item.input.question }}" },
    response_mapping: { context: "$.sources[*].text", output: "$.answer" },
    timeout_ms: 30000,
    url: "http://target:8090/answer",
  },
  updated_at: "2026-09-01T10:00:00Z",
};
const scorer = {
  application_id: applicationID,
  enabled: true,
  persisted: true,
  prompt_template_id: "",
  scorer: "groundedness",
  threshold: 0.5,
};
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => localStorage.setItem("assay.admin-token.v1", "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderApp(): void {
  render(
    <MemoryRouter initialEntries={[`/apps/${applicationID}/settings`]}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
}

test("preserves an endpoint secret when saving endpoint settings", async () => {
  let requestBody: unknown;
  server.use(
    http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })),
    http.get(`*/v1/applications/${applicationID}/scorers`, () =>
      HttpResponse.json({ items: [scorer] }),
    ),
    http.patch(`*/v1/applications/${applicationID}/endpoint`, async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json(application);
    }),
  );
  renderApp();
  const user = userEvent.setup();

  const heading = await screen.findByRole("heading", { name: "Application settings" });
  const endpoint = within(heading.closest("section") ?? document.body);
  await user.click(endpoint.getByRole("button", { name: "Save endpoint" }));

  expect(requestBody).toEqual({
    endpoint: {
      headers: { Authorization: "Bearer {{ .secret }}" },
      method: "POST",
      request_template: { question: "{{ .item.input.question }}" },
      response_mapping: { context: "$.sources[*].text", output: "$.answer" },
      timeout_ms: 30000,
      url: "http://target:8090/answer",
    },
  });
});

test("saves the full scorer override when changing a threshold", async () => {
  let requestBody: unknown;
  server.use(
    http.get("*/v1/applications", () => HttpResponse.json({ items: [application] })),
    http.get(`*/v1/applications/${applicationID}/scorers`, () =>
      HttpResponse.json({ items: [scorer] }),
    ),
    http.put(`*/v1/applications/${applicationID}/scorers/groundedness`, async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json({ ...scorer, threshold: 1 });
    }),
  );
  renderApp();
  const user = userEvent.setup();

  const threshold = await screen.findByLabelText("Groundedness threshold");
  await user.clear(threshold);
  await user.type(threshold, "1");
  await user.click(screen.getByRole("button", { name: "Save groundedness scorer" }));

  expect(requestBody).toEqual({ enabled: true, prompt_template_id: "", threshold: 1 });
});
