# Template Import v2 API Contract

所有写接口都要求认证、服务端权限和数据范围校验；错误返回 `{error:{code,message,details},request_id}`。响应不得包含密码、密码哈希或未授权员工。

## Source workbook validation

### `POST /api/source-workbooks/validate`

权限：`kpi:view` 或 `system:edit`，且必须在 HR 可见范围内。

请求：`multipart/form-data`，字段 `workbook`（`.xlsx`，最大 10 MB）。按上传内容解析并按 SHA 保存不可变解析清单，不创建部门、模板、指标或考核；相同 SHA 重新校验刷新诊断，不重写已导入业务版本。不同 SHA 的修订文件独立保存。

成功 `200`：

```json
{
  "data": {
    "workbook_id": "workbook-...",
    "source_sha256": "50bcf21d...3735fb3",
    "parser_version": "v2",
    "sheet_count": 21,
    "summary": {
      "process_sheets": 1,
      "position_templates": 18,
      "hr_summary_sheets": 2,
      "source_departments": 9,
      "indicators": 137
    },
    "diagnostics": [
      {
        "severity": "warning",
        "code": "DIMENSION_INNER_WEIGHT_SUM_INVALID",
        "sheet": "02-结构部门-部门负责人",
        "cell": "E...",
        "message": "维度内权重合计为 130",
        "repair_hint": "保留原权重；按逐项舍入后的综合权重合计 100% 校验发布"
      }
    ],
    "can_import_draft": true,
    "can_publish": true
  },
  "request_id": "req_..."
}
```

失败：`400 INVALID_WORKBOOK`、`413 WORKBOOK_TOO_LARGE`、`403 FORBIDDEN`、`500 WORKBOOK_PARSE_FAILED`。解析失败不返回部分模板。

### `GET /api/source-workbooks/:id/diagnostics`

权限：`kpi:view` 或 `system:edit`。

返回完整工作表、公式单元格、部门映射、岗位模板和 HR 汇总诊断，按 `severity`、工作表顺序和单元格排序。不存在返回 `404 SOURCE_WORKBOOK_NOT_FOUND`。

## Import and template publication

### `POST /api/source-workbooks/:id/import`

权限：`kpi:edit` 或 `system:edit`。

请求：

```json
{
  "source_sha256": "50bcf21d...3735fb3",
  "mode": "draft",
  "explicit_department_mappings": [],
  "allow_warning_diagnostics": true
}
```

`mode=draft` 新增模板必须为 draft/inactive，不允许用于新建考核；`mode=publish` 仅当无 blocking 诊断且综合权重逐项舍入合计为 100 时允许。维度内合计告警和仅序号空白行缺公式告警不单独阻止发布，接受告警时提交 `allow_warning_diagnostics=true`。若同一部门、岗位、周期已有发布版本，新来源发布会在同一事务中创建新版本并将旧版本状态改为 `retired`、`is_active=false`；事务失败不能残留部分目录。

成功 `201` 返回工作簿版本、`mode`、`created_templates`/`reused_templates`、模板 ID 和来源 SHA。同来源已有模板可复用：draft 保持已有状态；publish 只复用已发布且启用的模板，遇到同来源已有草稿/停用版本须到模板列表显式处理，事务原子拒绝。不同来源的新 revision 会切换当前版本，旧版本仅作为历史保留，历史考核快照不变。已成功导入的来源重复提交返回 `200` 和实际复用数量，不重复创建模板或来源版本。

### `GET /api/templates`

沿用现有模板查询，增加 `source_workbook_sha256`、`source_sheet`、`position_type`、`period`、`calculation_version`、`export_layout_version` 和 `diagnostic_status`。已发布模板按部门、岗位类型和周期唯一匹配。

### `POST /api/templates/:id/publish`

权限：`kpi:publish`。

发布前校验：来源哈希存在、部门/岗位/周期唯一、每个维度有指标、所有权重有限且在 0–100、综合权重逐项舍入合计 100、必填指标有评分职责、阻断诊断为零。失败返回 `400 TEMPLATE_PUBLISH_BLOCKED`，`details` 包含指标、维度、来源行和修复提示。发布新 revision 时，旧 active 版本由同一来源事务退役，不在页面默认列表展示。

## Template-driven evaluation creation

### `GET /api/templates/match-preview`

权限：`kpi:view`，后端按员工与考核数据范围继续校验。

查询参数：`employee_id`、`period`。服务端以员工实际组织数据为准。

成功返回唯一已发布模板、指标数量、职责候选和会签摘要；不创建数据。多个匹配返回 `409 TEMPLATE_MATCH_AMBIGUOUS`，无匹配返回 `404 TEMPLATE_NOT_FOUND`。

### `POST /api/evaluations`

沿用考核创建接口，新增响应字段：`template_version`、`source_workbook_sha256`、`scorer_snapshot`、`signoff_snapshot`、`calculation_version`、`snapshot_checksum`。

创建事务必须完成：模板匹配、指标复制、业务职责解析、评分任务生成和快照写入。职责缺失返回 `422 SCORER_RESOLUTION_INCOMPLETE`，`details.missing` 列出 `PM`、`DEPARTMENT_MANAGER`、`CTO`、`PRODUCT_MANAGER` 和对应指标；不得创建空考核。

## Scoring and confirmation gate

沿用 Feature 007 的评分任务接口语义：

- `GET /api/evaluations/:id/scoring-tasks`：返回当前用户有权看到的任务、职责类型、指标、必填状态、来源和完成度。
- `POST /api/scoring-tasks/:task_id/submit`：接受 `score` 0–100、`comment`、可选 `not_applicable` 与理由；任务归属、版本和状态在后端校验。
- `GET /api/evaluations/:id/result`：返回每项综合权重、加权得分、总分、等级、未完成必填项和计算版本。
- `POST /api/evaluations/:id/workflow/advance`：进入确认前必须通过所有必填指标门禁，否则 `409 REQUIRED_SCORES_INCOMPLETE`，不改变状态。

`0` 是有效分数；空值和未提交草稿不算完成；多个有效评分取算术平均。模板未允许的不适用返回 `400 NOT_APPLICABLE_NOT_ALLOWED`。

## HR aggregation and diagnostics

### `GET /api/statistics/performance-summary`

参数：`employee_id`、`period_type`（monthly/quarterly/annual）、`period`、可选 `diagnostics=true`。返回总分、等级、周期权重、投入占比、加权贡献、来源快照哈希和计算版本。投入占比 `<0.20` 的贡献为 0，`>=0.20` 按总分乘投入占比。

### `GET /api/source-workbooks/:id/hr-diagnostics`

返回月度、季度和年度汇总每个公式单元格的主/子 shared 关系、展开公式、缓存值和诊断级别。真实数据行的公式断档返回 `blocking`，明确空白模板行返回 `warning`。没有通过诊断的汇总不可发布或计入年度结果。

## Audit events

以下动作写入既有审计表：`source_workbook_validate`、`source_workbook_import`、`template_publish`、`template_retire`、`evaluation_template_resolved`、`score_submitted`、`evaluation_calculated`、`confirmation_gate_rejected`、`aggregation_diagnosed`、`aggregation_calculated`。审计事件只写来源 ID、规则版本、数量和诊断摘要，不写密码或完整上传内容。
