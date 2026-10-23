import assert from "node:assert/strict"
import fs from "node:fs/promises"
import ts from "typescript"

const source = await fs.readFile(new URL("../../lib/evaluation-signoff.ts", import.meta.url), "utf8")
const pageSource = await fs.readFile(new URL("../../app/evaluations/page.tsx", import.meta.url), "utf8")
const apiSource = await fs.readFile(new URL("../../lib/api.ts", import.meta.url), "utf8")
const javascript = ts.transpileModule(
  `${source.replace(/^export /gm, "")}\nreturn { canRegisterPaperSignoff }`,
  { compilerOptions: { target: ts.ScriptTarget.ES2022 } },
).outputText
const { canRegisterPaperSignoff } = Function(javascript)()

assert.equal(canRegisterPaperSignoff("awaiting_employee_confirmation", true, false), true)
assert.equal(canRegisterPaperSignoff("pending_confirm", true, false), true)
assert.equal(canRegisterPaperSignoff("completed", true, false), false)
assert.equal(canRegisterPaperSignoff("awaiting_employee_confirmation", false, false), false)
assert.equal(canRegisterPaperSignoff("awaiting_employee_confirmation", true, true), false)
assert.match(pageSource, /canRegisterPaperSignoff\(/, "evaluation details must use the shared paper sign-off state guard")
assert.match(pageSource, /selectedEvaluation\.status === "completed" && hasPermission\("assessment:approve"\)/, "completed evaluations must expose the supplemental sign-off action to approvers")
assert.match(pageSource, /handleSupplementUpload/, "completed sign-off upload must have a dedicated handler")
assert.match(pageSource, /不会改变考核结果或完成状态/, "supplement dialog must explain that completion status is immutable")
assert.match(pageSource, /handleDownloadSupplement/, "supplemental attachments must provide a protected download action")
assert.match(apiSource, /confirmation\/supplements/, "API client must expose supplemental sign-off endpoints")

console.log("PASS: paper sign-off state guard and completed supplemental sign-off UI contract")
