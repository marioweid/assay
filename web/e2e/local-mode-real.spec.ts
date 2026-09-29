import { randomUUID } from "node:crypto";

import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const endpoint = process.env["ASSAY_ACCEPTANCE_ENDPOINT"];
const project = process.env["ASSAY_ACCEPTANCE_PROJECT"];
if (
  endpoint !== "http://127.0.0.1:18081" ||
  !project?.startsWith("assay-acceptance-local-") ||
  process.env["ASSAY_LOCAL_MODE"] !== "true"
) {
  throw new Error("Real local-mode browser check requires a disposable local acceptance stack");
}

test.use({ baseURL: endpoint });

test("embedded UI creates a project without storing an admin token", async ({ page, request }) => {
  const name = `browser-local-${randomUUID()}`;
  await page.addInitScript(() => localStorage.setItem("assay.admin-token.v1", "stale-token"));
  await page.goto("/");
  await expect(page.getByText("Local mode", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Admin token")).toHaveCount(0);
  expect(await page.evaluate(() => localStorage.getItem("assay.admin-token.v1"))).toBeNull();
  const created = await request.post("/v1/projects", { data: { name } });
  expect(created.status()).toBe(201);
  const body = (await created.json()) as { id: string };
  try {
    await page.goto("/projects");
    await expect(page.getByText(name)).toBeVisible();
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  } finally {
    const deleted = await request.delete(`/v1/projects/${body.id}`);
    expect(deleted.status()).toBe(204);
  }
});
