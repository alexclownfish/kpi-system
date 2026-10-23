import assert from "node:assert/strict"
import fs from "node:fs"

const component = fs.readFileSync(new URL("../../components/evaluation-scoring-task-list.tsx", import.meta.url), "utf8")
const page = fs.readFileSync(new URL("../../app/evaluations/page.tsx", import.meta.url), "utf8")
const api = fs.readFileSync(new URL("../../lib/api.ts", import.meta.url), "utf8")

// The scoring card is the page-level acceptance surface for T046. Keep these
// assertions close to the visible wording so a refactor cannot silently drop
// the completion gate or turn a valid zero into an empty value.
assert.match(component, /必填任务 \{data\.completion\.submitted\}\/\{data\.completion\.required\}，待完成 \{data\.completion\.blocking\}/)
assert.match(component, /aria-label="必填评分任务完成度"/)
assert.match(component, /当前还不能推进流程/)
assert.match(component, /尚未完成/)
assert.match(component, /notApplicable \? null : parsed/)
assert.match(component, /不适用（说明栏填写理由）/)
assert.match(component, /getApiErrorRequestId/)
assert.match(component, /请求 ID：\$\{requestId\}/)
assert.match(component, /重新读取/)
assert.match(component, /task\.score === null \|\| task\.score === undefined \? "" : String\(task\.score\)/)
assert.match(component, /计算版本 \{result\.calculation_version\}/)
assert.match(component, /贡献 \$\{item\.contribution\.toFixed\(2\)\}/)
assert.match(component, /第\{task\.sequence \|\| "-"\}顺位/)
assert.match(component, /请等待前序评分人完成后再提交/)
assert.match(api, /submitTask: \(id: number, data: \{ score\?: number \| null; comment: string; not_applicable: boolean; version: number \}\)/)
assert.match(api, /submitWorkSummary/)
assert.match(page, /getErrorMessage\(error, "确认最终得分失败，请重试。"\)/)
assert.match(page, /请先完成所有项目的自评/)
assert.match(page, /工作总结与后续计划/)
assert.match(page, /项目投入占比/)
console.log("PASS: scoring progress, blocking list, zero/N-A display and submit feedback UI contract")
