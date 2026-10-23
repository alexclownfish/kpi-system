import { expect, test } from "@playwright/test"

test("员工详情在自评前展示工作总结与后续计划门禁", async ({ page }) => {
  const user = { id: 1, name: "总结员工", email: "summary-ui@example.test", role: "employee", department_id: 1, permissions: ["assessment:view", "assessment:submit"], data_scope: "SELF" }
  await page.addInitScript(currentUser => {
    localStorage.setItem("__dootask_kpi__auth_token", "work-summary-ui-token")
    localStorage.setItem("__dootask_kpi__user_info", JSON.stringify(currentUser))
  }, user)
  const evaluation = {
    id: 101, employee_id: 1, template_id: 1, period: "monthly", year: 2026, month: 10,
    status: "self_evaluating", total_score: 0, calculation_version: "weighted-v2",
    work_summary_required: true, final_comment: "", has_objection: false, objection_reason: "",
    created_at: "2026-10-21T00:00:00Z", employee: { id: 1, name: "总结员工", department_id: 1 },
    template: { id: 1, name: "软件应用开发部 / 工程师项目绩效考核", period: "monthly", calculation_version: "weighted-v2" },
  }
  const errors: string[] = []
  page.on("pageerror", error => errors.push(error.message))
  await page.route("**/api/**", async route => {
    const url = new URL(route.request().url())
    const path = url.pathname
    let body: unknown = { data: [], total: 0, totalPages: 1 }
    if (path === "/api/settings") body = { data: { system_mode: "standalone", allow_registration: false } }
    if (path === "/api/me") body = { data: user, permissions: user.permissions, data_scope: user.data_scope }
    if (path === "/api/evaluations") body = { data: [evaluation], total: 1, page: 1, pageSize: 10, totalPages: 1, hasNext: false, hasPrev: false, stats: { total: 1, pending: 1, completed: 0, avgScore: 0 } }
    if (path === "/api/evaluations/101") body = { data: evaluation }
    if (path === "/api/evaluations/101/scores") body = { data: [], total: 0 }
    if (path === "/api/evaluations/101/scoring-tasks") body = { data: { evaluation_id: 101, workflow_state: "self_evaluating", completion: { required: 0, submitted: 0, blocking: 0 }, tasks: [] } }
    if (path === "/api/evaluations/101/comments") body = { data: [], total: 0, page: 1, pageSize: 5, totalPages: 1 }
    if (path === "/api/evaluations/101/confirmation") body = { data: null }
    if (path === "/api/evaluations/101/result") {
      await route.fulfill({ status: 404, json: { error: { code: "RESULT_NOT_FOUND", message: "尚未生成结果" } } })
      return
    }
    await route.fulfill({ json: body })
  })

  await page.goto("/evaluations")
  await page.getByRole("row").filter({ hasText: "总结员工" }).getByRole("button").first().click()
  await expect(page.getByText("工作总结与后续计划", { exact: true })).toBeVisible()
  await expect(page.getByLabel("本期工作总结")).toBeVisible()
  await expect(page.getByLabel("问题与风险")).toBeVisible()
  await expect(page.getByLabel("所需支持")).toBeVisible()
  await expect(page.getByLabel("下期计划、交付物与时间")).toBeVisible()
  await expect(page.getByRole("button", { name: "提交总结并继续" })).toBeVisible()
  expect(errors).toEqual([])
})
