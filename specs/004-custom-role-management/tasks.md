# Tasks: 用户角色与权限管理

**Input**: Design documents from `specs/004-custom-role-management/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/roles-api.md`, `quickstart.md`

**Tests**: 权限边界、数据范围、事务保护和 API 契约变更必须有自动化测试。

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 确认现有 RBAC、审计和前端组件可以被本 Feature 复用。

- [X] T001 核对并补齐 Git、Docker、Next.js 与 Go 产物忽略规则 `.gitignore`、`.dockerignore`
- [X] T002 核对角色 Feature 的现有 RBAC 种子、路由和 API 类型基线 `server/models/database.go`、`server/routes/routes.go`、`lib/api.ts`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 建立所有角色故事共同依赖的数据约束、授权上限和事务服务。

- [X] T003 在 `server/models/models.go` 为角色增加 `requires_manager`，并强化 `UserRole` 单用户唯一与 `RoleDataScope` 单角色唯一约束
- [X] T004 在 `server/models/database.go` 增加兼容 AutoMigrate、系统角色组织属性和历史多角色绑定幂等修复
- [X] T005 [P] 在 `server/handlers/permissions.go` 实现数据范围等级比较、角色授权上限校验和权限依赖补全辅助函数
- [X] T006 [P] 在 `server/handlers/role_test.go` 建立角色 API 测试数据库、种子和认证请求辅助函数
- [X] T007 在 `server/handlers/role.go` 建立角色请求/响应 DTO、稳定错误响应、事务与审计详情辅助函数

**Checkpoint**: 数据约束和授权基础可供全部用户故事使用。

---

## Phase 3: User Story 1 - 创建自定义角色 (Priority: P1) 🎯 MVP

**Goal**: 授权管理员可以创建具备单一数据范围、组织属性和权限集合的自定义角色。

**Independent Test**: 创建“绩效复核员”后，该角色出现在角色列表和员工角色选择器；无权限和越权创建均被拒绝且无部分数据。

- [X] T008 [P] [US1] 在 `server/handlers/role_test.go` 添加创建成功、重复名称、权限拒绝、越权权限和越权数据范围测试
- [X] T009 [US1] 在 `server/handlers/role.go` 实现角色列表、详情和权限目录分组所需的查询响应
- [X] T010 [US1] 在 `server/handlers/role.go` 实现自定义角色创建、服务端唯一代码、名称校验、权限依赖和原子写入
- [X] T011 [US1] 在 `server/routes/routes.go` 注册角色列表、详情和创建接口并强制 `role:view`、`role:create` 后端权限
- [X] T012 [P] [US1] 在 `lib/api.ts` 增加角色、权限、数据范围 DTO 及角色列表、详情、创建 API 客户端
- [X] T013 [US1] 在 `app/roles/page.tsx` 实现角色列表、新增表单、按资源分组权限矩阵、单一数据范围选择及提交反馈
- [X] T014 [US1] 在 `components/sidebar.tsx` 增加受 `role:view` 控制的“组织与绩效 / 用户角色”入口和 `/roles` 路由守卫

**Checkpoint**: 可独立创建并查看一个受授权上限保护的自定义角色。

---

## Phase 4: User Story 2 - 管理角色权限 (Priority: P1)

**Goal**: 管理员可以修改、复制和安全删除自定义角色，系统角色保持只读。

**Independent Test**: 自定义角色可原子更新和复制；系统角色编辑/删除以及已绑定角色删除均被拒绝并有审计记录。

- [X] T015 [P] [US2] 在 `server/handlers/role_test.go` 添加更新原子性、系统角色保护、复制和已绑定角色删除拒绝测试
- [X] T016 [US2] 在 `server/handlers/role.go` 实现自定义角色原子更新、系统/自定义角色复制和安全删除
- [X] T017 [US2] 在 `server/routes/routes.go` 注册角色更新、复制和删除接口并应用对应 `role:*` 权限
- [X] T018 [P] [US2] 在 `lib/api.ts` 增加角色更新、复制和删除客户端方法
- [X] T019 [US2] 在 `app/roles/page.tsx` 增加角色详情编辑、系统角色只读、复制确认、删除确认和成功/失败反馈

**Checkpoint**: 角色生命周期操作满足系统角色和在用角色保护要求。

---

## Phase 5: User Story 3 - 为用户分配角色 (Priority: P1)

**Goal**: 管理员可从角色页或员工页安全地为单个/多个用户原子更换主角色。

**Independent Test**: 批量分配后每个用户仅有一个角色；越权分配或改派最后一个超级管理员全部回滚。

- [X] T020 [P] [US3] 在 `server/handlers/role_test.go` 添加单角色替换、批量原子分配、越权分配和最后超级管理员保护测试
- [X] T021 [US3] 在 `server/handlers/permissions.go` 强化 `ReplaceUserRole` 的单角色事务语义与最后超级管理员保护辅助函数
- [X] T022 [US3] 在 `server/handlers/role.go` 实现角色绑定用户分页查询和批量用户分配接口，应用员工数据范围过滤
- [X] T023 [US3] 在 `server/handlers/employee.go` 与 `server/handlers/role.go` 统一单员工分配的授权上限、直属上级要求和兼容字段同步
- [X] T024 [US3] 在 `server/routes/routes.go` 注册角色用户查询和批量分配接口
- [X] T025 [P] [US3] 在 `lib/api.ts` 增加角色用户查询和批量分配客户端方法
- [X] T026 [US3] 在 `app/roles/page.tsx` 增加绑定用户查看、搜索和批量分配交互
- [X] T027 [US3] 在 `app/employees/page.tsx` 使用角色元数据展示动态名称并按 `requires_manager` 校验直属上级

**Checkpoint**: 自定义角色能安全应用到用户，且员工管理入口保持兼容。

---

## Phase 6: User Story 4 - 查看角色与用户关系 (Priority: P2)

**Goal**: 管理员能准确查看角色类型、数据范围、权限数、绑定人数和范围内用户明细。

**Independent Test**: 列表计数与实际绑定一致，SELF/DEPARTMENT 等范围调用者无法通过角色详情查看范围外员工。

- [X] T028 [P] [US4] 在 `server/handlers/role_test.go` 添加角色统计准确性和绑定用户数据范围隔离测试
- [X] T029 [US4] 在 `server/handlers/role.go` 完善角色统计、可分配标志和受范围过滤的用户摘要
- [X] T030 [US4] 在 `app/roles/page.tsx` 完善角色类型、范围、权限数、人数和影响用户展示

**Checkpoint**: 角色影响范围可审计且不会泄露范围外用户。

---

## Phase 7: 去除固定角色代码依赖与跨功能验证

**Purpose**: 确保自定义角色实际按权限、数据范围和组织关系参与现有业务。

- [X] T031 [P] 将 `lib/auth-context.tsx`、`lib/access-control.ts`、`components/employee-selector.tsx` 的流程判断改为权限、数据范围和角色元数据
- [X] T032 将 `server/handlers/auth.go`、`server/handlers/invitation.go`、`server/handlers/kpi.go`、`server/handlers/employee.go` 中影响自定义角色业务行为的固定角色分支替换为权限、数据范围或组织关系判断
- [X] T033 [P] 将 `app/evaluations/page.tsx`、`app/settings/page.tsx`、`app/performance-rules/page.tsx` 中影响业务操作的固定角色判断替换为权限能力判断
- [X] T034 在 `server/handlers/role_test.go` 添加自定义角色可执行业务权限与未授权操作拒绝的回归测试
- [X] T035 在 `specs/004-custom-role-management/quickstart.md` 补充实际页面路径、测试数据清理和角色变更后身份刷新步骤

---

## Phase 8: Polish & Quality Gates

**Purpose**: 格式化、静态检查、自动测试和 Docker 冒烟验证。

- [X] T036 运行 `gofmt` 并执行 `cd server && go test ./...`，修复角色 Feature 引入的测试失败
- [X] T037 运行 `npx tsc --noEmit` 与 `npm run build`，修复角色页面和 API 类型问题
- [X] T038 运行 `docker compose config`、`docker compose build backend frontend`、`docker compose up -d` 与 `/api/health` 冒烟检查且不删除数据卷
- [ ] T039 按 `specs/004-custom-role-management/quickstart.md` 完成允许/拒绝场景核对并记录实现状态

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → User Story 1。
- User Story 2 和 User Story 3 依赖 User Story 1 的角色 DTO 与 CRUD 基础。
- User Story 4 依赖角色统计和用户分配能力。
- 去硬编码阶段依赖角色元数据和授权服务完成。
- 质量门禁在全部目标故事完成后执行。

## Parallel Opportunities

- T005 与 T006 可并行；它们分别修改授权辅助和测试基线。
- 各用户故事中的后端测试与前端 API 类型任务可在其前置基础完成后并行。
- T031 与 T033 修改不同前端模块，可并行处理。

## Implementation Strategy

1. 先交付创建、查看角色的最小闭环（US1）。
2. 增加更新、复制、删除保护（US2）。
3. 增加用户分配与最后超级管理员保护（US3）。
4. 完成影响查看、去硬编码和完整质量门禁。

## Phase 9: Convergence

- [X] T040 实现仅在显式配置 `BOOTSTRAP_SUPER_ADMIN_EMAIL` 且系统没有有效超级管理员时才执行的一次性超级管理员初始化，并记录审计与启动日志 `server/models/database.go`、`docker-compose.yml`、`specs/004-custom-role-management/quickstart.md`（partial，FR-011）
