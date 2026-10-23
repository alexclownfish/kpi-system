import { request as requestFactory, type APIRequestContext } from "@playwright/test"
import { expect, test } from "./fixtures/acceptance"
import { AcceptanceApiClient, type LoginResult } from "./helpers/api-client"

/**
 * The full workflow suite is intentionally opt-in.  It creates two disposable
 * assessments, writes scores, and records sign-off evidence in the configured
 * service.  Keeping it behind an explicit environment flag prevents a normal
 * `playwright test` run from mutating a developer or production database.
 */
const realWorkflowEnabled = process.env.KPI_E2E_WORKFLOW === "1"
const requiredWorkflowEnv = [
  "KPI_TEST_HR_PASSWORD",
  "KPI_E2E_DEPARTMENT_ID",
  "KPI_E2E_MANAGER_ID",
]
const missingWorkflowEnv = requiredWorkflowEnv.filter(name => !process.env[name]?.trim())
const workflowReady = realWorkflowEnabled && missingWorkflowEnv.length === 0

const migrationEnabled = process.env.KPI_E2E_MIGRATION === "1"
const requiredMigrationEnv = [
  "KPI_TEST_ADMIN_EMAIL",
  "KPI_TEST_ADMIN_PASSWORD",
  "KPI_E2E_MIGRATION_EVALUATION_ID",
]
const missingMigrationEnv = requiredMigrationEnv.filter(name => !process.env[name]?.trim())
const migrationReady = migrationEnabled && missingMigrationEnv.length === 0

type EmployeeRecord = {
  id: number
  name: string
  email: string
  role: string
  department_id: number
  manager_id?: number
}

type EvaluationRecord = {
  id: number
  employee_id: number
  template_id?: number
  status: string
  period: string
  year: number
  month?: number
  quarter?: number
}

type TemplateRecord = {
  id: number
  name: string
  period: string
  status: string
  position_type: string
  items?: Array<{ scorer_types?: string[]; scorerTypes?: string[] }>
}

type ScoringTask = {
  id: number
  item_id: number
  scorer_user_id: number
  required: boolean
  status: string
  version: number
  max_score: number
}

type EvaluationTaskList = {
  workflow_state: string
  completion: { required: number; submitted: number; blocking: number }
  tasks: ScoringTask[]
}

type CanonicalResult = {
  evaluation_id: number
  result_version: number
  total_score: number
  snapshot_checksum: string
}

type SignoffSupplement = {
  id: number
  evaluation_id: number
  attachment_name?: string
  signed_at?: string
  remark: string
}

type ExportResponse = {
  file_url: string
  file_name: string
  file_size: number
  result_version?: number
  checksum?: string
  results?: Array<{ evaluation_id: number; result_version: number; checksum: string }>
}

type ResponsibilityRecord = {
  id: number
  code: string
  name: string
  scope_type: "company" | "department" | "position"
  department_id?: number | null
  position_type?: string
  project_id?: number | null
  user_id: number
  active: boolean
  effective_from?: string | null
  effective_to?: string | null
}

function bodyData<T>(body: unknown): T {
  if (body && typeof body === "object" && "data" in body) {
    return (body as { data: T }).data
  }
  return body as T
}

function requiredNumber(name: string): number {
  const value = Number(process.env[name])
  if (!Number.isInteger(value) || value <= 0) throw new Error(`${name} must be a positive integer`)
  return value
}

function periodInput(offset: number): Record<string, unknown> {
  const period = process.env.KPI_E2E_PERIOD?.trim() || "monthly"
  const year = Number(process.env.KPI_E2E_YEAR || 2099)
  if (period === "yearly") return { period, year }
  if (period === "quarterly") {
    const quarter = Number(process.env.KPI_E2E_QUARTER || 1)
    return { period, year, quarter: ((quarter - 1 + offset) % 4) + 1 }
  }
  const month = Number(process.env.KPI_E2E_MONTH || ((new Date().getUTCMonth() + 1)))
  return { period: "monthly", year, month: ((month - 1 + offset) % 12) + 1 }
}

test.describe("统一考核流程基础验收", () => {
  test("审计预演保持只读并返回可操作结果", async ({ acceptanceApi, hrAuth }) => {
    const result = await acceptanceApi.authorized(hrAuth.token, "POST", "/api/assessment-audits/preview", {
      include_completed: true,
    })
    expect(result.response.status()).toBe(200)
    const body = result.body as { data?: { run_id?: string; summary?: { evaluations_scanned?: number } } }
    expect(body.data?.run_id).toBeTruthy()
    expect(body.data?.summary?.evaluations_scanned).toBeGreaterThanOrEqual(0)
  })

})

test.describe("真实服务迁移预演（独立管理员前置）", () => {
  test.skip(
    !migrationReady,
    migrationEnabled
      ? `缺少迁移预演前置变量：${missingMigrationEnv.join(", ")}`
      : "未启用迁移预演：设置 KPI_E2E_MIGRATION=1 后才会读取指定考核",
  )

  let requestContext: APIRequestContext
  let api: AcceptanceApiClient
  let admin: LoginResult

  test.beforeAll(async () => {
    requestContext = await requestFactory.newContext()
    api = new AcceptanceApiClient(requestContext, (process.env.KPI_BASE_URL || "http://localhost").replace(/\/$/, ""))
    await api.health()
    admin = await api.login(process.env.KPI_TEST_ADMIN_EMAIL as string, process.env.KPI_TEST_ADMIN_PASSWORD as string)
    expect(admin.permissions).toContain("system:edit")
  })

  test.afterAll(async () => {
    await requestContext?.dispose()
  })

  test("独立管理员 dry-run 只创建迁移批次，不改变指定考核", async () => {
    const evaluationID = requiredNumber("KPI_E2E_MIGRATION_EVALUATION_ID")
    const before = await api.authorized(admin.token, "GET", `/api/evaluations/${evaluationID}`)
    expect(before.response.status(), JSON.stringify(before.body)).toBe(200)
    const beforeData = bodyData<Record<string, unknown>>(before.body)
    const tasksBefore = await api.authorized(admin.token, "GET", `/api/evaluations/${evaluationID}/scoring-tasks`)
    const immutableBefore = {
      id: beforeData.id,
      status: beforeData.status,
      total_score: beforeData.total_score,
      template_snapshot_json: beforeData.template_snapshot_json,
      updated_at: beforeData.updated_at,
      task_status: tasksBefore.response.status(),
      task_body: tasksBefore.body,
    }

    const dryRun = await api.authorized(admin.token, "POST", "/api/assessment-migrations/dry-run", {
      evaluation_ids: [evaluationID],
      target_version: "weighted-v2",
    })
    expect(dryRun.response.status(), JSON.stringify(dryRun.body)).toBe(200)
    const plan = bodyData<{ batch_id: string; mode: string; eligible_evaluation_ids: number[]; planned_task_count: number }>(dryRun.body)
    expect(plan.batch_id).toBeTruthy()
    expect(plan.mode).toBe("dry_run")
    expect(plan.planned_task_count).toBeGreaterThanOrEqual(0)

    const batch = await api.authorized(admin.token, "GET", `/api/assessment-migrations/${plan.batch_id}`)
    expect(batch.response.status(), JSON.stringify(batch.body)).toBe(200)
    expect(bodyData<{ batch_id: string; status: string }>(batch.body)).toMatchObject({ batch_id: plan.batch_id, status: "completed" })

    const after = await api.authorized(admin.token, "GET", `/api/evaluations/${evaluationID}`)
    expect(after.response.status()).toBe(200)
    const afterData = bodyData<Record<string, unknown>>(after.body)
    const tasksAfter = await api.authorized(admin.token, "GET", `/api/evaluations/${evaluationID}/scoring-tasks`)
    expect({
      id: afterData.id,
      status: afterData.status,
      total_score: afterData.total_score,
      template_snapshot_json: afterData.template_snapshot_json,
      updated_at: afterData.updated_at,
      task_status: tasksAfter.response.status(),
      task_body: tasksAfter.body,
    }).toEqual(immutableBefore)
  })
})

test.describe.serial("真实服务完整评分与签字闭环（显式启用）", () => {
  test.skip(
    !workflowReady,
    realWorkflowEnabled
      ? `缺少真实闭环前置变量：${missingWorkflowEnv.join(", ")}`
      : "未启用真实闭环：设置 KPI_E2E_WORKFLOW=1 后才会创建临时数据",
  )

  let requestContext: APIRequestContext
  let api: AcceptanceApiClient
  let hr: LoginResult
  let scorerAuth: LoginResult
  let scorer: EmployeeRecord
  let employee: EmployeeRecord
  let employeeAuth: LoginResult
  let templateID: number | undefined
  const evaluationIds: number[] = []
  const responsibilityIds: number[] = []

  async function createEvaluation(offset: number): Promise<EvaluationRecord> {
    const payload: Record<string, unknown> = {
      employee_id: employee.id,
      ...periodInput(offset),
    }
    if (templateID) payload.template_id = templateID
    const created = await api.authorized(hr.token, "POST", "/api/evaluations", payload)
    expect(created.response.status(), JSON.stringify(created.body)).toBe(201)
    const evaluation = bodyData<EvaluationRecord>(created.body)
    expect(evaluation.id).toBeGreaterThan(0)
    evaluationIds.push(evaluation.id)
    return evaluation
  }

  async function getTasks(evaluationID: number, token = hr.token): Promise<EvaluationTaskList> {
    const response = await api.authorized(token, "GET", `/api/evaluations/${evaluationID}/scoring-tasks`)
    expect(response.response.status(), JSON.stringify(response.body)).toBe(200)
    const data = bodyData<EvaluationTaskList>(response.body)
    expect(data.tasks.length).toBeGreaterThan(0)
    return data
  }

  async function advance(evaluationID: number, expectedState: string, token = hr.token): Promise<string> {
    const response = await api.authorized(token, "POST", `/api/evaluations/${evaluationID}/workflow/advance`, { expected_state: expectedState })
    expect(response.response.status(), JSON.stringify(response.body)).toBe(200)
    const data = bodyData<{ current_state: string }>(response.body)
    expect(data.current_state).toBeTruthy()
    return data.current_state
  }

  async function completeScoring(evaluationID: number): Promise<EvaluationTaskList> {
    // The self score is still represented by the compatibility score rows. A
    // valid zero is deliberate: it proves zero is treated as completed input.
    const scoresResponse = await api.authorized(hr.token, "GET", `/api/scores/evaluation/${evaluationID}`)
    expect(scoresResponse.response.status(), JSON.stringify(scoresResponse.body)).toBe(200)
    const scores = bodyData<Array<{ id: number; item?: { required?: boolean } }>>(scoresResponse.body)
    expect(scores.length).toBeGreaterThan(0)

    // The employee is the only identity allowed to write self scores.
    for (const score of scores) {
      const saved = await api.authorized(employeeAuth.token, "PUT", `/api/scores/${score.id}/self`, {
        self_score: 0,
        self_comment: "真实闭环零分回归",
      })
      expect(saved.response.status(), JSON.stringify(saved.body)).toBe(200)
    }
    await advance(evaluationID, "self_evaluating", employeeAuth.token)

    const tasks = await getTasks(evaluationID)
    const scorerTasks = tasks.tasks.filter(task => task.scorer_user_id === scorerAuth.user.id)
    expect(scorerTasks.length, "configured scorer must receive at least one item task").toBeGreaterThan(0)
    const scorerVisible = await getTasks(evaluationID, scorerAuth.token)
    expect(scorerVisible.tasks.length).toBeGreaterThan(0)
    expect(scorerVisible.tasks.every(task => task.scorer_user_id === scorerAuth.user.id)).toBeTruthy()
    const unassigned = tasks.tasks.find(task => task.scorer_user_id !== employee.id)
    if (unassigned) {
      // Owner visibility does not grant write access to another scorer's task.
      const denied = await api.authorized(employeeAuth.token, "POST", `/api/scoring-tasks/${unassigned.id}/submit`, {
        score: 0,
        comment: "越权回归",
        not_applicable: false,
        version: unassigned.version,
      })
      expect(denied.response.status(), JSON.stringify(denied.body)).toBe(403)
    }

    // HR approval permission is used as the explicit test override so the
    // suite does not need to know how many business scorer accounts exist.
    for (const task of scorerTasks.filter(item => item.status !== "submitted" && item.status !== "not_applicable")) {
      const submitted = await api.authorized(scorerAuth.token, "POST", `/api/scoring-tasks/${task.id}/submit`, {
        score: 0,
        comment: "真实闭环授权评分",
        not_applicable: false,
        version: task.version,
      })
      expect(submitted.response.status(), JSON.stringify(submitted.body)).toBe(200)
    }
    for (const task of tasks.tasks.filter(item => item.scorer_user_id !== scorerAuth.user.id && item.status !== "submitted" && item.status !== "not_applicable")) {
      const submitted = await api.authorized(hr.token, "POST", `/api/scoring-tasks/${task.id}/submit`, {
        score: 0,
        comment: "真实闭环管理员补充评分",
        not_applicable: false,
        version: task.version,
      })
      expect(submitted.response.status(), JSON.stringify(submitted.body)).toBe(200)
    }
    const finalState = await advance(evaluationID, "scoring")
    expect(finalState).toBe("awaiting_employee_confirmation")
    return tasks
  }

  async function exportPair(evaluationID: number): Promise<{ xlsx: ExportResponse; pdf: ExportResponse }> {
    // Keep the two requests ordered so both formats resolve the same immutable
    // snapshot rather than racing snapshot creation in a busy service.
    const xlsxResponse = await api.authorized(hr.token, "GET", `/api/export/evaluation/${evaluationID}?format=xlsx`)
    const pdfResponse = await api.authorized(hr.token, "GET", `/api/export/evaluation/${evaluationID}?format=pdf`)
    expect(xlsxResponse.response.status(), JSON.stringify(xlsxResponse.body)).toBe(200)
    expect(pdfResponse.response.status(), JSON.stringify(pdfResponse.body)).toBe(200)
    const xlsx = bodyData<ExportResponse>(xlsxResponse.body)
    const pdf = bodyData<ExportResponse>(pdfResponse.body)
    expect(xlsx.file_url).toBeTruthy()
    expect(pdf.file_url).toBeTruthy()
    expect(xlsx.result_version).toBe(pdf.result_version)
    expect(xlsx.checksum).toBe(pdf.checksum)

    for (const file of [xlsx, pdf]) {
      const download = await requestContext.get(file.file_url, { headers: { Authorization: `Bearer ${hr.token}` } })
      expect(download.status(), file.file_name).toBe(200)
      const bytes = await download.body()
      expect(bytes.length, file.file_name).toBeGreaterThan(100)
      if (file.file_name.endsWith(".pdf")) expect(bytes.subarray(0, 4).toString()).toBe("%PDF")
    }
    return { xlsx, pdf }
  }

  test.beforeAll(async () => {
    requestContext = await requestFactory.newContext()
    api = new AcceptanceApiClient(requestContext, (process.env.KPI_BASE_URL || "http://localhost").replace(/\/$/, ""))
    await api.health()
    hr = await api.login(process.env.KPI_TEST_HR_EMAIL || "sunba@company.com", process.env.KPI_TEST_HR_PASSWORD as string)
    expect(hr.permissions).toContain("assessment:create")
    const departmentID = requiredNumber("KPI_E2E_DEPARTMENT_ID")
    const managerID = requiredNumber("KPI_E2E_MANAGER_ID")
    const period = String(periodInput(0).period)
    const configuredTemplateID = Number(process.env.KPI_E2E_TEMPLATE_ID || 0)
    const templatesResponse = await api.authorized(hr.token, "GET", `/api/templates?department_id=${departmentID}&period=${encodeURIComponent(period)}&status=published`)
    expect(templatesResponse.response.status(), JSON.stringify(templatesResponse.body)).toBe(200)
    const templates = bodyData<TemplateRecord[]>(templatesResponse.body)
    const selectedTemplate = configuredTemplateID > 0
      ? templates.find(template => template.id === configuredTemplateID)
      : templates.find(template => template.position_type === process.env.KPI_E2E_POSITION) || templates[0]
    expect(selectedTemplate, "至少需要一个适用部门和周期的已发布模板").toBeTruthy()
    templateID = selectedTemplate?.id
    expect(templateID).toBeGreaterThan(0)
    const position = process.env.KPI_E2E_POSITION?.trim() || selectedTemplate?.position_type
    expect(position, "已发布模板必须提供岗位类型").toBeTruthy()
    const scorerTypes = [...new Set((selectedTemplate?.items || []).flatMap(item => item.scorer_types || item.scorerTypes || []).map(value => value.trim().toUpperCase()).filter(Boolean))]
      .filter(code => ["PM", "DEPARTMENT_MANAGER", "PRODUCT_MANAGER", "CTO"].includes(code))
    expect(scorerTypes.length, "模板至少需要一个受支持的评分职责").toBeGreaterThan(0)

    const scorerPassword = `Kpi-${Date.now().toString(36)}-scorer!Aa1`
    const scorerEmail = `acceptance-scorer-${Date.now()}@example.test`
    const scorerCreated = await api.authorized(hr.token, "POST", "/api/employees", {
      name: `真实闭环评分人-${Date.now()}`,
      email: scorerEmail,
      password: scorerPassword,
      position,
      department_id: departmentID,
      manager_id: managerID,
      role: "employee",
      is_active: true,
    })
    expect(scorerCreated.response.status(), JSON.stringify(scorerCreated.body)).toBe(201)
    scorer = bodyData<EmployeeRecord>(scorerCreated.body)
    scorerAuth = await api.login(scorerEmail, scorerPassword)
    expect(scorerAuth.permissions).toContain("assessment:submit")

    for (const code of scorerTypes) {
      const responsibility = await api.authorized(hr.token, "POST", "/api/evaluation-responsibilities", {
        code,
        name: `E2E ${code}`,
        scope_type: "position",
        department_id: departmentID,
        position_type: position,
        user_id: scorer.id,
        active: true,
      })
      expect(responsibility.response.status(), JSON.stringify(responsibility.body)).toBe(201)
      responsibilityIds.push(bodyData<ResponsibilityRecord>(responsibility.body).id)
    }

    const password = process.env.KPI_E2E_TEMP_PASSWORD || `Kpi-${Date.now().toString(36)}!Aa1`
    const email = `acceptance-${Date.now()}@example.test`
    const created = await api.authorized(hr.token, "POST", "/api/employees", {
      name: `真实闭环员工-${Date.now()}`,
      email,
      password,
      position,
      department_id: departmentID,
      manager_id: managerID,
      role: "employee",
      is_active: true,
    })
    expect(created.response.status(), JSON.stringify(created.body)).toBe(201)
    employee = bodyData<EmployeeRecord>(created.body)
    expect(employee.id).toBeGreaterThan(0)
    employeeAuth = await api.login(email, password)

    await createEvaluation(0)
    await createEvaluation(1)
  })

  test.afterAll(async () => {
    if (!api || !hr) {
      await requestContext?.dispose()
      return
    }
    // Delete only the exact IDs created by this run. Never use email prefixes,
    // status-wide deletes, or volume resets in a real service acceptance run.
    for (const evaluationID of [...evaluationIds].reverse()) {
      const deleted = await api.authorized(hr.token, "DELETE", `/api/evaluations/${evaluationID}`)
      expect([200, 404], `cleanup evaluation ${evaluationID}`).toContain(deleted.response.status())
      const gone = await api.authorized(hr.token, "GET", `/api/evaluations/${evaluationID}`)
      expect(gone.response.status(), `evaluation ${evaluationID} must be gone`).toBe(404)
      // The deleted evaluation must no longer expose its supplement listing;
      // the handler's transactional cleanup also removes stored attachments.
      const supplementsGone = await api.authorized(hr.token, "GET", `/api/evaluations/${evaluationID}/confirmation/supplements`)
      expect([403, 404], `supplements for deleted evaluation ${evaluationID}`).toContain(supplementsGone.response.status())
    }
    // Responsibilities have an audit-preserving deactivate API rather than a
    // hard delete. Deactivate only the exact IDs created by this run.
    for (const responsibilityID of responsibilityIds) {
      const deactivated = await api.authorized(hr.token, "DELETE", `/api/evaluation-responsibilities/${responsibilityID}`)
      expect([200, 404], `cleanup responsibility ${responsibilityID}`).toContain(deactivated.response.status())
    }
    if (employee?.id) {
      const deleted = await api.authorized(hr.token, "DELETE", `/api/employees/${employee.id}`)
      expect([200, 404], `cleanup employee ${employee.id}`).toContain(deleted.response.status())
      const gone = await api.authorized(hr.token, "GET", `/api/employees/${employee.id}`)
      expect(gone.response.status(), `employee ${employee.id} must be gone`).toBe(404)
    }
    if (scorer?.id) {
      const deleted = await api.authorized(hr.token, "DELETE", `/api/employees/${scorer.id}`)
      expect([200, 404], `cleanup scorer ${scorer.id}`).toContain(deleted.response.status())
      const gone = await api.authorized(hr.token, "GET", `/api/employees/${scorer.id}`)
      expect(gone.response.status(), `scorer ${scorer.id} must be gone`).toBe(404)
    }
    await requestContext?.dispose()
  })

  test("真实评分闭环：授权校验、零分、异议处理和纸质签字", async () => {
    const evaluationID = evaluationIds[0]
    const initialTasks = await completeScoring(evaluationID)
    const canonical = await api.authorized(hr.token, "GET", `/api/evaluations/${evaluationID}/result`)
    expect(canonical.response.status()).toBe(200)
    const result = bodyData<CanonicalResult>(canonical.body)
    expect(result.evaluation_id).toBe(evaluationID)
    expect(result.total_score).toBe(0)

    const exportsBeforeObjection = await exportPair(evaluationID)
    const batch = await api.authorized(hr.token, "GET", "/api/export/signoff-batch?status=awaiting_employee_confirmation")
    expect(batch.response.status(), JSON.stringify(batch.body)).toBe(200)
    const batchData = bodyData<ExportResponse>(batch.body)
    const batchResult = batchData.results?.find(item => item.evaluation_id === evaluationID)
    expect(batchResult).toMatchObject({
      evaluation_id: evaluationID,
      result_version: exportsBeforeObjection.xlsx.result_version,
      checksum: exportsBeforeObjection.xlsx.checksum,
    })

    const objection = await api.authorized(employeeAuth.token, "POST", `/api/evaluations/${evaluationID}/objection`, { reason: "真实闭环异议回归" })
    expect(objection.response.status(), JSON.stringify(objection.body)).toBe(200)
    const handled = await api.authorized(hr.token, "PUT", `/api/evaluations/${evaluationID}/objection/handle`, { final_comment: "异议已核查" })
    expect(handled.response.status(), JSON.stringify(handled.body)).toBe(200)

    const exportsAfterObjection = await exportPair(evaluationID)
    expect(exportsAfterObjection.xlsx.checksum).toBeTruthy()
    expect(exportsAfterObjection.xlsx.checksum).not.toBe(exportsBeforeObjection.xlsx.checksum)

    // A finalized assessment remains immutable even if a stale scorer submits
    // with the version that was valid before confirmation.
    const staleTask = initialTasks.tasks[0]
    const staleWrite = await api.authorized(scorerAuth.token, "POST", `/api/scoring-tasks/${staleTask.id}/submit`, {
      score: 0,
      comment: "确认后不应写入",
      not_applicable: false,
      version: staleTask.version,
    })
    expect(staleWrite.response.status(), JSON.stringify(staleWrite.body)).toBe(409)

    // Published template edits cannot rewrite a historical snapshot. The
    // route must reject the write, and exports remain byte-version identical.
    if (templateID) {
      const templateWrite = await api.authorized(hr.token, "PUT", `/api/templates/${templateID}`, { name: `不得修改-${Date.now()}` })
      expect(templateWrite.response.status(), JSON.stringify(templateWrite.body)).toBe(409)
      const frozenExport = await exportPair(evaluationID)
      expect(frozenExport.xlsx.checksum).toBe(exportsAfterObjection.xlsx.checksum)
      expect(frozenExport.xlsx.result_version).toBe(exportsAfterObjection.xlsx.result_version)
    }

    const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64")
    const paper = await requestContext.post(`${api.baseUrl}/api/evaluations/${evaluationID}/confirm-paper`, {
      headers: { Authorization: `Bearer ${hr.token}` },
      multipart: {
        signed_at: "2099-01-01",
        remark: "真实闭环纸质签字",
        file: { name: "signoff.png", mimeType: "image/png", buffer: png },
      },
    })
    const paperText = await paper.text()
    expect(paper.status(), paperText).toBe(200)
    const paperBody = JSON.parse(paperText) as unknown
    expect(bodyData<{ method: string }>(paperBody).method).toBe("paper")
    const attachment = await requestContext.get(`${api.baseUrl}/api/evaluations/${evaluationID}/confirmation/attachment`, { headers: { Authorization: `Bearer ${hr.token}` } })
    expect(attachment.status()).toBe(200)
    expect((await attachment.body()).length).toBeGreaterThan(20)
  })

  test("真实评分闭环：在线确认、快照一致性和统计", async () => {
    const evaluationID = evaluationIds[1]
    await completeScoring(evaluationID)
    const exports = await exportPair(evaluationID)
    const confirmed = await api.authorized(employeeAuth.token, "POST", `/api/evaluations/${evaluationID}/confirm-online`)
    expect(confirmed.response.status(), JSON.stringify(confirmed.body)).toBe(200)
    const confirmation = bodyData<{ method: string; snapshot?: { version: number; checksum: string } }>(confirmed.body)
    expect(confirmation.method).toBe("online")
    expect(confirmation.snapshot?.version).toBe(exports.xlsx.result_version)
    expect(confirmation.snapshot?.checksum).toBe(exports.xlsx.checksum)

    // An online confirmation completes the workflow, but HR can still append
    // one or more paper-signature supplements without mutating the immutable
    // confirmation or reopening the assessment.
    const completedBeforeSupplement = await api.authorized(hr.token, "GET", `/api/evaluations/${evaluationID}`)
    expect(completedBeforeSupplement.response.status(), JSON.stringify(completedBeforeSupplement.body)).toBe(200)
    expect(bodyData<EvaluationRecord>(completedBeforeSupplement.body).status).toBe("completed")

    const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64")
    const supplementUpload = await requestContext.post(`${api.baseUrl}/api/evaluations/${evaluationID}/confirmation/supplements`, {
      headers: { Authorization: `Bearer ${hr.token}` },
      multipart: {
        signed_at: "2099-01-02",
        remark: "在线确认后的纸质签字补充",
        file: { name: "supplement-1.png", mimeType: "image/png", buffer: png },
      },
    })
    const supplementText = await supplementUpload.text()
    expect(supplementUpload.status(), supplementText).toBe(200)
    const supplement = bodyData<SignoffSupplement>(JSON.parse(supplementText))
    expect(supplement.id).toBeGreaterThan(0)
    expect(supplement.evaluation_id).toBe(evaluationID)
    expect(supplement.attachment_name).toBe("supplement-1.png")

    const statusAfterSupplement = await api.authorized(hr.token, "GET", `/api/evaluations/${evaluationID}`)
    expect(statusAfterSupplement.response.status()).toBe(200)
    expect(bodyData<EvaluationRecord>(statusAfterSupplement.body).status).toBe("completed")

    const supplementsAfterFirstUpload = await api.authorized(hr.token, "GET", `/api/evaluations/${evaluationID}/confirmation/supplements`)
    expect(supplementsAfterFirstUpload.response.status(), JSON.stringify(supplementsAfterFirstUpload.body)).toBe(200)
    const firstRows = bodyData<SignoffSupplement[]>(supplementsAfterFirstUpload.body)
    expect(firstRows).toHaveLength(1)
    expect(firstRows[0]).toMatchObject({ id: supplement.id, attachment_name: "supplement-1.png", remark: "在线确认后的纸质签字补充" })

    const supplementAttachment = await requestContext.get(`${api.baseUrl}/api/evaluations/${evaluationID}/confirmation/supplements/${supplement.id}/attachment`, {
      headers: { Authorization: `Bearer ${hr.token}` },
    })
    expect(supplementAttachment.status()).toBe(200)
    expect((await supplementAttachment.body()).length).toBeGreaterThan(20)

    // Repeated uploads create separate evidence rows, preserving the audit
    // trail instead of overwriting the first uploaded document.
    const repeatedUpload = await requestContext.post(`${api.baseUrl}/api/evaluations/${evaluationID}/confirmation/supplements`, {
      headers: { Authorization: `Bearer ${hr.token}` },
      multipart: {
        remark: "重复上传补充材料",
        file: { name: "supplement-2.png", mimeType: "image/png", buffer: png },
      },
    })
    const repeatedText = await repeatedUpload.text()
    expect(repeatedUpload.status(), repeatedText).toBe(200)
    const repeated = bodyData<SignoffSupplement>(JSON.parse(repeatedText))
    expect(repeated.id).toBeGreaterThan(supplement.id)
    expect(repeated.evaluation_id).toBe(evaluationID)

    const supplementsAfterRepeat = await api.authorized(hr.token, "GET", `/api/evaluations/${evaluationID}/confirmation/supplements`)
    expect(supplementsAfterRepeat.response.status()).toBe(200)
    expect(bodyData<SignoffSupplement[]>(supplementsAfterRepeat.body)).toHaveLength(2)

    // The employee may view the completed result but cannot append HR-owned
    // paper evidence without assessment:approve.
    const unauthorizedUpload = await requestContext.post(`${api.baseUrl}/api/evaluations/${evaluationID}/confirmation/supplements`, {
      headers: { Authorization: `Bearer ${employeeAuth.token}` },
      multipart: { file: { name: "employee-upload.png", mimeType: "image/png", buffer: png } },
    })
    expect(unauthorizedUpload.status()).toBe(403)

    const signoffs = await api.authorized(hr.token, "GET", "/api/statistics/signoffs?status=completed")
    expect(signoffs.response.status(), JSON.stringify(signoffs.body)).toBe(200)
    const signoffData = bodyData<{ items: Array<{ evaluation_id: number; method: string; total_score: number }> }>(signoffs.body)
    expect(signoffData.items).toEqual(expect.arrayContaining([
      expect.objectContaining({ evaluation_id: evaluationIds[0], method: "paper", total_score: 0 }),
      expect.objectContaining({ evaluation_id: evaluationID, method: "online", total_score: 0 }),
    ]))

    const dashboard = await api.authorized(hr.token, "GET", "/api/statistics/dashboard")
    expect(dashboard.response.status(), JSON.stringify(dashboard.body)).toBe(200)
    const dashboardData = bodyData<{ completed?: number }>(dashboard.body)
    expect(dashboardData.completed).toBeGreaterThanOrEqual(2)
  })
})
