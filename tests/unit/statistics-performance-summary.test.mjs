import assert from "node:assert/strict"
import fs from "node:fs"

const api = fs.readFileSync(new URL("../../lib/api.ts", import.meta.url), "utf8")
const page = fs.readFileSync(new URL("../../app/statistics/page.tsx", import.meta.url), "utf8")
const handler = fs.readFileSync(new URL("../../server/handlers/statistics_v2.go", import.meta.url), "utf8")
const statistics = fs.readFileSync(new URL("../../server/handlers/statistics.go", import.meta.url), "utf8")

assert.match(api, /getPerformanceSummary:\s*\(params:/)
assert.match(api, /performance-summary/)
assert.match(api, /diagnostics\?: boolean/)
assert.match(api, /diagnostic_level\?:/)
assert.match(page, /statisticsApi\.getPerformanceSummary\(/)
assert.match(page, /source_snapshot_sha256/)
assert.match(page, /calculation_version/)
assert.match(page, /diagnostic_level/)
assert.match(page, /请求标识：/)
assert.match(page, /重试/)
assert.match(page, /allocation_threshold/)
assert.match(page, /period_type/)
assert.match(page, /evaluation_count/)
assert.match(page, /performanceSummary\.projects/)
assert.match(handler, /EvaluationCount/)
assert.match(handler, /projects/)
assert.match(handler, /json:"diagnostic_level"/)
assert.match(statistics, /EmployeeID\s+uint\s+`json:"employee_id"`/)
assert.match(statistics, /EmployeeID:\s+eval\.EmployeeID/)

console.log("PASS: statistics performance-summary source, diagnostics, version, retry, and employee scope contract")
