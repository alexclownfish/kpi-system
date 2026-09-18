# Implementation Plan: 统一绩效结果导出

**Branch**: `005-unified-result-exports` | **Date**: 2026-09-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-unified-result-exports/spec.md`

## Summary

将模板级“结果导出版式”作为带版本的受控 JSON 值对象保存到 `KPITemplate`，在考核结果快照生成时把规范化版式和完整评分过程一并冻结。新增统一的导出视图模型与 Excel/PDF 渲染器，使单个 Excel、单个 PDF 和批量 PDF 共享同一字段解析、顺序和签字栏逻辑。模板管理页提供两个预设以及字段选择、排序、区域开关和签字栏编辑；旧模板与旧快照回退到“最终签字简版”。

## Technical Context

**Language/Version**: Go 1.23；TypeScript；React 19

**Primary Dependencies**: Gin、GORM、SQLite、excelize、Chromium headless；Next.js 15、现有 UI 组件库

**Storage**: SQLite；`kpi_templates.export_layout_json` 保存当前模板配置，`evaluation_result_snapshots.snapshot_json` 保存定稿时的版式副本

**Testing**: Go `testing` + `httptest`；TypeScript `tsc --noEmit`；Next.js production build；Docker Compose 构建验证

**Target Platform**: Docker Compose 部署的 Linux Web 应用

**Project Type**: Next.js 前端 + Go API 后端的单仓库 Web 应用

**Performance Goals**: 单份导出在正常考核规模下交互可用；100 份批量导出不串用模板配置且全部成功生成

**Constraints**: 保留现有 SQLite 数据；不能清除 Compose 数据卷；历史快照必须兼容；导出必须执行权限与数据范围校验；不引入自由表格设计器

**Scale/Scope**: 2 个系统预设、11 个受控明细字段、最多 6 个签字栏；单份 Excel/PDF 与批量 PDF

## Constitution Check

*GATE: Passed before research and re-checked after design.*

- **后端强制授权**: 模板更新继续使用 `kpi:edit`，所有导出继续使用 `report:export`，单份导出保留 `ApplyEvaluationScope`，批量导出保留范围过滤。PASS。
- **数据范围隔离**: 不改变现有范围语义；新增格式参数不得绕过范围查询。PASS。
- **明确接口契约与用户反馈**: contracts 记录模板配置及格式参数；前端保存和导出使用现有用户反馈。PASS。
- **可测试的增量交付**: 为布局校验、快照冻结、统一字段解析、权限/范围拒绝和历史兼容增加自动化测试。PASS。
- **数据与部署安全**: AutoMigrate 只新增可空文本列，默认回退不强制重写历史行；不操作数据卷。PASS。
- **单一代码源与最小复杂度**: 只修改根目录正式代码；采用模板内嵌值对象而非新增复杂设计器和通用报表引擎。PASS。

## Project Structure

### Documentation (this feature)

```text
specs/005-unified-result-exports/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── result-export-api.md
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── models/models.go
├── handlers/template.go
├── handlers/confirmation.go
├── handlers/export.go
├── handlers/batch_results.go
├── handlers/result_export.go
├── handlers/result_export_test.go
└── routes/routes.go

app/templates/page.tsx
app/evaluations/page.tsx
lib/api.ts
```

**Structure Decision**: 复用现有 handler 组织方式，在 `server/handlers/result_export.go` 集中布局规范化、快照解析、统一视图模型和格式渲染；两个旧 handler 仅保留请求编排，避免引入新的 service 层体系。

## Design Phases

### Phase 0 - Research

- 明确模板与岗位职责边界、预设和历史兼容策略。
- 明确快照冻结时点以及 Excel/PDF 共用语义模型的边界。

### Phase 1 - Data and Contracts

- 为模板增加可空布局 JSON。
- 扩展结果快照 payload，纳入评分过程和布局副本。
- 定义模板更新与单份导出格式参数契约。

### Phase 2 - Implementation

- 后端先实现布局规范化、校验和快照兼容测试。
- 实现统一导出文档模型与 Excel/PDF 渲染。
- 替换单个和批量两条旧渲染链路。
- 增加模板配置 UI 与单份格式选择。
- 执行 Go 测试、类型检查、生产构建和 Compose 配置验证。

## Post-Design Constitution Check

设计没有新增权限或数据范围类型；所有写入和导出仍由后端权限保护。Schema 为向后兼容的新增可空列，旧数据通过运行时默认值读取。单一统一渲染模型消除了第二套业务实现。PASS。

## Complexity Tracking

无宪章例外。
