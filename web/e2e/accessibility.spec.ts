import AxeBuilder from "@axe-core/playwright";

import { authenticate, expect, test } from "./fixtures";

for (const theme of ["light", "dark"] as const) {
  for (const width of [360, 1440] as const) {
    test(`has no axe violations at ${width}px in ${theme} theme`, async ({ page, workspace }) => {
      await page.setViewportSize({ width, height: 900 });
      await authenticate(page, theme);
      await page.goto(`/apps/${workspace.applicationId}/traces`);
      await expect(page.getByRole("heading", { name: "Traces" })).toBeVisible();
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);

      const result = await new AxeBuilder({ page }).analyze();

      expect(result.violations).toEqual([]);
    });
  }
}

test("populated project and score tables pass axe at mobile width", async ({
  page,
  request,
  workspace,
}) => {
  await page.setViewportSize({ width: 360, height: 900 });
  await authenticate(page);
  await page.goto(`/projects/${workspace.projectId}`);
  await expect(page.getByRole("heading", { name: "API keys" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Revoke" })).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);

  const headers = { Authorization: `Bearer ${process.env["ASSAY_ACCEPTANCE_ADMIN_TOKEN"]}` };
  const datasetResponse = await request.post("/v1/datasets", {
    headers,
    data: { application_id: workspace.applicationId, name: "Accessibility evidence" },
  });
  expect(datasetResponse.status()).toBe(201);
  const dataset = (await datasetResponse.json()) as { id: string };
  const itemsResponse = await request.post(`/v1/datasets/${dataset.id}/items`, {
    headers,
    data: {
      items: [
        {
          input: { question: "What is Assay?" },
          output: "Assay evaluates AI systems.",
          expected_output: "Assay evaluates AI systems.",
          context: [{ id: "k0", text: "Assay evaluates AI systems." }],
        },
      ],
    },
  });
  expect(itemsResponse.status()).toBe(201);
  const runResponse = await request.post("/v1/runs", {
    headers,
    data: {
      application_id: workspace.applicationId,
      dataset_id: dataset.id,
      name: "Accessible aggregates",
      mode: "score_existing",
      scorers: ["groundedness"],
    },
  });
  expect(runResponse.status()).toBe(202);
  const run = (await runResponse.json()) as { id: string };
  await expect
    .poll(async () => {
      const response = await request.get(`/v1/runs/${run.id}`, { headers });
      expect(response.status()).toBe(200);
      return ((await response.json()) as { status: string }).status;
    })
    .toBe("succeeded");

  for (const [route, label] of [
    [`/apps/${workspace.applicationId}/runs/${run.id}`, "Score aggregates table"],
    [`/apps/${workspace.applicationId}/metrics`, "Daily score trends table"],
  ] as const) {
    await page.goto(route);
    const scrollRegion = page.getByRole("region", { name: label });
    await expect(scrollRegion.getByRole("table")).toBeVisible();
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
    await scrollRegion.focus();
    await expect(scrollRegion).toBeFocused();
    await page.keyboard.press("ArrowRight");
    await expect
      .poll(() => scrollRegion.evaluate((element) => element.scrollLeft))
      .toBeGreaterThan(0);
  }
});

test("mobile navigation traps focus and restores it on Escape", async ({ page, workspace }) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await authenticate(page);
  await page.goto(`/apps/${workspace.applicationId}/traces`);
  const trigger = page.getByRole("button", { name: "Open navigation" });

  await trigger.focus();
  await page.keyboard.press("Enter");
  const drawer = page.getByRole("dialog", { name: "Application navigation" });
  await expect(drawer).toBeVisible();
  for (let press = 0; press < 8; press++) {
    await page.keyboard.press("Tab");
    await expect(drawer.locator(":focus")).toBeVisible();
  }
  await page.keyboard.press("Escape");
  await expect(drawer).toBeHidden();
  await expect(trigger).toBeFocused();
});
