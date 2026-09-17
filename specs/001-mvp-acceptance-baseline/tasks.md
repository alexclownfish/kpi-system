---

description: "Implementation tasks for the KPI MVP acceptance and regression baseline"
---

# Tasks: MVP 验收与回归基线

**Input**: Design documents from `specs/001-mvp-acceptance-baseline/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: 本 Feature 的交付物就是可重复验收，因此测试任务为必需项，并按测试先行顺序排列。

**Organization**: 任务按用户故事组织；每个故事都具有独立执行和判定标准。

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 建立 Playwright 验收工具链和仓库级运行入口。

- [X] T001 在 `package.json` 和 `package-lock.json` 增加 `@playwright/test` 与 `test:e2e` 脚本并锁定依赖
- [X] T002 在 `playwright.config.ts` 配置 `KPI_BASE_URL`、单 worker、失败截图/trace、基础 HTML/JSON reporter 及全局超时
- [X] T003 [P] 在 `.gitignore` 忽略 `playwright-report/`、`test-results/` 和本地验收结果文件
- [X] T004 [P] 在 `tests/e2e/README.md` 记录环境变量、秘密处理、测试数据命名和禁止 `docker compose down -v` 的规则

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 提供所有用户故事共享的环境校验、身份、结果模型、API 调用和清理能力。

**⚠️ CRITICAL**: 此阶段完成前不得开始用户故事场景。

- [X] T005 在 `tests/e2e/helpers/acceptance-context.ts` 实现 `AcceptanceRun`、`AcceptanceScenarioResult`、`TestIdentity`、`TemporaryRecord` 类型和 runId 生成，严格使用 `data-model.md` 的枚举与字段约束
- [X] T006 [P] 在 `tests/e2e/helpers/redaction.ts` 实现错误与日志脱敏，过滤密码、Authorization、Cookie、完整 JWT 和密码哈希
- [X] T007 [P] 在 `tests/e2e/helpers/api-client.ts` 实现健康检查、登录、授权请求和结构化错误解析，拒绝把网络错误或 5xx 当作权限拒绝
- [X] T008 在 `tests/e2e/helpers/cleanup.ts` 实现临时记录登记、依赖反向排序、按精确 ID 删除、删除后复查及清理失败汇总，并生成可供运行脚本映射退出码的非敏感清理状态文件
- [X] T009 在 `tests/e2e/fixtures/acceptance.ts` 校验 `KPI_BASE_URL`、`KPI_TEST_HR_EMAIL`、`KPI_TEST_HR_PASSWORD`，在创建数据前完成健康检查和 HR 权限前置检查
- [X] T010 在 `scripts/acceptance/run-mvp.sh` 实现非破坏性运行入口：检查 Compose 服务、等待最多 60 秒健康状态、执行 Playwright、保留数据卷并透传退出码

**Checkpoint**: 运行器可在不创建员工的情况下验证环境并安全退出。

---

## Phase 3: User Story 1 - 验证员工创建与初始登录 (Priority: P1) 🎯

**Goal**: 验证 HR 创建普通员工和部门负责人、初始密码登录、用户反馈及拒绝场景。

**Independent Test**: 只运行标记为 `US1` 的场景，创建两个唯一临时员工、分别登录、验证角色与
数据范围，并在结束时确认两个 ID 均不存在。

### Tests for User Story 1

- [X] T011 [P] [US1] 在 `server/handlers/permissions_test.go` 先增加创建员工缺少密码、普通员工缺少直属上级、直属上级已停用、直属上级不在指定部门和重复邮箱的回归测试，并断言失败时没有新增记录
- [X] T012 [P] [US1] 在 `tests/e2e/mvp-acceptance.spec.ts` 先编写 HR 页面创建负责人和普通员工、提交中状态、成功提示及初始密码登录场景
- [X] T013 [US1] 在 `tests/e2e/mvp-acceptance.spec.ts` 先编写缺少密码、缺少直属上级、直属上级无效和重复邮箱的明确错误提示场景

### Implementation for User Story 1

- [X] T014 [US1] 在 `server/handlers/employee.go` 将重复邮箱等可识别的创建冲突映射为明确 4xx 错误，并保持密码和哈希不出现在响应中
- [X] T015 [US1] 在 `app/employees/page.tsx` 和必要的 `components/alert.tsx` 中补足 Playwright 可访问定位所需的稳定语义，同时保持提交中、成功和后端失败提示
- [X] T016 [US1] 在 `tests/e2e/mvp-acceptance.spec.ts` 接入唯一邮箱、临时负责人依赖、登录响应角色/Data Scope 断言和精确 ID 清理

**Checkpoint**: US1 可独立重复执行三次，员工创建、登录和清理均通过。

---

## Phase 4: User Story 2 - 验证权限与数据隔离 (Priority: P1) 🎯

**Goal**: 验证员工、部门负责人和 HR 的允许/拒绝边界以及 SELF/DEPARTMENT 数据范围。

**Independent Test**: 只运行标记为 `US2` 的场景，普通员工越权请求得到预期 403 且无敏感数据，
员工查询只返回本人，负责人查询只返回所属部门，HR 无法修改或删除超级管理员。

### Tests for User Story 2

- [X] T017 [P] [US2] 在 `server/handlers/permissions_test.go` 增加普通员工创建员工、访问公司统计被拒绝及 HR 保护超级管理员的路由回归测试
- [X] T018 [US2] 在 `server/handlers/permissions_test.go` 增加 SELF 与 DEPARTMENT 查询结果集合断言，确保结果中不存在范围外员工
- [X] T019 [P] [US2] 在 `tests/e2e/mvp-acceptance.spec.ts` 实现 `contracts/api-scenarios.md` 的员工越权、公司统计拒绝和响应不泄露数据断言
- [X] T020 [US2] 在 `tests/e2e/mvp-acceptance.spec.ts` 实现普通员工 SELF、负责人 DEPARTMENT 数据范围及登录返回 Permission/Data Scope 的断言
- [X] T021 [US2] 在 `tests/e2e/mvp-acceptance.spec.ts` 增加普通员工直接访问 `/employees`、`/statistics` 的页面跳转或拒绝验证

**Checkpoint**: US2 的所有拒绝场景命中预期 4xx，所有允许查询都通过集合边界检查。

---

## Phase 5: User Story 3 - 一键确认部署可用 (Priority: P2)

**Goal**: 使用一个命令验证后端、前端、网关、认证和 P1 场景，并可靠返回退出码。

**Independent Test**: 在服务正常和故意停止一个服务两种状态下运行入口，分别得到成功和可定位的
前置条件失败；两次运行都不删除已有卷。

### Tests for User Story 3

- [X] T022 [P] [US3] 在 `tests/e2e/deployment-readiness.spec.ts` 增加 `/health`、登录页面、HR 认证和统一入口前置条件测试
- [X] T023 [US3] 在 `tests/e2e/deployment-readiness.spec.ts` 增加非预期 5xx、连接失败和部分服务不可用不得误报成功的测试

### Implementation for User Story 3

- [X] T024 [US3] 在 `scripts/acceptance/run-mvp.sh` 完成环境失败退出码 2、功能失败退出码 1、清理失败退出码 3 的映射，并禁止任何带 `-v` 的 Compose 停止操作
- [X] T025 [US3] 在 `package.json` 将 `acceptance:mvp` 连接到非破坏性启动脚本，并允许将标准 Playwright 参数透传给运行器
- [X] T026 [US3] 按 `specs/001-mvp-acceptance-baseline/quickstart.md` 在保留现有卷的环境执行一次完整验收并补正文档中发现的不一致

**Checkpoint**: 一个命令可以判定部署是否可用，并以契约定义的退出码结束。

---

## Phase 6: User Story 4 - 快速定位回归原因 (Priority: P3)

**Goal**: 失败结果可以定位用户故事、执行阶段和实际响应，同时不泄露认证秘密。

**Independent Test**: 使用错误 HR 密码和故意失败断言运行场景，报告包含场景 ID、预期/实际摘要
和证据路径，但搜索报告找不到密码、Authorization、Cookie 或完整 JWT。

### Tests for User Story 4

- [X] T027 [P] [US4] 在 `tests/e2e/redaction.spec.ts` 增加密码、Authorization、Cookie、JWT 和 bcrypt 哈希不会出现在序列化结果中的测试
- [X] T028 [P] [US4] 在 `tests/e2e/reporting.spec.ts` 增加失败结果包含场景 ID、执行阶段、预期/实际和证据路径的契约测试

### Implementation for User Story 4

- [X] T029 [US4] 在 `tests/e2e/helpers/acceptance-reporter.ts` 实现 `contracts/acceptance-runner.md` 的人类可读阶段输出和结构化结果，复用 `redaction.ts`
- [X] T030 [US4] 在 `playwright.config.ts` 注册 T029 的结构化验收 reporter 和失败证据目录，并确保成功运行不会保留包含认证上下文的 trace

**Checkpoint**: 失败可以从报告直接定位，报告秘密扫描测试通过。

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 文档同步、全量质量门禁和重复执行验证。

- [X] T031 [P] 更新 `README.md` 的七角色、Permission/Data Scope、Docker 地址和 `npm run acceptance:mvp` 使用说明
- [X] T032 [P] 更新 `docs/baseline/mvp-v1-status.md`，记录验收入口、报告位置和已关闭的自动化测试缺口
- [ ] T033 运行 `go test ./...`、`tsc --noEmit`、Next.js production build 和 Playwright 全量验收；安排一名未参与实现的开发者从快速说明开始计时验证，并在 `specs/001-mvp-acceptance-baseline/quickstart.md` 记录命令、结果、耗时和验证人
- [X] T034 连续运行 `npm run acceptance:mvp` 三次，核对临时记录遗留为 0、种子账号不变、Compose 命名卷未删除，并将证据摘要写入 `specs/001-mvp-acceptance-baseline/quickstart.md`
- [X] T035 运行 `$speckit-converge` 对照 FR-001 至 FR-014、SC-001 至 SC-007 和 Constitution 检查剩余缺口

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 Setup**：无依赖。
- **Phase 2 Foundational**：依赖 Phase 1，阻塞所有用户故事。
- **US1 和 US2**：依赖 Phase 2；两者均为 MVP P1，可以并行，但共享
  `tests/e2e/mvp-acceptance.spec.ts` 时需协调修改顺序。
- **US3**：依赖 US1 和 US2 已能通过，因为一键入口需要运行完整 P1 验收。
- **US4**：依赖 Phase 2；可与 US1/US2 测试场景并行开发，最终 reporter 集成在其后完成。
- **Polish**：依赖所有目标用户故事完成。

### User Story Dependency Graph

```text
Setup → Foundational ┬→ US1 ─┐
                    ├→ US2 ─┼→ US3 → Polish
                    └→ US4 ─┘
```

### Within Each User Story

- 先提交能复现缺口的测试并确认失败。
- 再实现最小代码使对应故事通过。
- 每个故事结束时单独运行其标记测试和清理检查。
- 不得在 US1/US2 中顺便扩展自定义角色或真实部门树能力。

## Parallel Opportunities

- T003 与 T004 可并行。
- T006 与 T007 可并行；T008 依赖 T005 的类型定义。
- US1 的后端回归测试 T011 与浏览器测试 T012/T013 可并行。
- US2 的 Go 测试 T017/T018 与端到端测试 T019/T020 可并行。
- US3 的健康与失败测试 T022/T023 修改同一文件，按顺序执行。
- US4 的脱敏与报告契约测试 T027/T028 可并行。
- README 和基线文档 T031/T032 可并行。

## Parallel Example: P1 Stories

```text
Task A: T011 + T014，负责员工创建错误契约和 Go 回归测试
Task B: T012 + T013 + T015 + T016，负责员工创建页面与 Playwright US1
Task C: T017 + T018，负责 Go 权限和数据范围回归测试
Task D: T019 + T020 + T021，负责 Playwright API/UI 权限验收
```

## Implementation Strategy

### MVP First

本 Feature 的最小可交付范围是：

1. Phase 1 Setup。
2. Phase 2 Foundational。
3. US1 员工创建与登录。
4. US2 权限与数据隔离。
5. 对 US1/US2 做一次独立验证并清理临时数据。

### Incremental Delivery

1. 先形成可运行但只覆盖 P1 的验收入口。
2. 加入部署就绪和退出码，完成 US3。
3. 加入结构化 reporter 和秘密扫描，完成 US4。
4. 最后同步 README、连续运行三次并 Converge。

## Notes

- `[P]` 只表示不同文件或无未完成依赖，不代表可以无协调修改同一测试文件。
- 所有临时记录必须在创建成功后立即登记精确 ID。
- 不得使用固定邮箱、批量删除、清空数据库或删除 Compose 卷来解决重复运行问题。
- 每个逻辑任务组完成后提交本地 Git，保持规格、测试和实现可追溯。

## Phase 8: Convergence

- [X] T036 在 `scripts/acceptance/run-mvp.sh` 和 `tests/e2e/redaction.spec.ts` 扫描真实结构化报告与失败证据，确保密码、Authorization、Cookie、完整 JWT 和 bcrypt 哈希不会写入产物，并对包含认证上下文的场景禁用 trace，依据 FR-012、US4/AC2 (partial)
- [X] T037 在 `tests/e2e/mvp-acceptance.spec.ts` 增加重复邮箱和缺少直属上级的真实页面失败弹窗断言，确认用户获得可操作反馈且列表不新增记录，依据 US1/AC3、Constitution III (partial)
- [ ] T038 安排一名未参与实现的开发者从 `quickstart.md` 开始计时执行人工快速验证，并记录验证人、耗时和结果，依据 SC-007 (missing)
