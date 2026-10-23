import assert from "node:assert/strict"
import fs from "node:fs"

const api = fs.readFileSync(new URL("../../lib/api.ts", import.meta.url), "utf8")
const page = fs.readFileSync(new URL("../../app/templates/page.tsx", import.meta.url), "utf8")

for (const method of ["validate", "getDiagnostics", "getHRDiagnostics", "importWorkbook"]) {
  assert.match(api, new RegExp(`export const sourceWorkbookApi[\\s\\S]*?\\b${method}:`), `source API missing ${method}`)
}
assert.match(api, /source-workbooks\/.+\/diagnostics/)
assert.match(api, /source-workbooks\/.+\/hr-diagnostics/)
assert.match(page, /导入新版工作簿/)
assert.match(page, /sourceBlockingCount/)
assert.match(page, /sourcePreview\.can_publish/)
assert.match(page, /setSourcePreview\(null\)/)
assert.match(page, /sourceValidating/)
assert.match(page, /sourceImporting === "publish"/)
assert.match(page, /clearSourceResult\(\)/)
assert.match(page, /导入失败/)
assert.match(page, /请求标识：/)

// A blocking source diagnostic must disable publication at the UI boundary,
// while draft import remains available for HR correction.
assert.match(page, /sourceBlockingCount > 0 \|\| !sourcePreview\.can_publish/)
assert.match(page, /sourcePreview\.can_import_draft/)
console.log("PASS: source workbook preview, diagnostic, retry and publication-gate UI contract")
