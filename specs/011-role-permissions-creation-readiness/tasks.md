# Tasks: 独立角色权限维护与考核创建准备

**Input**: spec.md、plan.md、research.md、data-model.md、contracts/。

## Phase 1: Setup

- [x] T001 完成规格、澄清扫描和设计，检查忽略文件及运行入口，在 specs/011-role-permissions-creation-readiness/ 留存方案。

## Phase 2: Foundational

- [x] T002 编写回归测试 server/handlers/role_edit_test.go 与 server/models/database_test.go，证明系统角色不可编辑、空权限回退及重启恢复缺陷。

## Phase 3: User Story 1 - 独立维护角色

**Independent test**: 单角色修改不影响其他角色或用户绑定；越权拒绝且重启保留。

- [x] T003 [US1] 在 server/models/models.go 和 server/models/database.go 加 default:false 内部定制标记，定制后跳过种子授权及范围回填。
- [x] T004 [US1] 在 server/handlers/role.go 校验现有目标授权和提交授权，保护超级管理员及系统角色稳定属性，事务保存与审计；补齐 DEPARTMENT_TREE 目录。
- [x] T005 [US1] 在 server/handlers/permissions.go 仅在无 UserRole 绑定时使用旧角色回退，禁止空授权和撤销授权被旧数据重新授予。
- [x] T006 [US1] 在 app/roles/page.tsx 开放非超级管理员角色编辑、锁定稳定属性、复用错误提取；增加 tests/unit/role-editor.test.mjs。

## Phase 4: User Story 2 - 创建准备

**Independent test**: audit 阻塞、缺职责阻塞、补齐后创建快照任务一致。

- [x] T007 [US2] 在 server/handlers/department_hierarchy_test.go 增加预览流程模式和阻塞时无记录断言；server/handlers/evaluation_template.go 与 lib/api.ts 提供 workflow_mode。
- [x] T008 [US2] 在 app/evaluations/page.tsx 显示流程状态、权限感知职责维护入口，不取消原创建阻塞。

## Phase 5: Polish

- [x] T009 运行全部 Go、前端单元测试、tsc、目标 lint 与 Next build，在 specs/011-role-permissions-creation-readiness/acceptance.md 记录证据。
- [x] T010 在隔离克隆验证角色编辑保存及创建预览，桌面/移动截图；停止临时服务，完善 specs/011-role-permissions-creation-readiness/quickstart.md。

## Dependencies & Execution Order

T001 -> T002 -> T003 -> T004 -> T005 -> T006；T007 -> T008；全部完成后 T009 -> T010。
US1、US2独立；测试可分文件运行，但本次顺序实现，不创建代理任务。

## Implementation Strategy

先修复角色持久化和授权边界为 MVP，再改创建准备反馈；环境/真实职责授权未确认不写入正常服务。
