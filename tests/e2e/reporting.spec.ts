import { expect, test } from "@playwright/test"
import { buildScenarioResult } from "./helpers/acceptance-reporter"

test("US4-AC1 structured failure identifies scenario, stage, actual result and evidence", () => {
  const result = buildScenarioResult(
    { title: "US2-AC1 employee request is rejected" },
    {
      status: "failed",
      duration: 42,
      error: { message: "expected 403, received 500" },
      attachments: [{ name: "screenshot", contentType: "image/png", path: "test-results/failure.png" }],
    },
  )
  expect(result).toMatchObject({
    id: "US2-AC1",
    status: "failed",
    durationMs: 42,
    actual: "expected 403, received 500",
    evidence: ["test-results/failure.png"],
  })
})
