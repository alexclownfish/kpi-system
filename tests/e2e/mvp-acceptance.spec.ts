import { request as requestFactory, type APIRequestContext } from "@playwright/test"
import { test, expect } from "./fixtures/acceptance"
import { CleanupRegistry } from "./helpers/cleanup"
import { createRunId } from "./helpers/acceptance-context"
import { AcceptanceApiClient, type LoginResult } from "./helpers/api-client"

type EmployeeRecord = {
  id: number
  name: string
  email: string
  role: string
  department_id: number
  manager_id?: number
}

test.use({ trace: "off" })

test.describe.serial("MVP acceptance", () => {
  const runId = createRunId()
  const password = process.env.KPI_TEST_TEMP_PASSWORD || `Kpi-${runId.slice(-8)}!`
  const cleanup = new CleanupRegistry()
  let api: AcceptanceApiClient
  let requestContext: APIRequestContext
  let hr: LoginResult
  let departmentId: number
  let departmentName: string
  let manager: EmployeeRecord
  let employee: EmployeeRecord

  test.beforeAll(async () => {
    requestContext = await requestFactory.newContext()
    api = new AcceptanceApiClient(requestContext, (process.env.KPI_BASE_URL || "http://localhost").replace(/\/$/, ""))
    await api.health()
    const hrPassword = process.env.KPI_TEST_HR_PASSWORD
    if (!hrPassword) throw new Error("Missing required environment variable KPI_TEST_HR_PASSWORD")
    hr = await api.login(process.env.KPI_TEST_HR_EMAIL || "sunba@company.com", hrPassword)
    expect(hr.permissions).toContain("employee:create")
    const departmentsResponse = await requestContext.get(`${api.baseUrl}/api/auth/departments`)
    expect(departmentsResponse.status()).toBe(200)
    const departments = await departmentsResponse.json() as { data: Array<{ id: number; name: string }> }
    expect(departments.data.length).toBeGreaterThan(0)
    departmentId = departments.data[0].id
    departmentName = departments.data[0].name
  })

  test.afterAll(async () => {
    try {
      await cleanup.cleanup(async record => {
        const deletion = await api.authorized(hr.token, "DELETE", `/api/employees/${record.id}`)
        expect(deletion.response.status()).toBe(200)
        const verification = await api.authorized(hr.token, "GET", `/api/employees/${record.id}`)
        expect(verification.response.status()).toBe(404)
      })
    } finally {
      await requestContext?.dispose()
    }
  })

  test("US1-AC2 HR creates a department manager who can log in", async () => {
    const email = `${runId}-manager@example.test`
    const created = await api.authorized(hr.token, "POST", "/api/employees", {
      name: `${runId} 负责人`, email, password, position: "验收负责人",
      department_id: departmentId, role: "department_manager", is_active: true,
    })
    expect(created.response.status()).toBe(201)
    manager = (created.body as { data: EmployeeRecord }).data
    cleanup.register({ resource: "employee", id: manager.id, label: email, dependsOn: [], cleanupOrder: 20 })
    const login = await api.login(email, password)
    expect(login.user.role).toBe("manager")
    expect(login.data_scope).toBe("DEPARTMENT")
  })

  test("US1-AC1 HR page creates an employee and shows success feedback", async ({ page }) => {
    const email = `${runId}-employee@example.test`
    await page.goto("/auth/login")
    await page.getByLabel("邮箱").fill(process.env.KPI_TEST_HR_EMAIL || "sunba@company.com")
    await page.getByLabel("密码").fill(process.env.KPI_TEST_HR_PASSWORD || "")
    await page.getByRole("button", { name: "登录", exact: true }).click()
    await page.waitForFunction(() => Boolean(localStorage.getItem("__dootask_kpi__auth_token")))
    await page.goto("/employees")
    await expect(page.getByRole("heading", { name: "员工管理" })).toBeVisible()
    await page.getByRole("button", { name: "添加员工" }).click()
    await page.getByLabel("姓名").fill(`${runId} 普通员工`)
    await page.getByLabel("邮箱").fill(email)
    await page.getByLabel("职位").fill("验收员工")
    await page.getByLabel("初始密码", { exact: true }).fill(password)
    await page.getByLabel("确认初始密码", { exact: true }).fill(password)
    const selects = page.getByRole("combobox")
    await selects.nth(0).click()
    await page.getByRole("option", { name: departmentName }).click()
    await selects.nth(2).click()
    await page.getByRole("option", { name: new RegExp(manager.name) }).click()
    const submit = page.getByTestId("employee-submit")
    await submit.click()
    await expect(page.getByTestId("app-alert")).toContainText("创建成功")
    await page.getByRole("button", { name: "确定" }).click()

    const lookup = await api.authorized(hr.token, "GET", `/api/employees?search=${encodeURIComponent(email)}&pageSize=10`)
    expect(lookup.response.status()).toBe(200)
    const matches = (lookup.body as { data: EmployeeRecord[] }).data
    employee = matches.find(item => item.email === email) as EmployeeRecord
    expect(employee?.manager_id).toBe(manager.id)
    cleanup.register({ resource: "employee", id: employee.id, label: email, dependsOn: [manager.id], cleanupOrder: 10 })
    const login = await api.login(email, password)
    expect(login.user.role).toBe("employee")
    expect(login.data_scope).toBe("SELF")

    await page.getByRole("button", { name: "添加员工" }).click()
    await page.getByLabel("姓名").fill(`${runId} 重复员工`)
    await page.getByLabel("邮箱").fill(email)
    await page.getByLabel("职位").fill("重复验证")
    await page.getByLabel("初始密码", { exact: true }).fill(password)
    await page.getByLabel("确认初始密码", { exact: true }).fill(password)
    const retrySelects = page.getByRole("combobox")
    await retrySelects.nth(0).click()
    await page.getByRole("option", { name: departmentName }).click()
    await page.getByTestId("employee-submit").click()
    await expect(page.getByTestId("app-alert")).toContainText("普通员工必须选择直属上级")
    await page.getByRole("button", { name: "确定" }).click()
    await retrySelects.nth(2).click()
    await page.getByRole("option", { name: new RegExp(manager.name) }).click()
    await page.getByTestId("employee-submit").click()
    await expect(page.getByTestId("app-alert")).toContainText("邮箱已被使用")
    await page.getByRole("button", { name: "确定" }).click()
    const duplicateLookup = await api.authorized(hr.token, "GET", `/api/employees?search=${encodeURIComponent(email)}&pageSize=10`)
    expect((duplicateLookup.body as { data: EmployeeRecord[] }).data.filter(item => item.email === email)).toHaveLength(1)
  })

  test("US1-AC3 invalid and duplicate creation return actionable 4xx errors", async () => {
    const common = { name: "拒绝场景", position: "验收", department_id: departmentId, role: "employee", is_active: true }
    const missingPassword = await api.authorized(hr.token, "POST", "/api/employees", { ...common, email: `${runId}-no-password@example.test`, manager_id: manager.id })
    expect(missingPassword.response.status()).toBe(400)
    expect(JSON.stringify(missingPassword.body)).toContain("密码")
    const missingManager = await api.authorized(hr.token, "POST", "/api/employees", { ...common, email: `${runId}-no-manager@example.test`, password })
    expect(missingManager.response.status()).toBe(400)
    expect(JSON.stringify(missingManager.body)).toContain("直属上级")
    const duplicate = await api.authorized(hr.token, "POST", "/api/employees", { ...common, email: employee.email, password, manager_id: manager.id })
    expect(duplicate.response.status()).toBe(409)
    expect(JSON.stringify(duplicate.body)).toContain("邮箱")
  })

  test("US2-AC1 employee is denied employee creation and company statistics without leaked data", async () => {
    const employeeAuth = await api.login(employee.email, password)
    const create = await api.authorized(employeeAuth.token, "POST", "/api/employees", {})
    expect(create.response.status()).toBe(403)
    expect(JSON.stringify(create.body)).not.toContain(manager.email)
    const statistics = await api.authorized(employeeAuth.token, "GET", "/api/statistics/dashboard")
    expect(statistics.response.status()).toBe(403)
    expect(JSON.stringify(statistics.body)).not.toContain("total_employees")
  })

  test("US2-AC2 and US2-AC3 SELF and DEPARTMENT scopes constrain employee results", async () => {
    const employeeAuth = await api.login(employee.email, password)
    const selfResult = await api.authorized(employeeAuth.token, "GET", "/api/employees?pageSize=100")
    expect(selfResult.response.status()).toBe(200)
    const selfEmployees = (selfResult.body as { data: EmployeeRecord[] }).data
    expect(selfEmployees.map(item => item.id)).toEqual([employee.id])

    const managerAuth = await api.login(manager.email, password)
    const departmentResult = await api.authorized(managerAuth.token, "GET", "/api/employees?pageSize=100")
    expect(departmentResult.response.status()).toBe(200)
    const departmentEmployees = (departmentResult.body as { data: EmployeeRecord[] }).data
    expect(departmentEmployees.some(item => item.id === employee.id)).toBeTruthy()
    expect(departmentEmployees.every(item => item.department_id === departmentId)).toBeTruthy()
  })

  test("US2-AC1 direct page access by employee is rejected or redirected", async ({ page }) => {
    const employeeAuth = await api.login(employee.email, password)
    await page.addInitScript(({ token, user }) => {
      localStorage.setItem("__dootask_kpi__auth_token", token)
      localStorage.setItem("__dootask_kpi__user_info", JSON.stringify(user))
    }, { token: employeeAuth.token, user: { ...employeeAuth.user, permissions: employeeAuth.permissions, data_scope: employeeAuth.data_scope } })
    await page.goto("/employees")
    await expect(page).not.toHaveURL(/\/employees$/)
    await page.goto("/statistics")
    await expect(page).not.toHaveURL(/\/statistics$/)
  })
})
