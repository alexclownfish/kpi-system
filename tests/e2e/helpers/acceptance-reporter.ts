import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import path from "node:path"
import type { FullResult, Reporter, TestCase, TestResult } from "@playwright/test/reporter"
import { redact, safeError } from "./redaction"

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, character => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character] || character)
}

export interface ReporterScenarioResult {
  id: string
  title: string
  status: "passed" | "failed" | "skipped"
  durationMs: number
  expected: string
  actual?: string
  evidence: string[]
}

export function scenarioId(title: string): string {
  return title.match(/\b(?:PRE|US\d+)-[A-Z0-9]+(?:-[A-Z0-9]+)?\b/)?.[0] || "UNMAPPED"
}

export function buildScenarioResult(test: Pick<TestCase, "title">, result: Pick<TestResult, "status" | "duration" | "error" | "attachments">): ReporterScenarioResult {
  const status = result.status === "passed" ? "passed" : result.status === "skipped" ? "skipped" : "failed"
  const evidence = result.attachments
    .map(item => item.path)
    .filter((item): item is string => Boolean(item))
  return {
    id: scenarioId(test.title),
    title: test.title,
    status,
    durationMs: result.duration,
    expected: "Scenario assertions pass",
    ...(result.error ? { actual: safeError(result.error) } : {}),
    evidence,
  }
}

export default class AcceptanceReporter implements Reporter {
  private readonly startedAt = new Date().toISOString()
  private readonly scenarios: ReporterScenarioResult[] = []

  onTestEnd(test: TestCase, result: TestResult): void {
    const scenario = buildScenarioResult(test, result)
    this.scenarios.push(scenario)
    const label = scenario.status === "passed" ? "PASS" : scenario.status === "skipped" ? "SKIP" : "FAIL"
    const detail = scenario.actual ? `: ${scenario.actual}` : ""
    console.log(`[${label}] ${scenario.id} ${scenario.title} (${scenario.durationMs} ms)${detail}`)
  }

  onEnd(result: FullResult): void {
    const artifactDir = path.resolve(".artifacts")
    mkdirSync(artifactDir, { recursive: true })
    let cleanup: unknown = { status: "pending", recordsCreated: 0, recordsDeleted: 0 }
    try {
      const parsed = JSON.parse(readFileSync(path.join(artifactDir, "cleanup-status.json"), "utf8")) as { status: string; records?: Array<{ cleanupStatus: string }> }
      cleanup = {
        status: parsed.status,
        recordsCreated: parsed.records?.length || 0,
        recordsDeleted: parsed.records?.filter(item => item.cleanupStatus === "deleted").length || 0,
      }
    } catch {
      // Tests that create no temporary records legitimately have no cleanup file.
    }
    const payload = redact({
      runId: process.env.KPI_ACCEPTANCE_RUN_ID || `acceptance-${Date.now()}`,
      status: result.status === "passed" ? "passed" : "failed",
      startedAt: this.startedAt,
      finishedAt: new Date().toISOString(),
      scenarios: this.scenarios,
      cleanup,
    })
    writeFileSync(path.join(artifactDir, "acceptance-result.json"), JSON.stringify(payload, null, 2), "utf8")
    const rows = this.scenarios.map(scenario => `<tr><td>${escapeHtml(scenario.id)}</td><td>${escapeHtml(scenario.title)}</td><td>${scenario.status}</td><td>${scenario.durationMs} ms</td><td>${escapeHtml(scenario.actual || "")}</td></tr>`).join("")
    const html = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>KPI MVP Acceptance</title><style>body{font-family:system-ui;margin:2rem}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ddd;padding:.5rem;text-align:left}.passed{color:#087f23}.failed{color:#b42318}</style></head><body><h1>KPI MVP Acceptance</h1><p>Status: <strong class="${result.status}">${result.status}</strong></p><table><thead><tr><th>ID</th><th>Scenario</th><th>Status</th><th>Duration</th><th>Actual</th></tr></thead><tbody>${rows}</tbody></table></body></html>`
    writeFileSync(path.join(artifactDir, "acceptance-result.html"), html, "utf8")
  }
}
