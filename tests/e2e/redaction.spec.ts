import { expect, test } from "@playwright/test"
import { mkdir, rm, writeFile } from "node:fs/promises"
import { spawnSync } from "node:child_process"
import path from "node:path"
import { redact, redactText } from "./helpers/redaction"

test("US4-AC2 removes authentication secrets from text and objects", () => {
  const jwt = ["eyJhbGciOiJIUzI1NiJ9", "eyJzdWIiOiIxMjM0NTY3ODkwIn0", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"].join(".")
  const hash = ["$2a$10$", "abcdefghijklmnopqrstuvwxyz", "ABCDEFGHIJKLMNOPQRSTUVWXYZ12345"].join("")
  const text = redactText(`Authorization=Bearer ${jwt} password=hunter2 hash=${hash}`)
  expect(text).not.toContain(jwt)
  expect(text).not.toContain("hunter2")
  expect(text).not.toContain(hash)

  const payload = redact({ password: "secret", headers: { Cookie: "sid=123", Authorization: `Bearer ${jwt}` } })
  expect(JSON.stringify(payload)).not.toContain("secret")
  expect(JSON.stringify(payload)).not.toContain("sid=123")
  expect(JSON.stringify(payload)).not.toContain(jwt)
})

test("US4-AC2 artifact scanner rejects a real report containing a secret", async () => {
  const fixtureDir = path.resolve(".artifacts/redaction-scan-fixture")
  const secret = "artifact-secret-must-not-survive"
  await mkdir(fixtureDir, { recursive: true })
  try {
    await writeFile(path.join(fixtureDir, "report.json"), JSON.stringify({ password: secret }), "utf8")
    const result = spawnSync(process.execPath, ["scripts/acceptance/scan-artifacts.mjs", fixtureDir], {
      env: { ...process.env, KPI_TEST_TEMP_PASSWORD: secret },
      encoding: "utf8",
    })
    expect(result.status).toBe(1)
    expect(result.stderr).toContain("SECRET LEAK")
  } finally {
    await rm(fixtureDir, { recursive: true, force: true })
  }
})
