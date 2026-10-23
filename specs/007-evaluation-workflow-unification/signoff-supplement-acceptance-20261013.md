# 已完成考核补充签字材料增量验收（2026-10-13）

## 目的

员工已经在线确认后，纸质签字材料可能稍后才回收。本增量将纸质扫描件
作为已完成考核的补充证据归档，保持完成状态、最终结果和原确认记录不变。

## 已落地行为

| 场景 | 预期 | 当前证据 |
|---|---|---|
| `completed` 且已有最终确认，上传 PDF/JPG/JPEG/PNG | 新增补充记录，状态仍为 `completed` | `UploadConfirmationSupplement` 与 `TestCompletedEvaluationAcceptsSignoffSupplementWithoutChangingConfirmation` |
| 上传后检查原确认 | 原确认 ID、方法和确认时间不变 | `server/handlers/confirmation_test.go` 已断言 |
| 上传后检查审计 | 生成 `upload_signoff_supplement`，备注内容不进入审计详情 | 同一回归测试已断言 |
| 补充材料读取 | 具备 assessment view 和范围权限的用户可列表、下载 | 同一回归测试覆盖列表与文件下载 |
| 尚未完成时上传 | 返回冲突，不创建补充记录 | `TestSignoffSupplementRejectsNonCompletedEvaluation` |
| 页面操作 | 完成状态显示“补充纸质签字”，成功提示“完成状态不变”，并展示历史补充材料 | `app/evaluations/page.tsx` 与 `lib/api.ts` |
| 删除考核 | 一并清理补充记录和签字文件 | `TestDeleteEvaluationCleansWorkflowChildrenAndPaperAttachment` |
| 删除失败 | 数据库行和已暂存附件均恢复，不留 `.deleting-*` 临时目录 | `TestDeleteEvaluationRestoresSupplementWhenChildDeleteFails` |

## API 验收基线

- `POST /api/evaluations/:id/confirmation/supplements`：要求
  `assessment:approve` 和考核数据范围；必须是 `completed` 且存在最终确认。
- `GET /api/evaluations/:id/confirmation/supplements` 和
  `GET /api/evaluations/:id/confirmation/supplements/:supplement_id/attachment`：
  要求 `assessment:view` 和相同数据范围，读取无业务写副作用。
- 文件大小上限为 10 MB，支持 PDF、JPG、JPEG、PNG；签字日期为可选的
  `YYYY-MM-DD`，备注最多 500 字。
- 文件使用服务端生成的安全存储名；响应只暴露原始文件名、类型和大小等
  展示元数据。
- 每次成功上传产生一个 `upload_signoff_supplement` 审计动作，包含考核和
  补充记录 ID 及非敏感文件元数据，不包含备注正文或原始文件内容。

## 自动化与环境依赖

现有 `server/handlers/confirmation_test.go` 已覆盖成功上传、原确认不可变、
`completed` 状态保持、审计备注脱敏、列表/下载、非 completed 拒绝、非法/空/超大
文件、错误日期/备注、缺少最终确认、越权读取、并发追加和数据库写入失败清理。
`server/handlers/kpi_deletion_test.go` 已覆盖删除成功清理和删除失败回滚。

T123 的 Playwright 场景已经加入 `tests/e2e/evaluation-workflow.spec.ts` 和
`tests/e2e/README.md`，但需要在用户启动真实服务并提供验收账号后执行；本轮未启动
Compose，因此只保留真实环境验收待办。无服务时 Playwright 会因无法连接
`KPI_BASE_URL` 而失败，不能视为功能失败。

T124 已完成：补充上传、列表和 JSON 错误响应现在包含统一 `request_id`；成功
上传仍保留原有 `data` 和 `evaluation.status` 字段，文件下载成功时继续返回
原始文件流并回显 `X-Request-ID`；`confirmation_test.go` 增加成功与终态冲突的
request ID/错误码合同断言。

## 手工验收步骤

1. 创建并完成一条在线确认的临时考核，记录状态、总分、结果版本、快照校验码
   和确认记录。
2. 以有审批权限且在数据范围内的账号上传签字图片，确认提示完成状态不变。
3. 重新读取考核和确认记录，确认所有记录值不变；列表并下载补充文件，核对
   文件内容和元数据。
4. 再上传一份材料，确认两条补充记录均保留。
5. 以无范围账号和无审批权限账号分别尝试读取/上传，确认拒绝且无副作用。
6. 删除临时考核，确认补充数据库行和附件文件均清理。不要删除 Compose 数据卷。
