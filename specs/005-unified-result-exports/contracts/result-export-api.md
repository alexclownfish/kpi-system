# API Contract: 统一绩效结果导出

## Template payload

模板响应增加规范化 `export_layout`：

```json
{
  "version": 1,
  "preset": "final_signoff",
  "title": "绩效考核结果确认表",
  "columns": ["item_name", "max_score", "final_score", "final_comment"],
  "show_summary": true,
  "show_employee_opinion": true,
  "signature_labels": ["员工签字", "直属主管签字", "HR签字", "签字日期"]
}
```

`PUT /api/templates/:id` 可包含 `export_layout`，沿用 `kpi:edit`。无效配置返回 `400`：

```json
{"error":"导出版式配置无效","message":"导出列必须包含指标和最终得分"}
```

## Single result export

`GET /api/export/evaluation/:id?format=xlsx|pdf`

- `format` 默认 `xlsx`，兼容现有调用。
- 仅允许 `xlsx`、`pdf`。
- 必须有 `report:export` 并通过考核数据范围校验。
- 仅允许 `pending_confirm`、`completed`。

成功响应沿用 `ExportResponse`，含 `result_version` 和 `checksum`。非法格式返回 `400`。

## Batch signoff export

`GET /api/export/signoff-batch` 查询参数保持不变。每个 PDF 使用该考核快照内布局；尚无快照则创建含当前模板布局的新快照。ZIP 继续包含 `签字回收汇总.xlsx`。

## Audit

- 布局变化：`update_template_export_layout`。
- 单份导出：`export_evaluation_result`，记录格式。
- 批量导出：`export_signoff_batch`。
