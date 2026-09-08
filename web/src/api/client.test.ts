import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";

import { configureClient } from "@/api/client";
import { Problem } from "@/api/errors";
import { client } from "@/api/generated/client.gen";
import { listApplications } from "@/api/generated/sdk.gen";

const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("includes validation messages but excludes rejected values", async () => {
  server.use(
    http.get("*/v1/applications", () =>
      HttpResponse.json(
        {
          title: "Unprocessable Entity",
          detail: "Invalid request",
          errors: [{ message: "1 items have no output", value: "secret-value" }],
        },
        { status: 422 },
      ),
    ),
  );
  configureClient(() => "token");
  const error = await listApplications().catch((reason: unknown) => reason);
  expect(error).toMatchObject({ detail: "Invalid request: 1 items have no output" });
  expect(JSON.stringify(error)).not.toContain("secret-value");
});

test("uses the current bearer token on same-origin requests", async () => {
  let token = "first-token";
  let authorization: string | null = null;
  let requestOrigin = "";
  server.use(
    http.get("*/v1/applications", ({ request }) => {
      authorization = request.headers.get("Authorization");
      requestOrigin = new URL(request.url).origin;
      return HttpResponse.json({ items: [] });
    }),
  );
  configureClient(() => token);

  token = "current-token";
  await listApplications();

  expect(authorization).toBe("Bearer current-token");
  expect(requestOrigin).toBe(window.location.origin);
  expect(client.getConfig().baseUrl).toBe(window.location.origin);
});

test("maps problem details without leaking token-bearing extensions", async () => {
  const token = "admin-secret-that-must-not-leak";
  server.use(
    http.get("*/v1/applications", () =>
      HttpResponse.json(
        {
          type: "https://assay.dev/problems/unauthorized",
          title: "Unauthorized",
          status: 401,
          detail: "Connect with a valid admin token.",
          request_headers: { authorization: `Bearer ${token}` },
        },
        { status: 401, headers: { "Content-Type": "application/problem+json" } },
      ),
    ),
  );
  configureClient(() => token);

  const error = await listApplications().catch((reason: unknown) => reason);

  expect(error).toBeInstanceOf(Problem);
  expect(error).toMatchObject({
    operation: "GET /v1/applications",
    status: 401,
    title: "Unauthorized",
    detail: "Connect with a valid admin token.",
  });
  expect(String(error)).not.toContain(token);
  expect(JSON.stringify(error)).not.toContain(token);
});
