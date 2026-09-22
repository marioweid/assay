import { authenticate, expect, test } from "./fixtures";

test("serves embedded UI with protected routes and CSP", async ({ page }) => {
  const response = await page.goto("/apps");

  expect(response?.headers()["content-security-policy"]).toContain("script-src 'self'");
  await expect(page.getByRole("button", { name: "Connect" })).toBeVisible();
  await expect(page).not.toHaveTitle(/Vite/);
});

test("seeds a captured trace through public APIs", async ({ capturedTrace, page, workspace }) => {
  await authenticate(page);
  await page.goto(`/apps/${workspace.applicationId}/traces/${capturedTrace.id}`);

  await expect(page).toHaveURL(new RegExp(`${capturedTrace.id}$`));
  await expect(page.getByRole("heading", { name: "acceptance answer", level: 1 })).toBeVisible();
  await expect(page.getByText("What is Assay?")).toBeVisible();
});

test("opens and closes an application dialog with the keyboard", async ({ page, workspace }) => {
  await authenticate(page);
  await page.goto("/apps");
  await expect(page.getByText(workspace.applicationSlug)).toBeVisible();
  const trigger = page.getByRole("button", { name: "New application" });

  await trigger.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "New application" });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(dialog.locator(":focus")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
});
