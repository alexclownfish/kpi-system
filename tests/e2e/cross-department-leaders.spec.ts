import { expect, test, type Page } from "@playwright/test"

// Browser interactions use mocked APIs only: no business database is touched.
async function setupDepartmentPage(page: Page, editable = true) {
  const permissions = ["department:view", ...(editable ? ["department:create", "department:edit"] : [])]
  const user = { id: 1, name: "验收管理员", email: "ui-test@example.invalid", role: "employee", permissions, data_scope: "ALL" }
  const departments = [
    { id: 1, name: "总部", description: "", created_at: "2026-10-28T00:00:00Z", employees: [] },
    { id: 2, name: "运控部", description: "", leader_id: 101, leader: { id: 101, name: "兼任CTO", position: "CTO", is_active: true }, created_at: "2026-10-28T00:00:00Z", employees: [] },
  ]
  const candidatePages: number[] = []
  const writes: Array<{ method: string; body: Record<string, unknown> }> = []
  await page.addInitScript(userInfo => {
    localStorage.setItem("__dootask_kpi__auth_token", "mock-token-for-ui-only")
    localStorage.setItem("__dootask_kpi__user_info", JSON.stringify(userInfo))
  }, user)
  await page.route("**/api/**", async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const headers = { "access-control-allow-origin": "*", "access-control-allow-headers": "*", "access-control-allow-methods": "*" }
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers })
    let body: unknown = { data: [] }
    if (path.endsWith("/settings")) body = { data: { system_mode: "standalone", system_name: "KPI测试" } }
    if (path.endsWith("/me")) body = { data: user, permissions, data_scope: "ALL" }
    if (path.endsWith("/departments/leader-candidates")) {
      const candidatePage = Number(new URL(request.url()).searchParams.get("page"))
      candidatePages.push(candidatePage)
      body = { data: candidatePage === 1
        ? Array.from({ length: 100 }, (_, index) => ({ id: index + 1, name: `候选${index + 1}`, position: "工程师", department_id: 1, is_active: true }))
        : [{ id: 101, name: "兼任CTO", position: "CTO", department_id: 1, is_active: true }], total: 101, totalPages: 2, page: candidatePage, pageSize: 100 }
    } else if (path.endsWith("/departments") && request.method() === "GET") {
      body = { data: departments, total: 2, totalPages: 1, page: 1, pageSize: 100 }
    } else if (path.match(/\/departments(?:\/\d+)?$/) && ["POST", "PUT"].includes(request.method())) {
      writes.push({ method: request.method(), body: request.postDataJSON() })
      body = { data: departments[1], previous_leader_direct_reports: request.method() === "PUT" ? 3 : 0 }
    }
    return route.fulfill({ status: 200, contentType: "application/json", headers, body: JSON.stringify(body) })
  })
  await page.goto("/departments")
  await expect(page.getByRole("heading", { name: "部门管理", exact: true })).toBeVisible()
  return { candidatePages, writes }
}

test("create department uses a cross-department leader from a later candidate page", async ({ page }) => {
  const { candidatePages, writes } = await setupDepartmentPage(page)
  await page.getByRole("button", { name: "添加部门", exact: true }).click()
  await page.getByLabel("部门名称", { exact: true }).fill("结构部")
  await expect(page.locator("#department-leader")).toBeEnabled()
  await page.locator("#department-leader").click()
  await page.getByRole("option", { name: "兼任CTO（CTO · 总部）", exact: true }).click()
  await page.getByRole("button", { name: "创建", exact: true }).click()
  await expect(page.getByText("部门保存成功", { exact: true })).toBeVisible()
  expect(candidatePages).toEqual([1, 2])
  expect(writes).toEqual([{ method: "POST", body: { name: "结构部", description: "", parent_id: null, leader_id: 101 } }])
})

test("edit shows external leader and warns about direct reports when clearing", async ({ page }) => {
  const { writes } = await setupDepartmentPage(page)
  await expect(page.getByRole("cell", { name: "兼任CTO", exact: true })).toBeVisible()
  await page.getByRole("button", { name: "编辑 运控部", exact: true }).click()
  await expect(page.locator("#department-leader")).toBeEnabled()
  await page.locator("#department-leader").click()
  await page.getByRole("option", { name: "未指定", exact: true }).click()
  await page.getByRole("button", { name: "更新", exact: true }).click()
  await expect(page.getByText(/旧负责人仍是 3 名员工的直属上级/)).toBeVisible()
  expect(writes[0]?.body.leader_id).toBeNull()
})

test("read-only department users do not fetch staff candidates", async ({ page }) => {
  const { candidatePages } = await setupDepartmentPage(page, false)
  await expect(page.getByRole("button", { name: "添加部门", exact: true })).toBeDisabled()
  await expect(page.getByRole("button", { name: "编辑 运控部", exact: true })).toBeDisabled()
  expect(candidatePages).toEqual([])
})

test("candidate load failures are visible and block accidental saves", async ({ page }) => {
  const { writes } = await setupDepartmentPage(page)
  await page.route("**/api/departments/leader-candidates?**", route => route.fulfill({
    status: 500,
    contentType: "application/json",
    headers: { "access-control-allow-origin": "*" },
    body: JSON.stringify({ error: "负责人候选加载失败" }),
  }))
  await page.getByRole("button", { name: "添加部门", exact: true }).click()
  await expect(page.getByRole("alert")).toHaveText("负责人候选加载失败")
  await expect(page.getByRole("button", { name: "创建", exact: true })).toBeDisabled()
  expect(writes).toEqual([])
})
