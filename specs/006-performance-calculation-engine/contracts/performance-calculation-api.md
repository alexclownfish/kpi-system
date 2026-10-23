# Performance Calculation API Contract

## Template

- `GET /api/kpi/templates`：按部门、岗位类型、周期查询模板
- `POST /api/kpi/templates`：创建草稿模板
- `PUT /api/kpi/templates/:id`：编辑草稿或复制生成新版本
- `POST /api/kpi/templates/:id/publish`：校验权重后发布
- `POST /api/kpi/templates/:id/retire`：停用模板

发布失败必须返回具体权重或评分人配置错误，不得部分写入。

## Evaluation

- 创建考核时返回匹配的模板版本和快照摘要
- `GET /api/evaluations/:id/scoring-progress`：返回必填项完成情况
- `POST /api/evaluations/:id/submit-confirmation`：仅在所有必填指标有效评分完成后允许提交确认
- `GET /api/evaluations/:id/calculation`：返回维度、指标、综合权重、加权分、总分和等级

所有接口必须执行现有权限和数据范围校验。

## Aggregation

- `GET /api/statistics/performance-summary`：按月、季度、年度查询分数、等级、投入占比和加权贡献

返回中同时包含数值分数、等级和计算版本。
