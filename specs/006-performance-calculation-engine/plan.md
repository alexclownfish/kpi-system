# Implementation Plan: 绩效计算引擎与模板维护

**Branch**: `main` | **Date**: 2026-09-26 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/006-performance-calculation-engine/spec.md`

**Note**: This template is filled in by the `$speckit-plan` command; its definition describes the execution workflow.

## Summary

将附件中的研发绩效 Excel 规则结构化为可维护的考核模板和后端计算引擎。模板负责维度、指标、权重、评分人类型及规则；创建考核时生成不可变快照；评分完成后由后端统一计算总分和等级，并为现有导出、签字和 HR 汇总提供统一结果。

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.23；TypeScript/React 19；Next.js 15

**Primary Dependencies**: Gin、GORM、SQLite、现有 Next.js UI 与 Axios API 层

**Storage**: 现有 SQLite，使用 AutoMigrate 和兼容性迁移

**Testing**: Go 单元/接口测试、TypeScript 检查、Next 构建、Docker 冒烟和 Excel 公式对照测试

**Target Platform**: Docker Compose 部署的 Linux Web 应用

**Project Type**: Next.js 前端 + Go API Web 应用

**Performance Goals**: 单次考核 100 个指标内计算与保存 p95 < 500ms；批量汇总 1000 条考核可在 10 秒内完成

**Constraints**: 逐项 ROUND(...,1) 后综合权重合计必须为 100%，不要求维度内权重合计 100%，不归一化；评分范围 0–100；空白不等于 0；已创建考核使用模板快照；不执行用户上传的公式、宏或脚本

**Scale/Scope**: 9 个研发部门、负责人和工程师两类岗位模板；少于 100 个模板、单模板少于 200 个指标、单次考核少于 100 个指标

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

通过：复用现有模板、考核、结果快照与导出体系；后端授权和数据范围保持不变；所有计算在服务端完成；历史结果不可被模板修改破坏；迁移不删除既有数据卷和表。

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file ($speckit-plan command output)
├── research.md          # Phase 0 output ($speckit-plan command)
├── data-model.md        # Phase 1 output ($speckit-plan command)
├── quickstart.md        # Phase 1 output ($speckit-plan command)
├── contracts/           # Phase 1 output ($speckit-plan command)
└── tasks.md             # Phase 2 output ($speckit-tasks command - NOT created by $speckit-plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```text
server/models/models.go
server/models/database.go
server/handlers/kpi.go
server/handlers/evaluation.go
server/handlers/invitation.go
server/handlers/statistics.go
server/handlers/result_export.go
server/routes/routes.go
server/handlers/calculation_test.go
app/performance-rules/page.tsx
app/evaluations/page.tsx
lib/api.ts
lib/auth-context.tsx
```

**Structure Decision**: 沿用现有 Go API、GORM 模型和 Next.js 页面；新增计算服务和模板编辑 UI，扩展现有 KPI 模板与考核快照，不引入第二套权限、报表或公式执行引擎。

## Architecture Decisions

1. **模板归属**：模板按部门、岗位类型和周期匹配；岗位不直接保存计算公式。
2. **实例快照**：创建考核时复制维度、指标、权重、评分人配置和计算版本。
3. **评分聚合**：MVP 多人评分默认算术平均，保留原始评分与评分人；后续可扩展配置聚合方式。
4. **计算边界**：第一阶段人工录入 0–100 分；项目计划、禅道、测试平台等数据源自动换算另列后续 Feature。
5. **导出关系**：复用 `005-unified-result-exports`，导出读取最终结果快照，不重新读取当前模板。

## Delivery Phases

- Phase 0：研究现有模型、附件字段和兼容迁移策略。
- Phase 1：模板维度/指标/评分人结构与校验。
- Phase 2：岗位自动匹配、考核快照和评分任务。
- Phase 3：总分、等级、必填指标和确认前置校验。
- Phase 4：HR 月度/季度/年度汇总。
- Phase 5：前端模板维护、评分进度和结果展示。
- Phase 6：迁移、对照测试、Docker 验证。

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
