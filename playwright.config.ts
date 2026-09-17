import { defineConfig, devices } from "@playwright/test"
import { existsSync } from "node:fs"

const baseURL = process.env.KPI_BASE_URL || "http://localhost"
const macChrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
const executablePath = process.env.KPI_CHROME_PATH || (existsSync(macChrome) ? macChrome : undefined)

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  outputDir: "test-results",
  reporter: [
    ["./tests/e2e/helpers/acceptance-reporter.ts"],
  ],
  use: {
    baseURL,
    actionTimeout: 10_000,
    navigationTimeout: 15_000,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    launchOptions: executablePath ? { executablePath } : undefined,
    ...devices["Desktop Chrome"],
  },
})
