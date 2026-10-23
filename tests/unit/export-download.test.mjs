import assert from "node:assert/strict"
import fs from "node:fs/promises"
import ts from "typescript"

const source = await fs.readFile(new URL("../../lib/export-download.ts", import.meta.url), "utf8")
const javascript = ts.transpileModule(
  `${source.replace(/^export /gm, "")}\nreturn { downloadExportFile, ExportDownloadError, getExportDownloadRequestId }`,
  { compilerOptions: { target: ts.ScriptTarget.ES2022 } },
).outputText
const { downloadExportFile, ExportDownloadError, getExportDownloadRequestId } = Function(javascript)()

const response = { file_url: "https://example.test/result.xlsx", request_id: "download-request-1" }
let opened = []
await downloadExportFile(response, {
  downloadUrl: async url => { opened.push({ type: "download", url }) },
  openWindow: (url, target) => { opened.push({ type: "window", url, target }); return { closed: false } },
})
assert.deepEqual(opened, [{ type: "download", url: response.file_url }], "successful bridge download must not open a second window")

opened = []
await downloadExportFile(response, {
  downloadUrl: async () => { throw new Error("bridge unavailable") },
  openWindow: (url, target) => { opened.push({ type: "window", url, target }); return { closed: false } },
})
assert.deepEqual(opened, [{ type: "window", url: response.file_url, target: "_blank" }], "a rejected bridge may recover through a real fallback window")

opened = []
const blockedError = await assert.rejects(
  () => downloadExportFile(response, {
    downloadUrl: async () => { throw new Error("bridge unavailable") },
    openWindow: (url, target) => { opened.push({ type: "window", url, target }); return null },
  }),
  error => error instanceof ExportDownloadError && error.requestId === response.request_id && error.message.includes("阻止了备用窗口"),
)
assert.equal(blockedError, undefined, "blocked fallback must reject instead of reporting a successful export")

const missingUrlError = await assert.rejects(
  () => downloadExportFile({ request_id: "missing-url-request" }, {
    downloadUrl: async () => undefined,
    openWindow: () => ({ closed: false }),
  }),
  error => error instanceof ExportDownloadError && error.requestId === "missing-url-request" && error.message.includes("地址缺失"),
)
assert.equal(missingUrlError, undefined)

let retryAttempt = 0
await assert.rejects(
  () => downloadExportFile(response, {
    downloadUrl: async () => { retryAttempt++; throw new Error("bridge unavailable") },
    openWindow: () => null,
  }),
  error => getExportDownloadRequestId(error) === response.request_id,
)
await downloadExportFile(response, {
  downloadUrl: async () => { retryAttempt++; return undefined },
  openWindow: () => null,
})
assert.equal(retryAttempt, 2, "a failed export can be retried and recover without stale success state")

console.log("PASS: export download success, fallback recovery, blocked fallback diagnostics, and retry regression")
