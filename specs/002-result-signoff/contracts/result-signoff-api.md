# API Contract: Result Sign-off

所有接口要求 JWT。错误结构至少包含 `error`，校验错误可包含 `message`。

## GET `/api/export/evaluation/:id`

- 权限：`report:export`
- 数据范围：必须能访问该考核
- 状态：仅 `pending_confirm`、`completed`
- 成功：现有 `ExportResponse`，新增 `result_version`、`checksum`
- 错误：400 结果未定稿；403 权限或范围拒绝；404 不存在

## GET `/api/evaluations/:id/confirmation`

- 权限：`assessment:view`
- 数据范围：必须能访问该考核
- 成功：`{ data: EvaluationConfirmation | null }`

## POST `/api/evaluations/:id/confirm-online`

- 权限：`assessment:submit`
- 限制：仅被考核员工本人；状态 `pending_confirm`；无未处理异议；未确认
- 成功：`{ message, data: EvaluationConfirmation, evaluation: KPIEvaluation }`
- 错误：400 非法状态/异议/重复；403 非本人

## POST `/api/evaluations/:id/confirm-paper`

- Content-Type: `multipart/form-data`
- 权限：`assessment:approve`
- 字段：`signed_at`（YYYY-MM-DD，必填）、`remark`（可选，最多 500 字）、`file`（必填）
- 文件：PDF/JPG/JPEG/PNG，最大 10 MB
- 限制：状态 `pending_confirm`；无未处理异议；最近导出版本必须匹配当前结果
- 成功：`{ message, data: EvaluationConfirmation, evaluation: KPIEvaluation }`
- 错误：400 文件/日期/版本/状态错误；403 范围拒绝；409 已确认

## GET `/api/evaluations/:id/confirmation/attachment`

- 权限：`assessment:view`
- 数据范围：必须能访问该考核
- 成功：附件流，使用原文件名下载
- 错误：404 无纸质附件；403 权限或范围拒绝

## 锁定规则

通用考核更新和评分更新接口在考核 `completed` 时返回 400，不允许改变得分、总分、总结或异议结果。
