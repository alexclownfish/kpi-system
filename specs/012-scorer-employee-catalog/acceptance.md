# Acceptance: 评分职责候选员工完整加载

**Date**: 2026-10-02
**Scope**: Feature 012，不代表整个考核工作流验收完成。

## Spec Kit

specify：规格及质量清单 16/16；clarify：0 个关键歧义，无需询问；plan：研究、数据模型、契约、人工验收指南完成；tasks：9 项依赖有序任务。

实现前 analyze 只读审核：FR-001/002 → T002/003/004/005/009；FR-003 → T003/004；FR-004/005 → T006/007/009；FR-006 → T001/009。SC-001 → T002/003/009；SC-002 → T006/007/009；SC-003 → T003/009。6/6 功能要求和 3/3 成功标准均有任务；无歧义、重复、未映射任务、宪章冲突或 CRITICAL/HIGH 问题。

## Automated Evidence

- 新测试在实现前 0/9 通过，复现缺失完整目录、旧 pageSize=500 和缺失保存门禁。
- 实现后 employee-catalog.test.mjs 9/9：0、17、100、101、205 人完整；每页 100；传递过滤不额外按角色过滤；null 空 data；后页失败整体拒绝；清除旧目录及刷新恢复；保存门禁。
- Node 全量单测 14/14 通过。
- 后端 go test ./... 全量通过；新 EmployeeCatalog 测试验证 205 人无重漏、id 排序、停用过滤、空搜索、部门范围允许/范围外拒绝及撤销查看权限后 403。密码不出现在响应。
- 首次后端运行发现测试夹具 false 被 GORM 默认 true 覆盖，已显式停用测试员工后复测通过；未更改正式员工数据。
- tsc --noEmit、目标 ESLint 均通过。
- Next.js production build 通过；仅既有 app/templates/page.tsx 的 fetchTemplates Hook 依赖警告，本次未扩大修改范围。

## Real Browser Evidence

仅私有克隆数据库和临时回环服务，不修改正常 Compose 服务。

1. 新增评分职责的评分人列表实际出现 17 个选项，而非原先 10 个。
2. 实际选中 CTO 朱亮、嵌入式子部门负责人毛一凡、软件应用子部门负责人王成斌、普通员工王绍品；职责类型 CTO 也可选。
3. 编辑既有公司 CTO 职责时，朱亮姓名邮箱正确回显；全部取消，不保存业务职责。
4. 390x844 手机测试：原长姓名把控件撑至 468px；仅在该评分人控件增加宽度限制和长内容处理后，控件宽 308px、右边界 349px，在屏幕内。长姓名项键盘选中并回显，选项可换行。
5. 停止隔离后端后，刷新出现加载失败提示；确认后旧数据为空、新增禁用、刷新可用。恢复隔离后端后刷新成功、新增恢复、17 名员工仍完整，再次选中 CTO。

截图位于临时目录 `/private/tmp/kpi-hierarchy-runtime.aCSIPx/`：scorer-cto-selected.jpg、scorer-catalog-complete.jpg、scorer-mobile-options.jpg、scorer-mobile-long-fixed.jpg、scorer-load-error.jpg。

## Boundaries

没有更改实际角色、权限、部门负责人、评分职责或既有考核。没有重置密码、清理数据库或执行正式 Compose 构建。工作流 audit/enabled 模式不受本次修复影响。

更新方法与页面测试见 [quickstart.md](quickstart.md)。测试结束停止本轮临时服务并关闭测试标签，恢复浏览器视口。
