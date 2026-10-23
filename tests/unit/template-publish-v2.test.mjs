import assert from "node:assert/strict"
import fs from "node:fs"

const api = fs.readFileSync(new URL("../../lib/api.ts", import.meta.url), "utf8")
const page = fs.readFileSync(new URL("../../app/templates/page.tsx", import.meta.url), "utf8")

assert.match(api, /clone: .*template/)
assert.match(api, /publish: .*template/)
assert.match(api, /retire: .*template/)
assert.match(api, /source_workbook_sha256|source_sheet|source_issues/)
assert.match(page, /来源版本：|SHA-256：|来源诊断/)
assert.match(page, /复制新版本后编辑/)
assert.match(page, /已发布模板不可直接修改|历史考核必须保持原版式/)
assert.match(page, /发布中\.\.\.|导入并发布/)
assert.match(page, /请求标识：/)
console.log("PASS: template source provenance, version cloning, immutable history and publication feedback UI contract")
