import { expect, test } from "@playwright/test"

test("模板列表默认隐藏历史版本，切换后仅允许清理未发布版本", async ({ page }) => {
  const user = { id: 1, name: "测试管理员", email: "template-history@example.test", permissions: ["kpi:view", "kpi:edit", "kpi:delete"], data_scope: "ALL" }
  await page.addInitScript(currentUser => {
    localStorage.setItem("__dootask_kpi__auth_token", "template-history-token")
    localStorage.setItem("__dootask_kpi__user_info", JSON.stringify(currentUser))
  }, user)
  const errors: string[] = []
  page.on("pageerror", error => errors.push(error.message))
  const templates = [
    { id: 1, name: "当前工程师模板 V4", description: "current", period: "monthly", department_id: 1, position_type: "工程师", status: "published", version: 4, is_active: true, created_at: "2026-10-21" },
    { id: 2, name: "工程师模板 V3", description: "history", period: "monthly", department_id: 1, position_type: "工程师", status: "retired", version: 3, is_active: false, created_at: "2026-10-01" },
    { id: 3, name: "工程师模板 V5", description: "draft", period: "monthly", department_id: 1, position_type: "工程师", status: "draft", version: 5, is_active: false, created_at: "2026-10-21" },
  ]
  await page.route("**/api/**", async route => {
    const path = new URL(route.request().url()).pathname
    let body: unknown = { data: [], total: 0 }
    if (path === "/api/settings") body = { data: { system_mode: "standalone" } }
    if (path === "/api/me") body = { data: user, permissions: user.permissions, data_scope: "ALL" }
    if (path === "/api/departments/catalog") body = { data: [{ id: 1, name: "软件应用开发部" }] }
    if (path === "/api/templates") body = { data: templates, total: templates.length }
    await route.fulfill({ json: body })
  })

  await page.goto("/templates")
  await expect(page.getByRole("row").filter({ hasText: "当前工程师模板" })).toBeVisible()
  await expect(page.getByRole("row").filter({ hasText: "工程师模板 V5" })).toBeVisible()
  await expect(page.getByRole("row").filter({ hasText: "工程师模板 V3" })).toHaveCount(0)
  await page.getByRole("button", { name: "显示历史版本" }).click()
  const retiredRow = page.getByRole("row").filter({ hasText: "工程师模板 V3" })
  await expect(retiredRow).toBeVisible()
  await expect(retiredRow.getByRole("button", { name: "删除未引用版本" })).toBeEnabled()
  const publishedRow = page.getByRole("row").filter({ hasText: "当前工程师模板" })
  await expect(publishedRow.getByRole("button", { name: "请先停用当前版本，再清理未引用的历史版本" })).toBeDisabled()
  expect(errors).toEqual([])
})
