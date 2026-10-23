import assert from "node:assert/strict"
import fs from "node:fs"
import ts from "typescript"

// Execute the page's real import handler to cover warning consent, publication
// guards and honest feedback when the API reuses existing templates.
const source = fs.readFileSync(new URL("../../app/templates/page.tsx", import.meta.url), "utf8")
const tree = ts.createSourceFile("templates.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
let handler
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(tree) === "importSourceWorkbook") handler = node.initializer
  ts.forEachChild(node, visit)
}
visit(tree)
assert.ok(handler)
const handlerJS = ts.transpileModule(`(${handler.getText(tree)})`, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText

function scenario(overrides = {}) {
  const state = {
    sourcePreview: { workbook_id: "workbook-4", source_sha256: "sha-original", can_publish: true, can_import_draft: true },
    sourceImporting: null, sourceImportResult: null, sourceLoadingDiagnostics: false, sourceLoadingHRDiagnostics: false,
    sourceBlockingCount: 0, sourceWarningCount: 22,
    ...overrides,
  }
  const calls = []
  const messages = []
  const alerts = []
  let confirmations = 0
  let cleared = false
  const dependencies = {
    ...state,
    setSourceImporting: value => { state.sourceImporting = value },
    Confirm: async () => { confirmations++; return overrides.accepted !== false },
    sourceWorkbookApi: { importWorkbook: async (id, request) => {
      calls.push({ id, request })
      if (overrides.failure) throw overrides.failure
      return { data: { workbook_id: id, source_sha256: request.source_sha256, created_templates: 3, reused_templates: 15 } }
    } },
    setSourceImportResult: value => { state.sourceImportResult = value },
    setSourcePreview: update => { state.sourcePreview = update(state.sourcePreview) },
    toast: { success: message => messages.push(message) },
    fetchTemplates: async () => {},
    setSourceDiagnostics: () => {},
    clearSourceResult: () => { cleared = true; state.sourcePreview = null },
    getApiErrorRequestId: () => "req-import-test",
    getApiErrorMessage: () => "导入事务已回滚",
    Alert: async (title, message) => alerts.push({ title, message }),
  }
  const run = Function(...Object.keys(dependencies), `return ${handlerJS}`)(...Object.values(dependencies))
  return { run, state, calls, messages, alerts, get confirmations() { return confirmations }, get cleared() { return cleared } }
}

const cancelled = scenario({ accepted: false })
await cancelled.run("publish")
assert.equal(cancelled.calls.length, 0, "declining warnings must not publish")
assert.equal(cancelled.state.sourceImporting, null)

const published = scenario()
await published.run("publish")
assert.equal(published.confirmations, 1)
assert.equal(published.calls[0].request.allow_warning_diagnostics, true)
assert.equal(published.calls[0].request.source_sha256, "sha-original")
assert.equal(published.state.sourcePreview.can_publish, false, "successful import must close stale actions")
assert.equal(published.state.sourcePreview.can_import_draft, false)
assert.match(published.messages[0], /新增 3 套，复用 15 套/)
assert.match(published.messages[0], /旧版本保留为历史/)
assert.doesNotMatch(published.messages[0], /全部.*发布|已导入为草稿/)
assert.equal(published.state.sourceImporting, null)

const draft = scenario()
await draft.run("draft")
assert.equal(draft.confirmations, 0)
assert.equal(draft.calls[0].request.mode, "draft")

for (const guard of [
  { sourceBlockingCount: 1 }, { sourcePreview: null },
  { sourceImporting: "draft" }, { sourceImportResult: { status: "imported" } },
  { sourceLoadingDiagnostics: true }, { sourceLoadingHRDiagnostics: true },
]) {
  const guarded = scenario(guard)
  await guarded.run("publish")
  assert.equal(guarded.calls.length, 0, `stale or blocked state must not import: ${JSON.stringify(guard)}`)
}

const failed = scenario({ failure: new Error("transaction failed") })
await failed.run("draft")
assert.equal(failed.cleared, true)
assert.equal(failed.messages.length, 0, "failed import must not report success")
assert.match(failed.alerts[0].message, /req-import-test/)
assert.equal(failed.state.sourceImporting, null)
console.log("PASS: workbook warning consent, import guards, created/reused feedback and failure recovery")
