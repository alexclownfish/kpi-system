# Tasks: 跨部门兼任负责人

## Phase 1: 状态与设计
- [x] T001 记录真实现状及验收规格于 specs/015-cross-department-leaders/spec.md、plan.md、contracts/departments.md，完成Analyze。

## Phase 2: US1 负责人任命
- [x] T002 [US1] 在 server/handlers/cross_department_leader_test.go 复现跨部门任命/新建部门失败并覆盖权限、停用、交接审计。
- [x] T003 [US1] 在 server/handlers/department.go、server/models/models.go、server/routes/routes.go 实现候选、创建/替换、姓名展示及交接提示。

## Phase 3: US2 上级与评分
- [x] T004 [US2] 在 server/handlers/cross_department_leader_test.go、server/models/evaluation_workflow_models_test.go 覆盖普通员工创建、候选、自评循环、双职责任务、索引幂等升级与历史快照不变。
- [x] T005 [US2] 在 server/handlers/department_hierarchy.go 调整跨部门校验；在 evaluation_responsibility_resolver.go、scoring_task_factory.go、server/models/models.go、database.go 保留双职责任务并升级唯一索引；保留历史快照不变量；旧自动修复不跨部门修改兼任者上级。

## Phase 4: US3 数据范围
- [x] T006 [US3] 在 server/handlers/cross_department_leader_test.go 测试DEPARTMENT/TREE多部门与SELF/无关部门拒绝。
- [x] T007 [US3] 在 server/models/department_hierarchy.go、server/handlers/permissions.go、server/handlers/evaluation_responsibility.go 复用管理部门集合且不改变角色权限。

## Phase 5: 页面和回归
- [x] T008 [US1] 在 lib/api.ts、app/departments/page.tsx 实现候选分页、创建选负责人、跨部门姓名及交接提醒；在 tests/unit/department-hierarchy-form.test.mjs、tests/e2e/cross-department-leaders.spec.ts 加回归。
- [x] T009 在 specs/015-cross-department-leaders/acceptance.md 记录Go全套、TS、生产构建、隔离PG与评分流程回归结果，完成Converge；quickstart.md给出页面验收方法。

依赖T001→T002/T004/T006→T003/T005/T007→T008→T009；本次按顺序执行，不委派子代理。
