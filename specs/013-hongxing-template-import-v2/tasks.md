---
description: "Task list for 荭杏研发绩效模板导入与流程适配 v2"
---

# Tasks: 荭杏研发绩效模板导入与流程适配 v2

**Input**: Design documents from `/specs/013-hongxing-template-import-v2/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: 本 Feature 明确要求源表解析、权限/数据范围、权重计算、状态门禁、历史快照和真实页面验收；测试任务必须先于对应实现任务，并在实现前确认能够复现缺陷。

## Phase 1: Setup and source fixtures

- [X] T001 固化工作簿 SHA、21 张工作表清单、18 套岗位模板、137 指标和 9 源部门快照到 `scripts/testdata/hongxing-workbook-v2-manifest.json`，并在 `scripts/test_source_workbook.py` 验证哈希和清单。
- [X] T002 [P] 在 `scripts/test_source_workbook.py` 增加流程表位于首张、岗位表顺序变化、18 模板识别和 2 HR 汇总识别回归；先运行确认旧 `scripts/verify_source_workbook.py` 在新表上失败。
- [X] T003 [P] 在 `server/handlers/source_workbook_test.go` 建立来源 SHA、重复版本、诊断严重级别和原子回滚测试夹具。
- [X] T004 [P] 在 `tests/unit/hongxing-template-import.test.mjs` 建立页面导入预览、诊断、发布门禁、重试和空状态测试夹具。
- [X] T005 记录上述失败证据到 `specs/013-hongxing-template-import-v2/acceptance.md`，不启动 Docker、不修改业务数据。

## Phase 2: Foundational parsing and formula expansion

**Checkpoint**: 解析器能够对源工作簿生成稳定、可审计的中间结果后，才开始导入和页面实现。

- [X] T006 [US1] 在 `scripts/verify_source_workbook.py` 改为按内容识别 `process`、`position_template`、`hr_monthly`、`hr_quarterly`，移除前 9 张表假设。
- [X] T007 [US1] 在 `scripts/verify_source_workbook.py` 实现岗位、部门、层级和周期识别；对算法工程师空部门执行显式同组映射并保留 raw 值与映射原因。
- [X] T008 [P] [US1] 在 `scripts/verify_source_workbook.py` 等价模块读取 worksheet XML 的 shared formula master/child、`si`、`ref`，展开 A1 相对引用并保留原始 XML 元数据。
- [X] T009 [US1] 为 `scripts/test_source_workbook.py` 增加 shared formula 主/子节点、数组公式、普通公式和真实数据行公式断档测试，区分空白模板行 warning 与真实数据行 blocking。
- [X] T010 [P] [US1] 在 `scripts/verify_source_workbook.py` 实现 Decimal HALF_UP 综合权重、加权得分、总分和 S/A/B/C/D 等级计算，禁止使用浮点银行家舍入。
- [X] T011 [US1] 在 `scripts/verify_source_workbook.py` 输出 18 套模板的指标、评分人、来源单元格、维度异常、公式诊断和 HR 汇总诊断，不写入任何评分或演示员工。
- [X] T012 [US1] 在 `scripts/build_hongxing_catalog.py` 生成版本化 catalog v2 JSON，包含源 SHA、raw 值、来源行、职责类型和诊断摘要；后端 embed 接入留待后续任务。
- [X] T013 [US1] 运行源表解析回归并将计数、异常和公式统计写入 `specs/013-hongxing-template-import-v2/acceptance.md`；重复运行输出必须稳定。

## Phase 3: Source and diagnostic data model

- [X] T014 [P] 在 `server/models/models.go` 增加 SourceWorkbook、SourceSheet、DepartmentMapping、ImportDiagnostic、FormulaCell 和 HRAggregationRule 兼容模型、索引和唯一约束。
- [X] T015 [P] 在 `server/models/models.go` 扩展模板来源字段和诊断阻断状态；既有评分职责、会签、计算版本和导出布局字段保持兼容。
- [X] T016 [US1] 在 `server/models/database.go` 增加幂等迁移，失败回滚，且不删除旧模板、考核、签字附件或用户；SourceWorkbook 以 SHA 唯一约束防重复版本。
- [X] T017 [US1] 在 `server/handlers/source_workbook_test.go` 验证新迁移与旧数据库兼容、重复来源 SHA 拒绝、诊断读取和敏感元数据不返回。
- [X] T018 [US1] 在 `server/handlers/source_workbook.go` 实现只读解析服务、稳定错误码、request_id、诊断排序和敏感字段过滤。
- [X] T019 [US1] 在 `server/routes/routes.go` 注册 `/api/source-workbooks/validate`、诊断、HR诊断和导入路由，复用现有权限中间件。

## Phase 4: User Story 1 — 校验并导入新版 Excel (Priority: P1)

**Goal**: HR 能看到完整事实、公式和异常，并在失败时得到原子结果。

**Independent Test**: 给定 SHA 的工作簿得到 21/18/137/9 计数和诊断；损坏/歧义/阻断输入不写业务数据。

### Tests for User Story 1

- [X] T020 [P] [US1] 在 `server/handlers/source_workbook_test.go` 覆盖允许的 HR 预览、无权限拒绝、范围外模板拒绝和敏感字段不返回。
- [X] T021 [P] [US1] 在 `tests/unit/hongxing-template-import.test.mjs` 覆盖加载中禁用发布、导入失败清空旧数据、重试成功恢复和诊断列表展示。

### Implementation for User Story 1

- [X] T022 [US1] 在 `server/handlers/source_workbook.go` 实现 `POST /api/source-workbooks/validate`、`GET /:id/diagnostics` 和 `GET /:id/hr-diagnostics`。
- [X] T023 [US1] 在 `server/handlers/source_workbook.go` 实现 draft/import 原子事务、来源 SHA 校验、显式部门映射和阻断级别处理。
- [X] T024 [P] [US1] 在 `lib/api.ts` 增加源工作簿验证、导入、诊断、HR 诊断的类型和客户端方法，统一错误结构。
- [X] T025 [US1] 在 `app/templates/page.tsx` 增加导入预览、来源版本、岗位/部门/指标计数、shared formula 和诊断状态，所有写操作显示 loading/成功/失败。
- [X] T026 [US1] 在 `app/templates/page.tsx` 对 blocking 诊断禁用发布，对 warning 允许保存 draft；任意请求失败时清空残缺结果并提供刷新重试。

## Phase 5: User Story 2 — 模板维护和发布 (Priority: P1)

**Goal**: HR 可以修正异常并发布新版本，历史版本不可破坏性覆盖。

**Independent Test**: 修正结构/电子/软件异常后发布；未修正时发布被拒绝；旧版本和历史考核保持不变。

### Tests for User Story 2

- [X] T027 [P] [US2] 在 `server/handlers/template_import_v2_test.go` 覆盖综合权重 100 发布、维度内 73/120/140 异常仅告警、缺评分职责阻止、重复匹配键阻止和事务无半成品；发布口径由 T066/T069 纠正为已确认 A 方案。
- [X] T028 [P] [US2] 在 `tests/unit/template-publish-v2.test.mjs` 覆盖来源单元格展示、复制新版本、停用后历史读取和发布反馈。

### Implementation for User Story 2

- [X] T029 [US2] 在 `server/handlers/template.go` 接入 source hash、diagnostic status、scorer rules、signoff rules 和 export layout 的草稿/发布/停用/复制。
- [X] T030 [US2] 在 `server/handlers/template_calculation.go` 实现 HALF_UP 综合权重校验和发布错误详情；保留原始维度内权重，不做归一化。
- [X] T031 [US2] 在 `server/handlers/template.go` 增加审计事件 `source_workbook_import`、`template_publish`、`template_retire`，不记录敏感内容。
- [X] T032 [P] [US2] 在 `lib/api.ts` 更新模板版本、诊断和发布 API 类型；在 `app/templates/page.tsx` 实现 HR 修正、复制、发布和历史版本显示。
- [X] T033 [US2] 在 `server/handlers/template_import_v2_test.go` 验证已被历史考核引用的模板不能破坏性更新，且新版本匹配键唯一。

## Phase 6: User Story 3 — 岗位自动匹配和职责快照 (Priority: P1)

**Goal**: 新建考核按部门/岗位/周期命中模板，解析 PM、部门负责人、CTO、PD 实际员工并生成评分与会签快照。

**Independent Test**: 负责人、工程师、产品岗位各创建一份考核；缺失职责时原子拒绝；RBAC 普通员工仍能作为业务评分人。

### Tests for User Story 3

- [X] T034 [P] [US3] 在 `server/handlers/evaluation_template_v2_test.go` 覆盖唯一匹配、无匹配、多匹配、部门层级、空岗位和周期边界。
- [X] T035 [P] [US3] 在 `server/handlers/evaluation_responsibility_v2_test.go` 覆盖 PM/部门负责人/CTO/PD 解析、停用用户、普通员工 RBAC、数据范围和缺失职责错误。
- [X] T036 [P] [US3] 在 `server/handlers/evaluation_snapshot_v2_test.go` 覆盖 source SHA、评分职责、会签、导出布局、计算版本和 checksum 的只读快照。
- [X] T037 [P] [US3] 在 `tests/unit/evaluation-template-match-v2.test.mjs` 覆盖模板预览、评分人列表完整加载和失败反馈。

### Implementation for User Story 3

- [X] T038 [US3] 在 `server/handlers/evaluation_template.go` 实现按部门/岗位/周期的唯一已发布模板匹配和预览接口。
- [X] T039 [US3] 在 `server/handlers/evaluation_responsibility_resolver.go` 按业务职责解析真实员工，保持后端范围校验，不使用 RBAC 角色名称筛选。
- [X] T040 [US3] 在 `server/handlers/evaluation_template.go` 将模板、指标、评分任务、会签、source hash 和计算版本写入创建事务；缺失职责整体回滚。
- [X] T041 [US3] 在 `server/routes/routes.go` 和 `lib/api.ts` 接入预览、职责候选和快照字段；在 `app/evaluations/page.tsx` 展示模板、职责和创建错误。
- [X] T042 [US3] 在 `server/handlers/evaluation_snapshot_v2_test.go` 验证修改部门、岗位、角色、职责配置或模板后旧快照不变。

## Phase 7: User Story 4 — 评分、计算和必填门禁 (Priority: P1)

**Goal**: 评分人只填写职责范围内指标；全部必填项完成后才可进入员工确认。

**Independent Test**: 漏一项必填指标确认失败；有效 0 分通过；完整分数与 Excel 公式一致；多人平均可审计。

### Tests for User Story 4

- [X] T043 [P] [US4] 在 `server/handlers/scoring_v2_test.go` 覆盖空白、0 分、N/A 理由、越权、重复提交、版本冲突、范围边界和多评分人平均。
- [X] T044 [P] [US4] 在 `server/handlers/calculation_v2_test.go` 覆盖 18 套模板代表样例、逐项 HALF_UP、总分、S/A/B/C/D 边界和综合权重异常阻断；维度内合计不单独阻断。
- [X] T045 [P] [US4] 在 `server/handlers/evaluation_workflow_v2_test.go` 覆盖必填门禁、评分顺序、确认失败不改状态、完整评分进入 pending_confirm 和审计事件。
- [X] T046 [P] [US4] 在 `tests/unit/scoring-gate-v2.test.mjs` 覆盖进度、未完成清单、0 分显示、N/A 提示和提交状态反馈。

### Implementation for User Story 4

- [X] T047 [US4] 在 `server/handlers/scoring.go` 接入快照指标和 item-user 任务，后端校验任务归属、职责、范围、版本和评分区间。
- [X] T048 [US4] 在 `server/handlers/calculation.go` 统一 HALF_UP 计算、有效评分平均、未评分/0/N/A 状态和等级映射，拒绝客户端总分。
- [X] T049 [US4] 在 `server/handlers/evaluation_workflow.go` 接入必填门禁，返回未完成指标、责任人和稳定错误码 `REQUIRED_SCORES_INCOMPLETE`。
- [X] T050 [P] [US4] 在 `lib/api.ts` 和 `app/evaluations/page.tsx` 展示评分职责、完成进度、计算明细、错误 request_id 和重试入口。
- [X] T051 [US4] 增加 `score_submitted`、`evaluation_calculated`、`confirmation_gate_rejected` 审计并验证无敏感字段。

## Phase 8: User Story 5 — HR 汇总诊断与历史导出 (Priority: P2)

**Goal**: 月度/季度/年度汇总可解释、可诊断，历史导出稳定。

**Independent Test**: 定位季度公式断档；10%/50% 投入占比得到 0/按比例贡献；模板更新后旧导出校验码不变。

### Tests for User Story 5

- [X] T052 [P] [US5] 在 `server/handlers/statistics_v2_test.go` 覆盖月度/季度/年度分数等级、0.20 阈值、周期缺失、公式断档和计算版本。
- [X] T053 [P] [US5] 在 `server/handlers/result_export_v2_test.go` 覆盖模板快照字段、来源哈希、纵向 PDF、Excel 和批量签字 PDF 的字段一致性。
- [X] T054 [P] [US5] 在 `tests/unit/hr-diagnostics-v2.test.mjs` 覆盖诊断列表、阻断发布、部分汇总失败和重试反馈。

### Implementation for User Story 5

- [X] T055 [US5] 在 `server/handlers/statistics.go` 增加 HR 汇总规则诊断、投入占比阈值、周期总分/等级/加权贡献和版本保存。
- [X] T056 [US5] 在 `server/handlers/result_export.go`、`server/handlers/batch_results.go`、`server/handlers/export.go` 统一读取 EvaluationSnapshot 和计算结果，保留历史导出布局。
- [X] T057 [P] [US5] 在 `lib/api.ts`、`app/statistics/page.tsx` 展示汇总来源、诊断级别、计算版本和失败重试；补充 `tests/unit/statistics-performance-summary.test.mjs` Node 契约回归。
- [X] T058 [US5] 增加 `aggregation_diagnosed`、`aggregation_calculated` 审计，真实数据行公式断档不得计入年度汇总。

## Phase 9: Quality, acceptance, and convergence

- [X] T059 [P] 运行 Python 源表解析/哈希/公式回归，核对 21/18/137/9 计数和所有阻断诊断。
- [X] T060 [P] 运行 `gofmt`、`(cd server && go test ./...)`，修复本 Feature 相关失败。2026-10-16 使用临时 Go 1.23.12 实际通过完整后端测试；修正旧权重断言及草稿启用默认值，补充创建/复制/初始化/旧表迁移/审计回滚回归，证据见 `acceptance.md`。
- [X] T061 [P] 运行 `npx tsc --noEmit`、目标 ESLint、`npm run build` 和 Node 单元测试。
- [X] T062 [P] 运行 `docker compose config`；由用户执行 `docker compose up -d --build` 后验证 `/health`，不自动清卷或改生产配置。2026-10-21 用户已完成重建，当前 Compose 服务健康检查返回 `{"status":"OK"}`。
- [X] T063 按 `quickstart.md` 在私有克隆数据完成导入、模板发布、岗位匹配、四类评分人、必填门禁、HR 诊断和历史导出页面验收，记录截图/请求 ID 到 `acceptance.md`。用户已完成软件应用开发部负责人和工程师全流程页面验收；产品经理模板映射补充回归后需在下一次重建中复验。
- [X] T064 运行 Spec-Kit Analyze，解决 CRITICAL/HIGH 规格冲突；将未实现项追加到本文件并按依赖继续执行。2026-10-16 重审发现原 A 方案与本 Feature 的维度发布门禁冲突，已纠正规格并追加 T066–T071，早先“无高风险冲突”结论作废；Docker 和页面验收仍由 T062/T063 跟踪。
- [X] T065 运行 Spec-Kit Converge，确认需求→开发→测试→验收→修复→回归闭环，无高风险未满足项后才提交用户验收。2026-10-21 收敛检查覆盖 24 项 FR、8 项 SC、5 个用户故事；发现并修复产品部门“产品经理”到“部门负责人”模板的专用映射，加入别名不跨部门泄漏回归测试。

## Dependencies & Execution Order

- Phase 1 → Phase 2 → Phase 3 是所有用户故事的阻塞基础。
- US1 完成源目录和诊断后，US2 才能发布模板；US3 依赖 US2 的已发布版本；US4 依赖 US3 的任务快照；US5 依赖 US4 的结果和快照。
- 解析测试、模型测试和前端夹具可在各自文件中并行；同一文件的实现和测试必须顺序执行。
- 任何阻断诊断、权限失败、公式断档或门禁回归未解决前，不得把 Feature 标记完成。

## Traceability

| Requirement | Covered by tasks |
|---|---|
| FR-001 / FR-002 / FR-003 | T001, T002, T006, T007, T011, T012, T013, T022, T023 |
| FR-004 / FR-015 | T008, T009, T011, T013, T055, T058, T059 |
| FR-005 / FR-006 / FR-008 | T010, T027, T030, T044, T048 |
| FR-007 / FR-013 | T043, T045, T046, T047, T048, T049, T050, T051 |
| FR-009 / FR-010 / FR-011 | T020, T034, T035, T038, T039, T040, T041 |
| FR-012 / FR-018 / FR-019 | T014, T015, T016, T029, T033, T036, T040, T042, T053, T056 |
| FR-014 | T052, T055, T057, T058 |
| FR-016 / FR-017 / FR-020 | T018, T019, T020, T021, T022, T023, T024, T025, T026, T031, T051, T054, T058 |
| SC-001 / SC-002 | T001, T002, T008, T009, T011, T013, T059 |
| SC-003 | T010, T027, T030, T044, T059 |
| SC-004 / SC-005 | T034–T051, T063 |
| SC-006 | T044, T052, T055, T059, T063 |
| SC-007 | T036, T042, T053, T056, T063 |
| SC-008 | T018, T021, T025, T026, T041, T046, T054, T062, T063 |

## Implementation Strategy

1. 先用源工作簿回归重现当前解析器失败，建立可审计的解析与公式展开基础。
2. 再做来源版本、导入诊断和模板发布门禁，确保 HR 能修正并发布。
3. 然后接入岗位匹配、评分职责和历史快照，统一评分与会签流程。
4. 最后打通 HR 汇总、纵向 PDF/Excel/批量签字导出和完整页面验收。
5. 每个阶段都执行自动化测试和手工页面验证；Docker 构建由用户按 quickstart 执行。

## Phase 10: Convergence

- [X] T066 对齐 `spec.md`、`plan.md`、`research.md` 和 `contracts/template-import-api.md` 与已确认 A 方案：保留原权重，维度内合计仅告警，逐项舍入综合权重合计不为 100 才阻断；同步实际权限和接口路径，记录本次规格冲突及纠正 per FR-006, FR-008, US1/AC3, US2/AC3 (contradicts)。
- [X] T067 在 `scripts/verify_source_workbook.py` 和 `scripts/test_source_workbook.py` 按 B:H 真实业务输入识别 HR 数据行，覆盖仅序号、空白字符串、0 分和无序号输入，保留 I17/J17 来源诊断并重建 catalog per FR-015, US5/AC1 (contradicts)。
- [X] T068 在 `server/handlers/source_workbook_parser.go`、`source_workbook.go` 及测试中实际解析上传 XLSX、保存不可变来源清单，支持修订文件新 SHA 及相同 SHA 重新校验诊断，禁止用固定嵌入 catalog 替代上传事实 per FR-001, FR-004, FR-016, plan: Excel source decision (partial)。
- [X] T069 在 `server/handlers/source_workbook.go`、`hongxing_catalog.go`、`template_calculation.go` 及测试中严格落实 draft/publish 事务，纠正旧策略误阻断且保留诊断证据，保护 HR 修改、已发布版本和历史考核 per FR-008, FR-018, FR-019, US1/AC4 (contradicts)。
- [X] T070 在 `app/templates/page.tsx`、`lib/api.ts` 及页面契约测试中明确告警接受、实际新增/复用数量和导入状态，成功后关闭过期导入操作并提供重校验入口 per FR-016, US1/AC1 (partial)。
- [X] T071 运行本轮 Python、Go、Node、TypeScript、构建及 Compose 配置回归，将原表 0 阻断/22 告警、修订文件、草稿/发布及历史保护证据和最短页面验收步骤写入 `acceptance.md`、`quickstart.md`，保留待用户构建和页面验收状态 per FR-018, SC-003, SC-008, Constitution IV (partial)。

## Phase 11: Template lifecycle convergence

- [X] T072 [US2] 在 `server/handlers/source_workbook_regression_test.go` 增加新来源 SHA 发布时自动切换当前版本、旧版本保留 retired、同匹配键只有一个 active 的回归测试 per FR-008, FR-019, US2/AC4 (contradicts)。
- [X] T073 [US2] 在 `server/handlers/hongxing_catalog.go` 实现同一部门/岗位/周期的版本切换事务：新来源发布新版本，旧 published 版本原子 retired，同来源重复导入保持幂等，历史考核不变 per FR-018, FR-019 (partial)。
- [X] T074 [US2] 在 `app/templates/page.tsx` 默认隐藏 retired 历史版本、提供显式“显示历史版本”开关，并明确发布新版本会切换当前使用版本 per FR-016, US2/AC4 (partial)。
- [X] T075 [US3] 在 `server/handlers/department_hierarchy.go` 增加幂等数据修复：子部门负责人缺少直属上级时，只回填上级部门负责人或 CTO，不覆盖 HR 已设置值 per FR-010, FR-011, Constitution V (partial)。
- [X] T076 [US4] 在 `app/evaluations/page.tsx` 的新流程总结中展示统一加权结果和满分 100，隐藏旧自评/主管/HR 零值字段，补充页面契约回归 per FR-006, FR-014, Constitution III (contradicts)。
- [X] T077 更新 `acceptance.md`、`quickstart.md`，运行 Go、Node、TypeScript、ESLint、构建和 Compose 配置回归，并记录模板生命周期、组织回填和汇总显示证据 per SC-003, SC-007, SC-008 (partial)。

## Phase 12: Excel workflow completion

- [X] T078 [US4] 增加员工工作总结、问题风险、所需支持、后续计划和提交时间字段，提供本人提交 API/页面，并把总结提交设为自评分与流程推进的必填门禁 per FR-005, FR-007, FR-013。
- [X] T079 [US4] 为评分任务保存 Excel 定义的评分顺位，后端阻止 PM 未完成时提交第二/第三顺位评分，补充并发和越序回归 per FR-005, FR-009, FR-011。
- [X] T080 [US2] 将流程表八步流程和会签摘要保存到来源快照，模板初始化使用流程表默认会签规则，不再只依赖岗位名称硬编码 per FR-005, FR-009。
- [X] T081 [US5] 增加员工项目投入占比数据模型、考核创建/维护输入和汇总接口，支持多项目、低于 20% 贡献为 0 per FR-014, SC-006。
- [X] T082 [US5] 将月度/季度/年度汇总改为真正聚合多条已完成考核，按有效投入占比计算贡献、年度加权结果和空周期排除 per FR-014, SC-006。
- [X] T083 [US2] 收紧模板删除、指标读取权限和发布事务：禁止删除 active 模板、要求 `kpi:view`、消除并发双 active 窗口 per FR-018, FR-019, Constitution I。
- [X] T084 [US1] 提供只读预览和显式确认的孤立评分任务清理，清理前要求备份、只删除指定 orphan IDs，并写审计 per FR-017, Constitution V。
