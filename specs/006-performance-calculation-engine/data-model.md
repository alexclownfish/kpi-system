# Data Model: 绩效计算引擎与模板维护

## Template

现有 `KPITemplate` 扩展为可发布、可版本化的模板。

- `id`
- `name`
- `department_id`
- `position_type`
- `period`
- `version`
- `status`（draft/published/retired）
- `calculation_version`
- `is_active`

唯一匹配键为部门、岗位类型、周期和已发布状态。

## Dimension

新增 `KPIDimension`：

- `id`
- `template_id`
- `name`
- `weight`（0–100）
- `sort_order`

一个模板包含多个维度；维度合计仅供展示，保存和发布以逐项舍入后的综合权重合计 100% 为准，不归一化。

## Item

扩展现有 `KPIItem`：

- `id`
- `template_id`
- `dimension_id`
- `name`
- `description`
- `item_weight`（维度内 0–100）
- `composite_weight`（由服务端计算）
- `score_min`（默认 0）
- `score_max`（默认 100）
- `scoring_guidance`
- `data_source`
- `scorer_types_json`
- `required`
- `sort_order`

计算：`composite_weight = ROUND(dimension.weight * item.item_weight / 100, 1)`（十进制 HALF_UP）。所有指标综合权重合计必须为 100；维度内权重合计不要求为 100。权重须为 0–100 的有限数值，每个维度至少包含一个指标。

## Evaluation Snapshot

扩展 `KPIEvaluation` 或新增快照表保存：

- 模板版本
- 计算版本
- 维度 JSON 快照
- 指标 JSON 快照
- 评分人配置快照
- 投入占比规则版本

快照创建后只读。

## Score Assignment

建议新增评分任务实体：

- `evaluation_id`
- `item_id`
- `scorer_user_id`
- `scorer_type`
- `status`
- `submitted_at`

## Item Score

建议将现有固定评分列扩展为评分记录：

- `evaluation_id`
- `item_id`
- `scorer_user_id`
- `score`（0–100 或空）
- `comment`
- `source_data`
- `version`

同一指标的有效评分取算术平均，之后再计算指标加权分。

## Aggregation

项目汇总保存：

- 员工
- 项目
- 周期
- `total_score`
- `grade`
- `allocation_ratio`
- `weighted_contribution`
- `aggregation_version`

`allocation_ratio < 0.2` 时 `weighted_contribution = 0`；否则为 `total_score * allocation_ratio`。
