import { defineConfig, devices } from "@playwright/test";
import { projectBase, testOrigin } from "./site.config.mjs";

const testBaseURL = `${testOrigin}${projectBase}`;

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  forbidOnly: true,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [["line"], ["html", { open: "never" }]] : "line",
  use: {
    baseURL: testBaseURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: {
    command: "node scripts/serve-dist.mjs dist",
    url: testBaseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
});
