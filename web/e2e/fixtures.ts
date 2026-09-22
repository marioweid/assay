import { randomBytes, randomUUID } from "node:crypto";

import { expect, test as base } from "@playwright/test";
import type { APIRequestContext, APIResponse, Page } from "@playwright/test";

export type Workspace = {
  projectId: string;
  applicationId: string;
  applicationSlug: string;
  apiKey: string;
};

export type CapturedTrace = {
  id: string;
  otelTraceId: string;
};

type AcceptanceFixtures = {
  workspace: Workspace;
  capturedTrace: CapturedTrace;
  browserGuards: void;
};

const adminToken = requiredEnvironment("ASSAY_ACCEPTANCE_ADMIN_TOKEN");
const adminHeaders = { Authorization: `Bearer ${adminToken}` };

export const test = base.extend<AcceptanceFixtures>({
  browserGuards: [
    async ({ page }, use) => {
      const errors: string[] = [];
      page.on("console", (message) => {
        if (message.type() === "error") errors.push(`console: ${message.text()}`);
      });
      page.on("pageerror", (error) => errors.push(`page: ${error.message}`));
      await page.addInitScript(() => {
        window.addEventListener("securitypolicyviolation", (event) => {
          console.error(`CSP violation: ${event.violatedDirective} ${event.blockedURI}`);
        });
        Date.now = () => 1_787_911_200_000;
      });
      await use();
      expect(errors, "unexpected browser errors or CSP violations").toEqual([]);
    },
    { auto: true },
  ],
  workspace: async ({ request }, use) => {
    const suffix = randomUUID();
    const project = await post<{ id: string }>(request, "/v1/projects", {
      name: `Acceptance ${suffix}`,
    });
    try {
      const key = await post<{ key: string }>(request, `/v1/projects/${project.id}/keys`, {
        name: "acceptance",
      });
      const applicationSlug = `acceptance-${suffix}`;
      const application = await post<{ id: string }>(request, "/v1/applications", {
        project_id: project.id,
        name: "Acceptance application",
        slug: applicationSlug,
      });
      await use({
        projectId: project.id,
        applicationId: application.id,
        applicationSlug,
        apiKey: key.key,
      });
    } finally {
      const response = await request.delete(`/v1/projects/${project.id}`, {
        headers: adminHeaders,
      });
      expect(response.status(), "delete acceptance project").toBe(204);
    }
  },
  capturedTrace: async ({ request, workspace }, use) => {
    const otelTraceId = randomBytes(16).toString("hex");
    const response = await request.post("/v1/traces", {
      headers: { "x-api-key": workspace.apiKey },
      data: tracePayload(workspace.applicationSlug, otelTraceId),
    });
    await expectResponse(response, "ingest synthetic trace", 200);
    const list = await get<{ items: Array<{ id: string; otel_trace_id: string }> }>(
      request,
      `/v1/traces?q=${otelTraceId}`,
      { "x-api-key": workspace.apiKey },
    );
    const trace = list.items.find((item) => item.otel_trace_id === otelTraceId);
    if (trace === undefined) throw new Error("ingested synthetic trace was not found");
    await use({ id: trace.id, otelTraceId });
  },
});

export { expect };

export async function authenticate(page: Page, theme: "light" | "dark" = "light"): Promise<void> {
  await page.addInitScript(
    ({ token, preference }) => {
      localStorage.setItem("assay.admin-token.v1", token);
      localStorage.setItem("assay.theme.v1", preference);
    },
    { token: adminToken, preference: theme },
  );
}

async function post<T>(request: APIRequestContext, path: string, data: object): Promise<T> {
  const response = await request.post(path, { data, headers: adminHeaders });
  await expectResponse(response, `POST ${path}`, 201);
  return (await response.json()) as T;
}

async function get<T>(
  request: APIRequestContext,
  path: string,
  headers: Record<string, string>,
): Promise<T> {
  const response = await request.get(path, { headers });
  await expectResponse(response, `GET ${path}`, 200);
  return (await response.json()) as T;
}

async function expectResponse(
  response: APIResponse,
  operation: string,
  status: number,
): Promise<void> {
  expect(response.status(), operation).toBe(status);
}

function requiredEnvironment(name: string): string {
  const value = process.env[name];
  if (value === undefined || value.trim() === "") throw new Error(`${name} is required`);
  return value;
}

function tracePayload(applicationSlug: string, traceId: string): object {
  const input = JSON.stringify([
    { role: "user", parts: [{ type: "text", content: "What is Assay?" }] },
  ]);
  const output = JSON.stringify([
    { role: "assistant", parts: [{ type: "text", content: "Assay evaluates AI systems." }] },
  ]);
  const context = JSON.stringify([
    { id: "k0", text: "Assay evaluates AI systems.", source: "synthetic" },
  ]);
  return {
    resourceSpans: [
      {
        resource: {
          attributes: [
            {
              key: "assay.application.slug",
              value: { stringValue: applicationSlug },
            },
          ],
        },
        scopeSpans: [
          {
            spans: [
              {
                traceId,
                spanId: randomBytes(8).toString("hex"),
                name: "acceptance answer",
                startTimeUnixNano: "1787911200000000000",
                endTimeUnixNano: "1787911201000000000",
                status: { code: 1 },
                attributes: [
                  { key: "assay.scorable", value: { boolValue: true } },
                  { key: "gen_ai.operation.name", value: { stringValue: "chat" } },
                  { key: "gen_ai.input.messages", value: { stringValue: input } },
                  { key: "gen_ai.output.messages", value: { stringValue: output } },
                  { key: "gen_ai.retrieval.documents", value: { stringValue: context } },
                ],
              },
            ],
          },
        ],
      },
    ],
  };
}
