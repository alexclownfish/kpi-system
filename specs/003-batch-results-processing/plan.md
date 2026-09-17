# Implementation Plan: 批量结果处理与签字回收

## Technical Context

- Go 1.23、Gin、GORM、SQLite、Excelize；Next.js/React/TypeScript。
- Debian Chromium headless + Noto CJK fonts 生成 PDF；Go `archive/zip` 打包。
- 复用结果快照、确认记录、RBAC 和 `ApplyEvaluationScope`。

## Constitution Check

- 新接口后端强制权限和数据范围。
- 预检、提交返回逐项结果并在前端明确反馈。
- 新表只 AutoMigrate，不删除或重建 SQLite 卷。
- 导入批次和提交写审计日志；实现后运行自动测试与 Converge。

## Design

1. `FinalScoreImportBatch` 用 JSON 保存服务端验证后的考核级变更与摘要。
2. 提交重新校验状态、数据范围和 `updated_at`，每条考核独立事务。
3. 批量导出复用快照，临时 HTML 经 Chromium 输出 PDF，再与汇总 XLSX 压缩。
4. 统计从考核左连接确认记录，只统计待确认和已完成。
