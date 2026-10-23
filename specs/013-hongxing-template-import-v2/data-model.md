# Data Model: 荭杏研发绩效模板导入与流程适配 v2

本设计复用现有部门、岗位、模板、考核、评分任务、审计和汇总实体；新增字段/实体必须通过兼容迁移，不能清理现有 SQLite 数据卷。所有源字段同时保存规范化值和可审计的 raw 值。

## SourceWorkbook

表示一次不可变的 Excel 输入版本。

- `id`
- `source_name`
- `source_sha256`（唯一）
- `parser_version`
- `uploaded_by`
- `created_at`
- `status`：`validated`、`imported`、`rejected`
- `sheet_count`
- `source_metadata_json`（源路径、文件大小、工作簿日期等非敏感元数据）
- 不可变解析清单（按上传 SHA 保存实际工作表、指标、权重、职责、公式和原始来源；不向客户端返回内部存储信息）

约束：相同 SHA 不重复创建来源版本；重新校验可刷新当前诊断，但不得覆盖已导入模板或历史快照。修订文件以新 SHA 保存独立解析清单；导入可复用同来源已有模板并返回实际数量。

## SourceSheet

- `id`
- `workbook_id`
- `name`
- `kind`：`process`、`position_template`、`hr_monthly`、`hr_quarterly`、`unknown`
- `sheet_order`
- `source_range`
- `diagnostic_status`
- `metadata_json`

约束：同一工作簿内工作表名称唯一；分类依据内容识别，顺序只用于审计。

## DepartmentMapping

保存源部门到系统组织的确定性映射。

- `workbook_id`
- `source_label`
- `normalized_label`
- `department_id`
- `mapping_reason`：`exact`、`same_group_fallback`、`explicit_hr_mapping`
- `raw_value`
- `is_ambiguous`

`05-算法开发-工程师` 的空部门单元格使用 `same_group_fallback` 映射到“算法开发部”，`raw_value` 保持空；若多个系统部门可匹配，`is_ambiguous=true` 并阻止导入。

## PositionTemplate

对应一套部门、岗位和周期规则。

- `id`
- `workbook_id`
- `source_sheet_id`
- `department_id`
- `position_type`：`DEPARTMENT_MANAGER`、`ENGINEER` 或产品岗位扩展类型
- `period`：`monthly`（源表均为月度）
- `version`
- `status`：`draft`、`published`、`retired`
- `source_template_name`
- `source_sheet_hash`
- `calculation_version`
- `export_layout_version`
- `published_at`
- `published_by`

唯一匹配键为 `(department_id, position_type, period, status=published)`；历史版本不覆盖。

## Dimension

- `id`
- `template_id`
- `name`
- `weight`（维度占比，0–100）
- `raw_weight`
- `sort_order`
- `source_cell`
- `inner_weight_sum`
- `diagnostic_status`

`inner_weight_sum` 只用于诊断和显示；不自动归一化。综合权重校验以指标逐项舍入结果为准。

## Indicator

- `id`
- `template_id`
- `dimension_id`
- `name`
- `item_weight`（维度内权重，0–100）
- `raw_item_weight`
- `composite_weight`（服务端 HALF_UP 计算值）
- `score_min`、`score_max`（默认 0、100）
- `scoring_guidance`
- `data_source`
- `required`
- `not_applicable_allowed`
- `not_applicable_requires_reason`
- `sort_order`
- `source_row`
- `source_cells_json`
- `source_formula_json`
- `diagnostic_status`

计算：`composite_weight = ROUND(dimension.weight * item.item_weight / 100, 1)`；所有有效指标的合计必须为 100 才能发布。

## ScorerRule

保存模板级或指标级业务评分职责。

- `id`
- `template_id`
- `indicator_id`（可为空表示模板默认）
- `scorer_type`：`PM`、`DEPARTMENT_MANAGER`、`CTO`、`PRODUCT_MANAGER`
- `weight`（来源 F/G 解析出的职责权重；如源表只在指标行表达则保留指标绑定）
- `sequence`
- `resolution_strategy`：`assigned_responsibility`、`department_leader`、`direct_manager`、`explicit_user`
- `source_label`
- `source_cell`
- `required`

评分职责不是 RBAC 角色。创建考核时解析出的实际员工 ID 复制到快照，不依赖之后的角色名称。

## SignoffRule

- `template_id`
- `signer_type`：`SUBJECT`、`PM`、`DEPARTMENT_MANAGER`、`CTO`、`PRODUCT_MANAGER`
- `required`
- `sequence`
- `source`：`template`、`process_default`、`hr_override`
- `source_cell`

推荐默认：负责人 `SUBJECT+CTO`；普通研发工程师 `SUBJECT+PM+PRODUCT_MANAGER`；产品岗位 `SUBJECT+CTO`。模板显式值优先。

## FormulaCell

公式解析审计实体，可内嵌 `source_formula_json`，也可独立存储以支持 HR 汇总。

- `workbook_id`
- `sheet_id`
- `coordinate`
- `formula_type`：`normal`、`shared_master`、`shared_child`、`array`、`missing`
- `shared_index`
- `shared_master_coordinate`
- `shared_ref`
- `raw_formula`
- `expanded_formula`
- `cached_value`
- `diagnostic_status`

shared 子节点没有公式文本时，必须通过 master 的 `ref` 和相对引用展开；不得以空值覆盖 `expanded_formula`。

## ImportDiagnostic

- `id`
- `workbook_id`
- `sheet_id`
- `severity`：`info`、`warning`、`blocking`
- `code`
- `coordinate`
- `raw_value`
- `normalized_value`
- `message`
- `repair_hint`
- `created_at`

阻断诊断包括：未识别岗位、重复匹配键、歧义部门、必填评分职责缺失、综合权重不为 100、真实 HR 数据行公式断档和事务一致性错误。

维度内合计不为 100 仅为 warning。HR 真实数据行由 B:H 业务输入识别（0 有效，空白字符串无效）；仅 A 列序号的空白行缺公式保留 warning 和来源坐标。

## HRAggregationRule

- `workbook_id`
- `sheet_id`
- `period_type`：`monthly`、`quarterly`、`annual`
- `field_name`
- `formula_cell`
- `expanded_formula`
- `allocation_threshold`（默认 0.20）
- `diagnostic_status`
- `source_row`

业务汇总保存 `total_score`、`grade`、`allocation_ratio`、`weighted_contribution` 和 `aggregation_version`。占比低于 0.20 时贡献为 0，达到阈值按 `total_score × allocation_ratio`。

## EvaluationSnapshot

创建考核时一次性复制以下数据并只读：

- `source_workbook_sha256`
- `template_id`、`template_version`
- `department_id`、`position_type`、`period`
- 维度、指标、综合权重、评分标准和数据来源 JSON
- 业务评分职责、解析到的用户 ID/名称/邮箱及解析时间
- 会签规则、导出布局、计算版本和汇总阈值
- `snapshot_checksum`

已有考核永远读取此快照；模板、组织部门、用户角色、职责配置和源工作簿后续变化只能影响新建考核。

### Evaluation workflow fields

- `project_name`：本次考核对应的项目名称，支持同员工同周期多项目。
- `allocation_ratio`：项目投入占比，范围 0–1；低于 20% 时加权贡献为 0。
- `work_summary`、`work_risks`、`support_needed`、`next_plan`：员工提交的工作总结与后续计划。
- `work_summary_submitted_at`：总结提交时间；新流程在自评前必须存在。
- `EvaluationScoringTask.sequence`：Excel 流程顺位的服务端快照，PM=1、负责人/CTO=2、产品经理=3。

## State transitions

```text
uploaded -> validated -> imported(draft)
                    \-> rejected
draft -> published -> retired
evaluation created -> scoring -> pending_confirm -> completed
```

- `validated` 只读，不写业务模板。
- `imported(draft)` 允许 HR 修正异常，但不能用于新建考核。
- `published` 的综合权重、评分职责和会签规则不可破坏性修改；修改通过复制新版本完成。
- 同一 `(department_id, position_type, period)` 只允许一个 active published 版本；发布新 revision 时旧 active 版本在同一事务内转为 retired，历史模板和历史考核快照继续保留。
- 页面默认隐藏 retired 版本；物理删除仅允许没有历史考核引用的版本，引用检查由后端执行。
- `pending_confirm` 前必须通过所有必填指标门禁。
- `completed` 的总分、等级、快照和原确认记录只读；纸质材料可通过既有补充签字记录追加，不改变状态。
