import { defineConfig, devices } from "@playwright/test";

// Synthetic UI acceptance only: Vite runs without Assay, Docker, or persistent data.
export default defineConfig({
  testDir: "./e2e/sessions",
  outputDir: "./e2e-results/sessions",
  workers: 1,
  reporter: "line",
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:5271",
    trace: "off",
    screenshot: "off",
  },
  webServer: {
    command: "node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 5271 --strictPort",
    url: "http://127.0.0.1:5271",
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
