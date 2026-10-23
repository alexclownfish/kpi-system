# Tasks: 可维护部门层级与负责人

## Phase 1: Setup
- [X] T001 在 `specs/010-maintainable-department-leaders/` 完成specify/clarify/plan设计；明确无RBAC猜测、无普通环境写入，映射FR-001至007。
## Phase 2: Foundation
- [X] T002 在 `server/handlers/department_leader_test.go` 和 `tests/unit/department-hierarchy-form.test.mjs` 写复现：无职责父负责人、跨部门拒绝、候选循环排除，先验证失败。
## Phase 3: US1 组织维护 MVP
**Independent Test**: 部门负责人设置、省略保持/null清空、循环父拒绝、员工范围拒绝。
- [X] T003 [US1] 在 `server/models/models.go`、`server/handlers/department.go` 新增nullable indexed leader_id；只允许本部门在职且可见员工，省略保持/null清空，审计负责人变更；FR-001/002/006。
- [X] T004 [US1] 在 `app/departments/page.tsx`、`lib/api.ts` 增加负责人选择和上级/负责人独立列、失效选项、后代排除及保存反馈；FR-001/003。
## Phase 4: US2 上级与评分
**Independent Test**: 无重复职责时父负责人可选、可保存、评分相同，自定义岗位身份、停用/调岗阻塞、历史不动。
- [X] T005 [US2] 在 `server/handlers/department_hierarchy.go` 统一明确负责人、负责人身份、候选/保存及祖先解析；在 `evaluation_responsibility_resolver.go`、`evaluation_template.go` 工程师优先本部门负责人，兼容未配置旧职责；FR-004/005/006。
- [X] T006 [US2] 在 `server/handlers/department_leader_test.go` 验证自定义部门/岗位、职责优先级、失效/循环/自己、预览创建任务和历史保留；FR-004/005/006。
## Phase 5: Quality
- [X] T007 在 `specs/010-maintainable-department-leaders/acceptance.md` 记录Go全套/unit/TS/lint/build、隔离浏览器完整组织配置与上级选择证据；核对quickstart并Converge，停止临时服务；FR-007、SC-001至003。

## Dependencies & Implementation Strategy
T001→T002→T003→T004→T005→T006→T007。US1组织维护是首个MVP；US2复用相同负责人定义。单代理按依赖执行；后端与前端测试可独立运行，不启动研究/开发代理。逐项验证标记，不批量假定完成。
