# Data Model: 统一绩效结果导出

## KPITemplate extension

- `ExportLayoutJSON`: 可空文本；保存规范化的 `ResultExportLayout`。空值表示系统默认简版。
- AutoMigrate 新增列，不批量重写旧模板。
- 读取空值或无法解析的旧值时回退默认简版；API 保存时严格校验。

## ResultExportLayout value object

- `version`: 当前为 `1`。
- `preset`: `final_signoff` 或 `full_process`。
- `title`: 1-60 个字符，默认“绩效考核结果确认表”。
- `columns`: 2-11 个唯一字段；必须含 `item_name` 和 `final_score`。
- `show_summary`: 布尔值。
- `show_employee_opinion`: 布尔值。
- `signature_labels`: 1-6 个非空、去重标签，每个最多 20 字符。

字段目录：`item_name`、`item_description`、`max_score`、`self_score`、`self_comment`、`manager_score`、`manager_comment`、`hr_score`、`hr_comment`、`final_score`、`final_comment`。

## EvaluationResultSnapshot payload extension

`resultSnapshotItem` 增加自评、主管、HR 的分数和说明字段；`resultSnapshotPayload` 增加 `export_layout`。

兼容规则：旧快照没有布局时使用默认简版；没有过程评分字段时对应单元格为空，最终得分和最终评价继续可用。

## UnifiedExportDocument

运行时对象，不持久化：标题、考核编号、结果版本、校验码、基本信息、动态列、明细行、总结区、员工确认意见区和签字栏。

## State transition

1. 模板创建/更新时布局被规范化、校验并保存。
2. 考核进入 `pending_confirm` 后首次导出时创建包含布局的结果快照。
3. `pending_confirm` 考核在模板版式变化后再次导出时，以当前结果和最新已保存版式生成新快照版本。
4. 在线确认或纸质签字归档完成后考核进入 `completed`，此后始终使用完成时的最新快照布局。
5. 旧快照回退默认简版；已完成历史考核不得因模板变化生成新布局版本。
