import assert from "node:assert/strict"
import fs from "node:fs"

const templatesPage = fs.readFileSync(new URL("../../app/templates/page.tsx", import.meta.url), "utf8")
const evaluationsPage = fs.readFileSync(new URL("../../app/evaluations/page.tsx", import.meta.url), "utf8")

assert.match(templatesPage, /showHistoricalTemplates/)
assert.match(templatesPage, /显示历史版本/)
assert.match(templatesPage, /filter\([^)]*status\s*!==\s*["']retired["']/)
assert.match(templatesPage, /切换当前使用版本|旧版本保留为历史/)
assert.match(evaluationsPage, /isNewWorkflowEvaluation\(selectedEvaluation\)/)
assert.match(evaluationsPage, /统一加权得分满分 100 分/)
assert.match(evaluationsPage, /旧字段不适用|统一评分任务/)

console.log("PASS: template history visibility, revision switch and canonical evaluation summary UI contract")
