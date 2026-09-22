import { defineConfig, devices } from "@playwright/test";

const endpoint = process.env["ASSAY_ACCEPTANCE_ENDPOINT"];
const project = process.env["ASSAY_ACCEPTANCE_PROJECT"];
if (endpoint === undefined || project?.startsWith("assay-acceptance-") !== true) {
  throw new Error(
    "Playwright requires ASSAY_ACCEPTANCE_ENDPOINT and an assay-acceptance-* project",
  );
}
const url = new URL(endpoint);
const chromiumExecutable = process.env["PLAYWRIGHT_CHROMIUM_EXECUTABLE"];
if (!(["127.0.0.1", "localhost"].includes(url.hostname) && url.port !== "8080")) {
  throw new Error("Playwright refuses a non-disposable or ordinary localhost:8080 endpoint");
}

export default defineConfig({
  testDir: "./e2e",
  outputDir: "./e2e-results",
  fullyParallel: false,
  workers: 1,
  retries: process.env["CI"] === undefined ? 0 : 1,
  reporter: "line",
  use: {
    ...devices["Desktop Chrome"],
    baseURL: endpoint,
    launchOptions: chromiumExecutable === undefined ? {} : { executablePath: chromiumExecutable },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
});
