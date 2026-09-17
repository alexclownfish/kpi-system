import { randomBytes } from "node:crypto"

export type AcceptanceRunStatus = "running" | "passed" | "failed" | "cleanup_failed"
export type ScenarioStatus = "pending" | "passed" | "failed" | "skipped"
export type CleanupStatus = "pending" | "complete" | "failed"

export interface AcceptanceScenarioResult {
  id: string
  title: string
  actor: "hr_admin" | "department_manager" | "employee" | "operator"
  status: ScenarioStatus
  startedAt: string
  durationMs: number
  expected: string
  actual?: string
  evidence: string[]
}

export interface TestIdentity {
  purpose: "seed_hr" | "temporary_manager" | "temporary_employee"
  employeeId?: number
  email: string
  role: string
  dataScope?: string
  secretSource: "environment" | "generated"
}

export interface TemporaryRecord {
  resource: "employee"
  id: number
  label: string
  dependsOn: number[]
  cleanupOrder: number
  cleanupStatus: "registered" | "deleted" | "failed"
  cleanupError?: string
}

export interface AcceptanceRun {
  runId: string
  startedAt: string
  finishedAt?: string
  baseUrl: string
  status: AcceptanceRunStatus
  scenarios: AcceptanceScenarioResult[]
  temporaryRecords: TemporaryRecord[]
  cleanupStatus: CleanupStatus
}

export function createRunId(now = new Date()): string {
  const stamp = now.toISOString().replace(/[-:TZ.]/g, "").slice(0, 14)
  return `acceptance-${stamp}-${randomBytes(3).toString("hex")}`
}

export function createAcceptanceRun(baseUrl: string): AcceptanceRun {
  return {
    runId: createRunId(),
    startedAt: new Date().toISOString(),
    baseUrl,
    status: "running",
    scenarios: [],
    temporaryRecords: [],
    cleanupStatus: "pending",
  }
}
