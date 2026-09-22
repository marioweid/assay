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
