# Tasks: 部门层级与考核创建一致性

## Phase 1: Setup
- [X] T001 在 `specs/009-department-hierarchy-assessment/research.md` 记录真实岗位404、空职责和旧预览/错误对象的根因，完成规格澄清及设计。
## Phase 2: Foundation
- [X] T002 在 `server/handlers/department_hierarchy_test.go` 和 `tests/unit/api-errors.test.mjs` 先写层级、岗位匹配、预览错误回归并确认失败。
## Phase 3: US1 创建入口 MVP
**Independent Test**: 两种错误显示文本；同模式预览保留模板/阻塞原因；岗位前缀匹配且不改员工。
- [X] T003 [US1] 在 `server/handlers/evaluation_template.go`、`kpi.go` 复用目录标准岗位、按当前模式解析预览并检查员工范围，满足 FR-002/FR-009。
- [X] T004 [US1] 在 `lib/api.ts`、`app/evaluations/page.tsx` 安全错误文本、匹配/阻塞分离、提交状态及防重复，满足 FR-001。
## Phase 4: US2 部门层级
**Independent Test**: 新旧库幂等补四子部门；更新拒环/删除保护，HR改动保留。
- [X] T005 [US2] 在 `server/models/models.go`、`department_hierarchy.go` 添加nullable parent_id、遍历及子树查询；在 `server/handlers/department_hierarchy.go`、`hongxing_catalog.go` 实现独立marker事务补丁，满足 FR-003/FR-004。
- [X] T006 [US2] 在 `server/handlers/department.go` 校验父存在/自引用/环、更新可清空parent且省略保持、保护有子部门删除；在 `app/departments/page.tsx`、`lib/api.ts` 实现可见父部门维护及提交反馈，满足 FR-003。
## Phase 5: US3 上级考核
**Independent Test**: 工程师本部门；负责人合法上级/父负责人/CTO优先，禁止自己/无关/多候选随机；子树授权。
- [X] T007 [US3] 在 `server/handlers/evaluation_responsibility_resolver.go`、`evaluation_template.go` 实现上级评分优先规则和自己排除，在 `department_hierarchy_test.go` 测有效期/停用/歧义/任务快照，满足 FR-006。
- [X] T008 [US3] 在 `server/handlers/employee.go`、`department_hierarchy.go`、`server/routes/routes.go` 实现授权候选、跨部门上级及循环校验；在 `app/employees/page.tsx` 使用候选，满足 FR-005。
- [X] T009 [US3] 在 `server/handlers/permissions.go`、`evaluation_responsibility.go` 实现真实子树范围，在 `department_hierarchy_test.go` 验证允许/拒绝及预览范围，满足 FR-007。
## Phase 6: Quality
- [X] T010 在 `specs/009-department-hierarchy-assessment/acceptance.md` 记录 Go全套、unit、TS/lint/build、HTTP/页面证据及明确部署边界，更新 `quickstart.md` 并Converge，满足 FR-008。

## Dependencies
T001→T002→T003→T004，T005→T006→T007→T008→T009→T010。T003与T007关联解析代码最终一起测试。先创建错误MVP，再层级，再评分规则。
US1 tests/unit 与后端HTTP测试可独立验证；US2初始化/维护可各自验证；US3职责解析/范围测试独立。当前单代理顺序执行，不启动其它代理。
