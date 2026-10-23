import assert from "node:assert/strict"
import fs from "node:fs"
import ts from "typescript"
import axios from "axios"
import React from "react"
import { renderToStaticMarkup } from "react-dom/server"

// Execute the actual page save handler, including its catch path; API failures
// must remain strings that React can render rather than becoming a white page.
const apiSource = fs.readFileSync(new URL("../../lib/api.ts", import.meta.url), "utf8")
const apiTree = ts.createSourceFile("api.ts", apiSource, ts.ScriptTarget.Latest, true)
const messageFunction = apiTree.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === "getApiErrorMessage")
const messageJS = ts.transpileModule(messageFunction.getText(apiTree).replace("export ", ""), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
const getApiErrorMessage = Function("axios", `${messageJS}; return getApiErrorMessage`)(axios)
const pageSource = fs.readFileSync(new URL("../../app/templates/page.tsx", import.meta.url), "utf8")
const pageTree = ts.createSourceFile("page.tsx", pageSource, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
let handler
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(pageTree) === "saveExportLayout") handler = node.initializer
  ts.forEachChild(node, visit)
}
visit(pageTree)
assert.ok(handler)
const handlerJS = ts.transpileModule(`(${handler.getText(pageTree)})`, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
const draft = { id: 17, status: "draft", export_layout: { title: "原版式" } }
let calls = 0
let failure
let selected
let layout
let templates
let busy
const alerts = []
const successes = []
const templateApi = { update: async (id, data) => {
  calls++
  assert.equal(id, draft.id)
  if (failure) throw failure
  return { data: { ...draft, export_layout: data.export_layout } }
} }
function createSave(locked = false) {
  return Function("selectedTemplate", "exportLayoutLocked", "savingExportLayout", "templateApi", "exportLayout", "setSavingExportLayout", "setSelectedTemplate", "setExportLayout", "setTemplates", "toast", "Alert", "getApiErrorMessage", `return ${handlerJS}`)(
    draft, locked, false, templateApi, { title: "新标题" }, value => { busy = value }, value => { selected = value }, value => { layout = value }, callback => { templates = callback([draft]) }, { success: value => successes.push(value) }, async (_, message) => {
      assert.equal(typeof message, "string")
      renderToStaticMarkup(React.createElement("p", null, message))
      alerts.push(message)
    }, getApiErrorMessage,
  )
}
for (const payload of [
  { error: { code: "PUBLISHED_TEMPLATE_IMMUTABLE", message: "请复制为新版本" } },
  { error: "配置无效", message: "最终得分为必填" },
  { error: { code: "INTERNAL_ERROR", message: "保存失败，请重试" } },
]) {
  failure = new axios.AxiosError("Request failed", undefined, undefined, undefined, { data: payload, status: 409 })
  await createSave()()
  assert.equal(busy, false)
}
assert.deepEqual(alerts, ["请复制为新版本", "最终得分为必填", "保存失败，请重试"])
assert.equal(successes.length, 0, "failed saves must not report success")
failure = undefined
await createSave()()
assert.equal(selected.export_layout.title, "新标题")
assert.equal(layout.title, "新标题")
assert.equal(templates[0].export_layout.title, "新标题")
assert.equal(successes.length, 1)
const previousCalls = calls
await createSave(true)()
assert.equal(calls, previousCalls, "published templates must not issue a save")
console.log("PASS: structured errors render safely, draft save updates all state, locked saves are blocked")
