import assert from "node:assert/strict"
import fs from "node:fs"

const api = fs.readFileSync(new URL("../../lib/api.ts", import.meta.url), "utf8")
const page = fs.readFileSync(new URL("../../app/evaluations/page.tsx", import.meta.url), "utf8")
assert.match(api, /previewMatch: \(employeeId: number, period: string\)/)
assert.match(api, /scorer_snapshot\?/)
assert.match(api, /signoff_snapshot\?/)
assert.match(page, /正在匹配适用模板和评分人/)
assert.match(page, /评分人：/)
assert.match(page, /blocking_errors/)
assert.match(page, /templatePreviewError/)
console.log("PASS: evaluation template match preview, scorer list and failure feedback UI contract")
