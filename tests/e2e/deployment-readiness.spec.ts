import { expect, test } from "./fixtures/acceptance"
import type { APIRequestContext, APIResponse } from "@playwright/test"
import { AcceptanceApiClient } from "./helpers/api-client"

test.use({ trace: "off" })

test("PRE-01 health endpoint is ready", async ({ acceptanceApi }) => {
  await expect(acceptanceApi.health()).resolves.toBeUndefined()
})

test("PRE-02 login page is reachable through the gateway", async ({ page }) => {
  const response = await page.goto("/auth/login")
  expect(response?.status()).toBe(200)
  await expect(page.getByText("登录账户", { exact: true })).toBeVisible()
})

test("PRE-03 HR authentication exposes permission and data scope", async ({ hrAuth }) => {
  expect(hrAuth.token).toBeTruthy()
  expect(hrAuth.permissions).toContain("employee:create")
  expect(hrAuth.data_scope).toBe("ALL")
})

test("US3-AC2 transport and 5xx failures are never accepted as authorization denial", async ({ acceptanceApi, hrAuth }) => {
  const notFound = await acceptanceApi.authorized(hrAuth.token, "GET", "/api/path-that-does-not-exist")
  expect(notFound.response.status()).toBe(404)

  const serviceFailure = {
    fetch: async () => ({
      status: () => 502,
      text: async () => JSON.stringify({ error: "bad gateway" }),
    } as APIResponse),
  } as unknown as APIRequestContext
  await expect(new AcceptanceApiClient(serviceFailure, "http://invalid").authorized("token", "GET", "/api/employees"))
    .rejects.toThrow("Service error is not an authorization result")

  const networkFailure = {
    fetch: async () => { throw new Error("connection refused") },
  } as unknown as APIRequestContext
  await expect(new AcceptanceApiClient(networkFailure, "http://invalid").authorized("token", "GET", "/api/employees"))
    .rejects.toThrow("Network request failed")
})
