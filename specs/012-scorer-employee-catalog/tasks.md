# Tasks: 评分职责候选员工完整加载

**Input**: 本目录 spec.md、plan.md、research.md、data-model.md、contracts/、quickstart.md
**Tests**: 规格明确要求回归及实际页面测试。

## Phase 1: Setup

- [x] T001 检查现有分页契约及安全边界，在 specs/012-scorer-employee-catalog/research.md 记录根因及不扩大授权边界。

## Phase 2: Foundational

- [x] T002 为 tests/unit/employee-catalog.test.mjs 添加完整目录回归：0/17/100/101/205 人、合法页大小、角色无过滤、过滤透传、后页失败；先运行确认旧代码失败。

## Phase 3: US1 完整候选名单 (P1 MVP)

**Independent Test**: 超过 10 和 100 人不遗漏；后端范围内允许、范围外和停用拒绝。

- [x] T003 [P] [US1] 在 server/handlers/employee_catalog_test.go 验证分页排序、205 人、停用过滤及数据范围。
- [x] T004 [US1] 在 lib/api.ts 增加 employeeApi.getCatalog，保留过滤、每页 100、读完全部页，失败整体 reject；在 server/handlers/employee.go 固定 employees.id ASC。
- [x] T005 [US1] 在 app/evaluation-responsibilities/page.tsx 使用完整启用员工目录，不添加 role 过滤。

## Phase 4: US2 不完整加载保护 (P2)

**Independent Test**: 加载/失败不能误保存，空名单有提示，刷新可恢复。

- [x] T006 [US2] 在 tests/unit/employee-catalog.test.mjs 为页面加载失败、空名单、操作门禁增加回归断言，先确认失败。
- [x] T007 [US2] 在 app/evaluation-responsibilities/page.tsx 清空旧数据、显示加载错误/空名单并禁用新增、编辑、评分人选择、保存；保存函数校验门禁。

## Phase 5: Validation

- [x] T008 运行 Node 全量单测、Go 全量测试、tsc、目标 ESLint、Next production build，将结果记录 specs/012-scorer-employee-catalog/acceptance.md。
- [x] T009 使用私有克隆数据在真实页面分别选择 CTO/子部门负责人/普通员工、编辑既有职责及移动视口；不保存实际职责，将证据记录 specs/012-scorer-employee-catalog/acceptance.md。

## Dependencies & Execution Order

T001 → T002/T003 → T004 → T005；T006 → T007；全部 → T008 → T009 → converge。
T003 可与 T002 并行；同文件实现顺序执行，不开启新开发会话。

## Implementation Strategy

先实现 US1 完整目录 MVP，再 US2 安全反馈。两故事分别可验收，但全部质量门禁后才交付。正式 Docker 由用户构建；无部署、初始化或业务数据更改。
