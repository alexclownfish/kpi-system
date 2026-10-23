import assert from "node:assert/strict"
import fs from "node:fs"

const api = fs.readFileSync(new URL("../../lib/api.ts", import.meta.url), "utf8")
const templatesPage = fs.readFileSync(new URL("../../app/templates/page.tsx", import.meta.url), "utf8")

assert.match(api, /getHRDiagnostics:\s*\(/)
assert.match(api, /performance-summary/)
assert.match(templatesPage, /HR 汇总诊断/)
assert.match(templatesPage, /HR诊断|hrDiagnostics|sourceHRDiagnostics/)
assert.match(templatesPage, /重试|刷新|retry/i)
assert.match(templatesPage, /blocking|阻断/)
console.log("PASS: HR formula diagnostics, publication blocking and retry UI contract")
