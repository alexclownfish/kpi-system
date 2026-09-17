# Implementation Plan: 绩效结果打印签字与归档

**Branch**: `002-result-signoff` | **Date**: 2026-09-17 | **Spec**: [spec.md](./spec.md)

## Summary

复用现有单考核 Excel 导出能力，增加不可变结果快照和打印签字版式；增加受保护的纸质签字附件上传、下载和确认记录；将员工在线确认改为专用事务接口，统一保存确认凭证并锁定已完成结果。

## Technical Context

**Language/Version**: TypeScript 5 / React 19 / Next.js 15；Go 1.23

**Primary Dependencies**: Axios、Gin、GORM、excelize v2

**Storage**: SQLite 保存快照与确认元数据；Docker 持久卷中的受保护目录保存签字附件

**Testing**: Go `testing` + `httptest`；TypeScript `tsc --noEmit`；Next production build；Docker Compose 配置与健康检查

**Target Platform**: Linux Docker Compose Web 应用

**Project Type**: Next.js 前端 + Go REST API

**Performance Goals**: 单条结果导出和确认操作在常规数据量下 2 秒内给出响应；10 MB 上传不长期占用内存副本

**Constraints**: 附件不得放入公开静态目录；单文件最大 10 MB；已完成结果不可通过通用更新接口修改；保留现有数据

**Scale/Scope**: 单次处理一条考核与一个附件；本阶段不做批量导入和 PDF 报表生成

## Constitution Check

- **I 后端强制授权 — PASS**：导出、纸质确认、查询确认、附件下载均在后端校验权限。
- **II 数据范围隔离 — PASS**：所有新增读取通过 `ApplyEvaluationScope`/`CanAccessEvaluation` 限制。
- **III 接口契约与反馈 — PASS**：新增接口记录于 contracts，前端包含 loading/success/error。
- **IV 可测试增量交付 — PASS**：权限允许/拒绝、状态流转、过期版本、文件校验均有后端测试任务。
- **V 数据与部署安全 — PASS**：使用 AutoMigrate 添加表；Compose 新增独立持久卷，不清理现有卷。
- **VI 单一代码源与最小复杂度 — PASS**：只修改根目录正式源码，复用现有 Excel、RBAC 与数据范围。

设计完成后复核：无例外或 Complexity Tracking 项。

## Project Structure

### Documentation

```text
specs/002-result-signoff/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/result-signoff-api.md
└── tasks.md
```

### Source Code

```text
app/evaluations/page.tsx          # 导出、确认信息及纸质回录 UI
lib/api.ts                        # 新增结果签字 API 类型与调用
server/models/models.go           # 快照与确认实体
server/models/database.go         # 幂等自动迁移
server/handlers/export.go         # 受控导出、打印版式与结果快照
server/handlers/confirmation.go   # 在线/纸质确认、附件下载、锁定逻辑
server/handlers/confirmation_test.go
server/routes/routes.go           # 新增专用路由和权限
docker-compose.yml                # 签字附件持久卷
```

**Structure Decision**: 延续现有页面聚合与 Gin handler 结构，不新增服务层或第二套前端页面。

## Complexity Tracking

无宪章例外。
