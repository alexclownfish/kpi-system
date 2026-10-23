import { expect, test, type Page } from "@playwright/test"

// Isolated UI regression: mock only HTTP responses, exercise the real page and
// alert/toast components without writing to the user's business database.
const layout = {
  version: 1, preset: "final_signoff", title: "原始结果确认表", orientation: "portrait",
  columns: ["item_name", "max_score", "final_score", "final_comment"],
  show_summary: true, show_employee_opinion: true, signature_labels: ["员工签字", "PM签字"],
}
async function setup(page: Page, status: string) {
  const user = { id: 1, name: "测试管理员", email: "ui-regression@example.test", permissions: ["kpi:view", "kpi:edit"], data_scope: "ALL" }
  await page.addInitScript(user => {
    localStorage.setItem("__dootask_kpi__auth_token", "ui-regression-token")
    localStorage.setItem("__dootask_kpi__user_info", JSON.stringify(user))
  }, user)
  let templates = [{ id: 41, name: "版式回归模板", description: "", period: "monthly", status, version: 1, is_active: status === "published", created_at: "2026-10-08", export_layout: layout }]
  let failSave = false
  let writes = 0
  const errors: string[] = []
  page.on("pageerror", error => errors.push(error.message))
  await page.route("**/api/**", async route => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    let body: unknown = { data: [], total: 0, totalPages: 1 }
    let responseStatus = 200
    if (path === "/api/settings") body = { data: { system_mode: "standalone" } }
    if (path === "/api/me") body = { data: user, permissions: user.permissions, data_scope: "ALL" }
    if (path === "/api/templates") body = { data: templates, total: templates.length }
    if (path === "/api/templates/41/clone") {
      writes++
      const cloned = { ...templates[0], id: 42, version: 2, status: "draft", name: "版式回归模板 V2", is_active: false }
      templates = [...templates, cloned]
      body = { data: cloned, message: "模板复制成功" }
      responseStatus = 201
    }
    if (/\/api\/templates\/\d+$/.test(path) && method === "PUT") {
      writes++
      if (failSave) {
        responseStatus = 409
        body = { error: { code: "PUBLISHED_TEMPLATE_IMMUTABLE", message: "已发布或已停用模板不可直接修改，请复制为新版本" }, request_id: "ui-regression-save" }
      } else {
        const id = Number(path.split("/").at(-1))
        templates = templates.map(template => template.id === id ? { ...template, ...route.request().postDataJSON() } : template)
        body = { data: templates.find(template => template.id === id) }
      }
    }
    if (path.endsWith("/calculation-structure") && method === "PUT") {
      writes++
      responseStatus = 409
      body = { error: { code: "TEMPLATE_LOCKED", message: "模板状态已变化，请刷新后重试" } }
    }
    await route.fulfill({ status: responseStatus, json: body, headers: { "Access-Control-Allow-Origin": "*" } })
  })
  await page.goto("/templates")
  if (status === "retired") {
    await page.getByRole("button", { name: "显示历史版本" }).click()
  }
  await page.getByRole("row").filter({ hasText: "版式回归模板" }).getByRole("button").first().click()
  await page.getByRole("tab", { name: "结果导出设置" }).click()
  return { errors, writes: () => writes, failSave: (value: boolean) => { failSave = value } }
}

for (const status of ["published", "retired"]) {
  test(`${status}版式只读，复制新版本后能保存`, async ({ page }) => {
    const state = await setup(page, status)
    await expect(page.getByRole("button", { name: "保存版式" })).toBeDisabled()
    await expect(page.locator("#export-title")).toBeDisabled()
    await expect(page.getByText("历史考核必须保持原版式", { exact: false })).toBeVisible()
    expect(state.writes()).toBe(0)
    await page.getByRole("button", { name: "复制新版本后编辑" }).click()
    await expect(page.locator("#export-title")).toBeEnabled()
    await page.locator("#export-title").fill("新版本自定义标题")
    await page.getByRole("combobox").first().click()
    await page.getByRole("option", { name: "评分过程完整版", exact: true }).click()
    // A preset resets the title as part of its configuration.
    await page.locator("#export-title").fill("新版本自定义标题")
    await expect(page.getByRole("checkbox", { name: "自评分", exact: true })).toBeChecked()
    await page.getByRole("combobox").last().click()
    await page.getByRole("option", { name: "横向（A4）", exact: true }).click()
    await expect(page.getByText("A4 横向 · 签字栏 4 个", { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "保存版式" }).click()
    await expect(page.getByText("结果导出版式已保存", { exact: true })).toBeVisible()
    await page.getByRole("tab", { name: "计算结构", exact: true }).click()
    await page.getByRole("tab", { name: "结果导出设置", exact: true }).click()
    await expect(page.locator("#export-title")).toHaveValue("新版本自定义标题")
    await expect(page.getByRole("checkbox", { name: "自评分", exact: true })).toBeChecked()
    await expect(page.getByText("A4 横向 · 签字栏 4 个", { exact: true })).toBeVisible()
    await page.getByRole("tab", { name: "模板列表", exact: true }).click()
    await expect(page.getByRole("row").filter({ hasText: "版式回归模板 V2" })).toBeVisible()
    expect(state.writes()).toBe(2)
    expect(state.errors).toEqual([])
  })
}

test("保存收到结构化错误可关闭并重试，不白屏、不误报成功", async ({ page }) => {
  const state = await setup(page, "draft")
  state.failSave(true)
  await page.locator("#export-title").fill("待保存的标题")
  await page.getByRole("button", { name: "保存版式" }).click()
  await expect(page.getByTestId("app-alert")).toContainText("请复制为新版本")
  await expect(page.getByText("结果导出版式已保存", { exact: true })).toHaveCount(0)
  await page.getByTestId("app-alert").getByRole("button", { name: "确定" }).click()
  await expect(page.locator("#export-title")).toHaveValue("待保存的标题")
  state.failSave(false)
  await page.getByRole("button", { name: "保存版式" }).click()
  await expect(page.getByText("结果导出版式已保存", { exact: true })).toBeVisible()
  expect(state.errors).toEqual([])
})

test("计算结构保存的结构化错误显示为文字提示，不白屏", async ({ page }) => {
  const state = await setup(page, "draft")
  await page.getByRole("tab", { name: "计算结构", exact: true }).click()
  await page.locator("input").nth(0).fill("交付质量")
  await page.locator("input").nth(2).fill("任务完成率")
  await page.getByRole("button", { name: "保存结构", exact: true }).click()
  await expect(page.getByText("模板状态已变化，请刷新后重试", { exact: true })).toBeVisible()
  await expect(page.getByRole("button", { name: "保存结构", exact: true })).toBeEnabled()
  expect(state.writes()).toBe(1)
  expect(state.errors).toEqual([])
})
