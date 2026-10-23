---
description: "Task list for 绩效计算引擎与模板维护"
---

# Tasks: 绩效计算引擎与模板维护

**Input**: Design documents from `/specs/006-performance-calculation-engine/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

## Phase 1: Setup

- [X] T001 建立绩效计算 Feature 的迁移和服务文件清单，保持现有 `KPITemplate`、`KPIEvaluation`、结果快照兼容
- [X] T002 [P] 在 `server/handlers/calculation_test.go` 建立附件公式对照测试夹具，覆盖维度权重、指标内权重、综合权重和等级边界
- [X] T003 [P] 在 `specs/006-performance-calculation-engine/quickstart.md` 固化本 Feature 的本地验证数据和验收命令

---

## Phase 2: Foundational

- [X] T004 在 `server/models/models.go` 扩展模板、维度、指标、快照和评分任务的数据模型，增加必要索引与唯一约束
- [X] T005 在 `server/models/database.go` 增加幂等迁移、默认计算版本和投入占比阈值配置，不删除既有数据
- [X] T006 [P] 在 `server/handlers/calculation.go` 实现权重校验、综合权重计算、评分范围校验和等级映射纯函数
- [X] T007 [P] 在 `server/handlers/calculation_test.go` 为 T006 增加空白评分、有效 0 分、权重不等于 100、S/A/B/C/D 边界测试
- [X] T008 在 `server/routes/routes.go` 注册模板结构、发布、停用和复制 API 路由并复用现有权限中间件；评分进度和汇总路由随对应用户故事接入

**Checkpoint**: 基础模型、计算函数、迁移和路由准备完成，后续用户故事可独立实现。

---

## Phase 3: User Story 1 - 维护岗位考核模板 (Priority: P1) 🎯 MVP

**Goal**: HR 能维护部门、岗位类型和周期对应的模板、维度、指标、权重和评分人配置。

**Independent Test**: 创建工程师模板，权重合计为 100% 时可发布；权重不合法或评分人配置缺失时发布被拒绝且无部分写入。

- [X] T009 [P] [US1] 在 `server/handlers/template.go` 和 `server/handlers/template_calculation.go` 实现模板草稿创建、查询、更新、复制和停用接口
- [X] T010 [US1] 在 `server/handlers/template_calculation.go` 实现维度和指标结构替换、排序以及综合权重计算
- [X] T011 [US1] 在 `server/handlers/template_calculation.go` 实现模板发布校验：部门/岗位/周期唯一、逐项舍入后的综合权重合计 100、必填指标和评分人类型完整（2026-10-01 A 决策增量见 feature 007 T106–T110）
- [X] T012 [US1] 在 `server/handlers/template_calculation_test.go` 增加模板权重校验、发布和事务无部分写入测试
- [X] T013 [P] [US1] 在 `lib/api.ts` 增加模板、维度、指标和发布接口的类型与客户端方法
- [X] T014 [US1] 在 `app/templates/page.tsx` 增加按部门/岗位/周期筛选、维度指标编辑、权重校验和发布反馈
- [X] T015 [US1] 在 `app/templates/page.tsx` 增加评分人类型、必填标记、评分标准和数据来源配置

**Checkpoint**: HR 可创建并发布至少一份负责人模板和一份工程师模板。

---

## Phase 4: User Story 2 - 按岗位创建考核 (Priority: P1)

**Goal**: 创建考核时按部门、岗位类型、周期自动匹配模板，并生成模板和评分人快照。

**Independent Test**: 两个岗位使用不同模板创建考核，考核详情展示正确指标、权重和评分任务，修改模板后旧考核不变。

- [X] T016 [P] [US2] 在 `server/models/models.go` 增加考核模板版本、维度/指标/评分人 JSON 快照字段或关联实体
- [X] T017 [US2] 在 `server/handlers/evaluation_template.go` 实现按部门、岗位类型、周期选择唯一有效模板的匹配服务
- [X] T018 [US2] 在 `server/handlers/evaluation_template.go` 实现创建考核时复制模板快照和计算版本的事务逻辑
- [X] T019 [US2] 在 `server/handlers/evaluation_template.go` 实现按评分人类型解析实际账号：直属负责人、部门负责人、PM、产品经理、CTO；无法解析时返回明确错误
- [X] T020 [US2] 在 `server/handlers/evaluation_template_test.go` 增加岗位模板匹配、快照隔离、评分人解析和失败回滚测试
- [X] T021 [P] [US2] 在 `server/routes/routes.go` 接入模板匹配预览和考核创建快照接口
- [ ] T022 [US2] 在 `app/evaluations/page.tsx` 展示自动匹配模板、计算版本和评分人预览，并允许授权管理员调整实际评分人

**Checkpoint**: 新建考核完全由岗位匹配模板，不依赖固定 RBAC 角色字符串。

---

## Phase 5: User Story 3 - 完成评分并计算总分 (Priority: P1)

**Goal**: 评分人完成所有必填指标后，系统按附件公式计算指标加权分、总分和等级，并允许进入员工确认。

**Independent Test**: 使用附件示例权重和分数得到一致总分；缺少任何必填指标时确认请求被拒绝。

- [ ] T023 [P] [US3] 在 `server/models/models.go` 增加可变评分人任务和逐评分人指标评分记录，兼容既有 `KPIScore` 数据
- [ ] T024 [US3] 在 `server/handlers/scoring.go` 实现评分任务查询、单项评分提交、评分修改权限和评分范围校验
- [ ] T025 [US3] 在 `server/handlers/scoring.go` 实现多人评分平均、指标加权分、总分和等级计算
- [ ] T026 [US3] 在 `server/handlers/scoring.go` 实现必填指标完成度检查，明确区分未评分、0 分和不适用
- [ ] T027 [US3] 在 `server/handlers/result_signoff.go` 接入确认前置校验，未完成必填评分时不得进入 `pending_confirm`
- [ ] T028 [US3] 在 `server/handlers/calculation_test.go` 增加附件 9 个部门权重样例、多人平均、空白评分和等级边界回归测试
- [ ] T029 [US3] 在 `server/handlers/scoring_test.go` 增加评分人越权、重复评分、事务回滚和确认前置校验测试
- [ ] T030 [P] [US3] 在 `lib/api.ts` 增加评分任务、评分进度、计算结果客户端方法
- [ ] T031 [US3] 在 `app/evaluations/page.tsx` 增加评分任务、完成进度、未完成指标提示和计算明细展示

**Checkpoint**: 所有必填指标完成后才能确认，最终总分和等级与附件公式一致。

---

## Phase 6: User Story 4 - HR 持续维护与历史冻结 (Priority: P1)

**Goal**: HR 可发布新模板版本，历史考核继续使用创建时的模板和计算快照。

**Independent Test**: 修改模板权重后，新考核使用新版本，旧考核重复计算、确认和导出结果不变。

- [ ] T032 [P] [US4] 在 `server/handlers/template.go` 增加发布版本、草稿版本和历史版本查询接口
- [ ] T033 [US4] 在 `server/handlers/evaluation_template.go` 增加旧考核快照完整性校验和兼容旧模板默认值
- [ ] T034 [US4] 在 `server/handlers/result_export.go` 确认 Excel、单份 PDF 和批量 PDF 使用考核快照中的字段和计算结果
- [ ] T035 [US4] 在 `server/handlers/template_test.go` 和 `server/handlers/result_export_test.go` 增加模板修改后历史结果不变测试
- [ ] T036 [P] [US4] 在 `app/performance-rules/page.tsx` 增加版本列表、发布说明、停用和复制模板交互

**Checkpoint**: HR 可持续维护模板，历史考核不被新模板破坏。

---

## Phase 7: User Story 5 - 月度、季度和年度汇总 (Priority: P2)

**Goal**: HR 可按附件规则查看项目投入占比、加权贡献、周期总分和年度等级。

**Independent Test**: 投入占比 10% 的项目贡献为 0，投入占比 50% 的项目按总分乘 50% 计算，年度分数和等级同时返回。

- [ ] T037 [P] [US5] 在 `server/models/models.go` 增加项目投入占比、加权贡献和汇总计算版本字段
- [ ] T038 [US5] 在 `server/handlers/statistics.go` 实现月度、季度、年度分数、等级和周期权重汇总
- [ ] T039 [US5] 在 `server/handlers/statistics.go` 实现最低有效投入占比阈值配置，默认 20%，低于阈值贡献为 0
- [ ] T040 [US5] 在 `server/handlers/statistics_test.go` 增加一人多项目、阈值边界、周期缺失和年度加权平均测试
- [ ] T041 [P] [US5] 在 `lib/api.ts` 增加 HR 汇总查询接口和类型
- [ ] T042 [US5] 在 `app/statistics/page.tsx` 或现有 HR 汇总页面增加项目投入、加权贡献、年度分数和等级展示

**Checkpoint**: HR 汇总结果与附件月度/季度表规则一致。

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T043 [P] 在 `server/handlers/calculation_test.go` 增加从附件抽取的 9 个部门模板对照数据测试
- [ ] T044 [P] 在 `server/handlers/permissions_test.go` 增加模板维护、评分和汇总接口的数据范围与权限测试
- [ ] T045 在 `server/handlers/audit.go` 或现有审计入口补充模板发布、评分提交、计算、确认和汇总审计事件
- [ ] T046 在 `server/models/database.go` 和迁移逻辑中验证旧模板、旧考核、旧快照兼容，不修改既有数据卷
- [ ] T047 运行 `gofmt`、`go test ./...`、`npx tsc --noEmit` 和 `npm run build`，修复本 Feature 引入的问题
- [ ] T048 运行 `docker compose config`、后端/前端镜像构建、服务启动和 `/health` 冒烟测试
- [ ] T049 按 `specs/006-performance-calculation-engine/quickstart.md` 完成手工验收并记录结果

## Dependencies & Execution Order

- Phase 1 → Phase 2 → US1。
- US1 完成后才能进行 US2；US2 完成后才能进行 US3。
- US4 依赖 US1–US3 的版本和快照能力。
- US5 依赖 US3 的总分结果，但可与 US4 的前端版本管理并行。
- Phase 8 在所有目标用户故事完成后执行。

## Parallel Opportunities

- T002/T003、T006/T007、T013、T016、T023、T030、T037、T041、T043/T044 可在依赖满足后并行。
- 前端模板页面与后端模板接口在接口契约确定后可并行。
- 计算纯函数测试可先于数据库和页面实现执行。

## Implementation Strategy

1. 先完成模板结构与权重校验，交付最小可用模板维护闭环。
2. 增加岗位匹配和考核快照，确保历史规则稳定。
3. 增加评分任务、总分计算和确认前置校验，形成完整 MVP。
4. 增加 HR 汇总和导出联动。
5. 最后执行兼容迁移、自动化测试和 Docker 验收。
