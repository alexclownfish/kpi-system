import { expect, test as base } from "@playwright/test"
import { AcceptanceApiClient, type LoginResult } from "../helpers/api-client"

type AcceptanceFixtures = {
  acceptanceApi: AcceptanceApiClient
  hrAuth: LoginResult
}

export const test = base.extend<AcceptanceFixtures>({
  acceptanceApi: async ({ request }, use) => {
    const baseUrl = process.env.KPI_BASE_URL || "http://localhost"
    const client = new AcceptanceApiClient(request, baseUrl.replace(/\/$/, ""))
    await client.health()
    await use(client)
  },
  hrAuth: async ({ acceptanceApi }, use) => {
    const email = process.env.KPI_TEST_HR_EMAIL || "sunba@company.com"
    const password = process.env.KPI_TEST_HR_PASSWORD
    if (!password) throw new Error("Missing required environment variable KPI_TEST_HR_PASSWORD")
    const auth = await acceptanceApi.login(email, password)
    expect(auth.permissions, "HR fixture must have employee:create").toContain("employee:create")
    await use(auth)
  },
})

export { expect } from "@playwright/test"
