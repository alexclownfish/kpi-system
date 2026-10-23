# 导出诊断验收（T087）

## 范围

本增量只统一导出接口的可诊断反馈，不修改 PDF/Excel 字段、版式、方向或历史快照。

## 已实现

- 单份导出成功响应返回 `request_id`、`result_version` 和 `checksum`。
- 批量签字 ZIP 返回每份文件对应的 `evaluation_id`、`result_version` 和 `checksum`。
- 单份、批量签字和最终评分导入模板的失败响应使用稳定错误码。
- 可重试失败在 `error.details.retryable=true`，并返回 `retry_guidance`；已有结果版本时同时返回 `result_version`。
- 考核页导出失败提示包含错误码、请求 ID、结果版本和可操作建议。

## 自动化证据

- 使用当前工作区源码传入一次性后端构建容器执行 `go test -count=1 ./...`：通过。
- `tsc --noEmit`：通过。
- 定向 ESLint（`app/evaluations/page.tsx`、`lib/api.ts`）：通过。
- Next.js production build：通过；仅保留既有 `app/templates/page.tsx` hook 依赖警告。
- `docker compose config --quiet`：通过。
- `git diff --check`：通过。

## 页面验收方法

用户执行 `docker compose up -d --build` 后，登录管理员进入“考核管理”：

1. 打开一条已定稿考核，分别点击“导出 Excel”和“导出 PDF”，确认下载成功提示包含结果版本。
2. 在批量结果区域使用无结果筛选条件，点击批量导出，确认页面显示错误码和筛选调整建议。
3. 若服务端导出失败，将错误提示中的 `request_id` 提供给管理员；可重试错误按提示稍后重试。

本轮未启动、重建或清理 Compose 服务及数据库卷。
