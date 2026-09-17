# Implementation Plan: MVP 验收与回归基线

**Branch**: `001-mvp-acceptance-baseline` | **Date**: 2026-09-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-mvp-acceptance-baseline/spec.md`

## Summary

为当前 KPI MVP 增加可重复执行的端到端验收入口。使用浏览器验收覆盖 HR 登录、员工创建、
用户反馈和页面权限；使用 API 验收覆盖角色权限、数据范围、拒绝场景、服务健康和精确清理。
验收数据使用唯一标识并在 `finally`/teardown 阶段按 ID 反向删除，不修改种子账号，不删除
SQLite 文件或 Compose 数据卷。

## Technical Context

**Language/Version**: TypeScript 5 / Node.js 20；现有后端 Go 1.23  
**Primary Dependencies**: Next.js 15、React 19、Axios；新增 Playwright Test 作为验收运行器  
**Storage**: 现有 SQLite 命名卷；本 Feature 不新增生产表或迁移  
**Testing**: Playwright browser/API tests、Go `go test ./...`、TypeScript `tsc --noEmit`、Next.js build  
**Target Platform**: macOS 开发主机上的 Docker Desktop/Colima，Linux ARM64/AMD64 容器  
**Project Type**: Web application（Next.js 前端 + Go API + Nginx 网关）  
**Performance Goals**: P1 验收在 5 分钟内完成；单个网络步骤默认不超过 10 秒；服务就绪等待不超过 60 秒  
**Constraints**: 不清空数据库、不删除数据卷、不修改种子身份；输出不得包含密码、完整 JWT 或哈希  
**Scale/Scope**: 1 个验收入口，3 类角色，至少 4 个权限允许/拒绝场景，2 个临时员工身份

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Gate | Plan Evidence | Status |
|---|---|---|---|
| I. 后端强制授权 | 同时验证允许和拒绝，不以页面隐藏代替 API 结果 | API 场景矩阵包含 200/201 与 403 检查 | PASS |
| II. 数据范围隔离 | 验证普通员工 SELF、负责人 DEPARTMENT | 登录响应和员工查询结果均纳入断言 | PASS |
| III. 接口契约与反馈 | 验证提交中、成功、失败及秘密脱敏 | 浏览器场景 + 结构化结果契约 | PASS |
| IV. 可测试增量交付 | 每个用户故事有独立验收任务 | 测试按 US1-US4 分组 | PASS |
| V. 数据与部署安全 | 不删除卷，测试数据精确清理 | 唯一前缀、ID 栈、反向清理、前后计数 | PASS |
| VI. 单一代码源 | 仅修改根目录正式代码 | `kpi-main/` 明确排除 | PASS |

**Pre-research gate result**: PASS，无需 Complexity Tracking 例外。

## Project Structure

### Documentation (this feature)

```text
specs/001-mvp-acceptance-baseline/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── acceptance-runner.md
│   └── api-scenarios.md
├── checklists/
│   └── requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
app/
├── auth/login/page.tsx
└── employees/page.tsx

components/
├── alert.tsx
└── protected-route.tsx

lib/
├── access-control.ts
├── api.ts
└── auth-context.tsx

server/
├── handlers/
│   ├── employee.go
│   └── permissions_test.go
├── models/
└── routes/

tests/
└── e2e/
    ├── README.md
    ├── fixtures/
    │   └── acceptance.ts
    ├── helpers/
    │   ├── acceptance-context.ts
    │   ├── api-client.ts
    │   ├── acceptance-reporter.ts
    │   ├── cleanup.ts
    │   └── redaction.ts
    ├── deployment-readiness.spec.ts
    ├── mvp-acceptance.spec.ts
    ├── redaction.spec.ts
    └── reporting.spec.ts

scripts/
└── acceptance/
    └── run-mvp.sh

playwright.config.ts
package.json
docker-compose.yml
```

**Structure Decision**: 保留现有 Web 应用结构，在根目录增加独立 `tests/e2e/` 和验收启动脚本。
不在 `kpi-main/` 创建镜像测试，也不引入第二套后端测试框架。

## Phase 0: Research Decisions

研究结论见 [research.md](./research.md)。核心决策：

- Playwright 同时承担浏览器交互和 API 请求，避免维护两套端到端运行器。
- 后端已有单元/路由测试继续保留；跨服务链路由端到端验收覆盖。
- 临时部门负责人先创建，普通员工引用其 ID，清理时先员工后负责人。
- 测试失败仍执行清理；清理失败成为独立失败并输出非敏感 ID。
- 使用可访问标签和按钮名称定位 UI，仅在现有语义不足时增加少量稳定测试标识。

## Phase 1: Design and Contracts

- 临时验收实体和生命周期见 [data-model.md](./data-model.md)。
- 验收运行入口、参数、退出码和结果格式见
  [contracts/acceptance-runner.md](./contracts/acceptance-runner.md)。
- API 允许/拒绝和数据范围矩阵见 [contracts/api-scenarios.md](./contracts/api-scenarios.md)。
- 本地构建、执行和人工复核步骤见 [quickstart.md](./quickstart.md)。

## Post-Design Constitution Check

设计完成后复核结果仍为 PASS：

- 没有以浏览器菜单状态替代服务端拒绝测试。
- 测试数据生命周期覆盖部分成功、异常和清理失败。
- 验收报告明确禁止输出认证秘密。
- 未增加生产数据模型或破坏性部署操作。
- 所有预期修改路径均位于根目录正式代码源。

## Complexity Tracking

无宪章例外或需要论证的额外复杂度。
