import { randomBytes, randomUUID } from "node:crypto";

import AxeBuilder from "@axe-core/playwright";

import { authenticate, expect, test, tracePayload } from "./fixtures";

test("embedded Sessions reads only tagged roots from a disposable database", async ({
  page,
  request,
  workspace,
  capturedTrace,
}) => {
  const sessionId = `conversation /?%# 🔒 ${randomUUID()}`;
  for (let index = 0; index < 2; index += 1) {
    const response = await request.post("/v1/traces", {
      headers: { "x-api-key": workspace.apiKey },
      data: tracePayload(workspace.applicationSlug, randomBytes(16).toString("hex"), sessionId),
    });
    expect(response.status(), "ingest tagged root turn").toBe(200);
  }

  await page.setViewportSize({ width: 360, height: 900 });
  await authenticate(page, "dark");
  await page.goto(`/apps/${workspace.applicationId}/sessions`);
  const sessions = page.getByRole("list", { name: "Sessions" });
  await expect(sessions.getByRole("listitem")).toHaveCount(1);
  await expect(sessions.getByText("2 turns")).toBeVisible();
  await sessions.getByRole("link", { name: "acceptance answer" }).click();
  expect(new URL(page.url()).searchParams.get("session_id")).toBe(sessionId);

  const transcript = page.getByRole("list", { name: "Session turns" });
  await expect(transcript.getByText("What is Assay?")).toHaveCount(2);
  await expect(page.getByText(sessionId, { exact: true }).first()).toBeVisible();
  await transcript
    .getByRole("button", { name: /Inspect source for assistant message/ })
    .first()
    .click();
  const inspector = page.getByRole("complementary", { name: "Selected trace inspector" });
  await expect(inspector.getByRole("region", { name: "Source scores" })).toContainText(
    "No scores recorded for this trace.",
  );
  await inspector.getByText("Model calls and captured context").click();
  await expect(
    inspector
      .getByRole("region", { name: "Conversation", exact: true })
      .getByText("Assay evaluates AI systems.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Timeline" }).click();
  await expect(page.getByRole("region", { name: "Session timeline" })).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
  const untagged = await request.get(`/v1/traces/${capturedTrace.id}`, {
    headers: { "x-api-key": workspace.apiKey },
  });
  expect(untagged.status(), "untagged trace remains accessible in Traces").toBe(200);
});
