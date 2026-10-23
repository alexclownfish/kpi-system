# Research: 荭杏研发绩效模板导入与流程适配 v2

## 研究范围

本研究仅基于 `docs/hongxing-workbook-codex-handoff.md` 和 SHA-256 为
`50bcf21db2fb357a5a471ecef459353b974355c2b7191e3f920fa708c3735fb3` 的
`荭杏研发项目绩效考核表 (1).xlsx`。没有修改源表、业务代码或数据库。

## Decision 1: Excel 是最终事实源，Markdown 是索引

**Decision**: 导入器以 Excel 二进制文件的单元格、公式、工作表和来源行作为事实；Markdown 仅用于解释工作表目录、已知冲突和建议实现。

**Rationale**: Markdown 是人工解析交接物，无法保留所有公式、格式和单元格定位。使用 SHA、工作表名和 cell address 可以复现导入结果，且可以在 HR 修订工作簿后识别版本变化。

**Alternatives considered**:

- 直接把 Markdown 当配置：拒绝，因为会丢失 shared formula 和 HR 汇总单元格信息。
- 继续使用旧 catalog：拒绝，旧 catalog 的指标数和来源 SHA 与新工作簿不一致。

## Decision 2: 自动识别工作表类别，不依赖位置

**Finding**: 旧的 `scripts/verify_source_workbook.py` 假设前 9 张表都是岗位模板，因此遇到第一张流程表时以空指标失败（`AssertionError: ('项目绩效考核流程', [])`）。新文件有 21 张表，岗位模板数量为 18。

**Decision**: 先按表头、标题和列结构识别流程表、岗位模板、月度 HR 汇总和季度 HR 汇总，再解析每类表。工作表顺序只作为审计属性，不作为业务判断。

**Rationale**: 工作簿可在后续版本中增删或调整工作表顺序；内容识别才能保证导入稳定。

## Decision 3: 逐项舍入的综合权重是发布门禁

**Decision**: 使用十进制 HALF_UP 计算：

```text
composite_weight = ROUND(dimension_weight * item_weight / 100, 1)
weighted_score = ROUND(composite_weight * score / 100, 2)
total_score = SUM(weighted_score)
```

发布前要求所有有效指标的 `composite_weight` 逐项舍入后合计为 100。遵循 2026-10-01 用户已确认的 A 方案：维度内合计不是 100 只作为告警，不独立阻止发布；不自动归一化、不修改 Excel 原值。等级为 S≥90、A≥80、B≥70、C≥60、D<60。2026-10-16 实际复核原表 18 套模板综合权重均为 100，10 项维度内合计诊断应降为告警。

**Rationale**: 这是工作簿公式的可解释结果，并能保留结构、电子、软件等岗位模板中的原始异常，避免静默改变 HR 意图。

**Alternatives considered**:

- 将每个维度内部权重归一化：拒绝，会改变源表规则且历史不可追溯。
- 使用二进制浮点或 Python 默认 round：拒绝，可能产生银行家舍入差异。

## Decision 4: 模板评分职责优先，流程表用于诊断

**Finding**: 流程表概括为研发普通工程师 PM 50%/部门负责人 30%/PD 20%，但部分 01–04 工程师模板是 PM 60%/部门负责人 20%/PD 20%；05–08 模板是 50%/30%/20%。负责人和产品岗位也有不同职责。

**Decision**: 每一指标的评分人列、评分权重和会签配置以岗位模板为准；流程表只提供顺序、默认会签和冲突诊断。业务职责采用 `PM`、`DEPARTMENT_MANAGER`、`CTO`、`PRODUCT_MANAGER` 类型，不绑定 RBAC 角色名称。

**Rationale**: 岗位模板是可维护、可版本化的实际表格；流程概述无法表达岗位之间的差异。职责配置必须在考核创建时解析为实际用户并保存快照。

**Recommended signoff interpretation**:

- 研发部门负责人：本人 + CTO。
- 普通研发工程师：本人 + PM + PD。
- 产品部门负责人/工程师：本人 + CTO。
- 流程表的“CTO 只需要签字”不追加为工程师必签，除非 HR 在模板显式打开 CTO 会签。

## Decision 5: shared formula 必须按 XML 定义展开

**Finding**: 指标 I 列、HR I/J 列及年度 P/Q/K/L 等区域大量使用 shared formula。主单元格通常包含 `t="shared"`、`si` 和 `ref`，子单元格只有 `t="shared"`、`si`，其 `formula` 为空是合法表示。

**Decision**: 解析器读取 worksheet XML 的 shared formula 主节点和引用范围，使用 A1 相对引用翻译为每个子单元格的显式公式，同时保留 `shared_index`、主单元格和原始 XML 元数据。校验时对主/子公式结构和已缓存值进行对照。

**Rationale**: `openpyxl` 可以读取单元格值，但 `data_only=True` 会丢失公式，直接依赖 `cell.value` 也会把合法 shared 子节点误认为空公式。公式展开是可复现诊断所必需的。

**Alternatives considered**:

- 只读取 `data_only=True` 的缓存结果：拒绝，无法发现公式断档和源规则。
- 运行任意 Excel 宏或公式脚本：拒绝，存在安全风险且违反项目不执行任意公式的约束。

## Decision 6: 文本评分标准先展示，不自动猜分

**Finding**: 工作簿中的大量标准是“延期一天扣 5 分”“超过基线扣分”等自然语言，缺少完整边界、舍入和数据来源接入约定。

**Decision**: 第一阶段保存并展示评分标准、数据来源和原文；评分人提交 0–100 分。只有另立规则并通过 Spec-Kit 评审后，才允许从项目计划、测试或产品数据自动换算。

**Rationale**: 猜测算法会使总分看似自动化但无法审计，也可能与 HR 的人工口径不一致。

## Decision 7: HR 汇总以诊断优先，公式断档阻止发布

**Finding**: 月度和季度 HR 汇总表包含大量 shared formula；季度表的序号 13 行（A17）I17/J17 没有公式或 shared 属性，需区分空行与真实员工行。

**Decision**: 导入器生成 HR 汇总诊断，报告工作表、行号、预期公式、实际公式和严重级别。真实数据行的公式缺失为阻断级别，明确空白模板行可为警告。系统不静默补公式、不凭空生成汇总分。

2026-10-16 复核：第 17 行仅 A17=13，B:H 全空，I17/J17 应为 warning。判行依据 B:H 的真实输入；数字 0 算输入，空白字符串不算，A 列序号不作为数据证据。无序号但存在业务输入仍为真实数据行。

**Rationale**: 汇总是财务/绩效结果，缺公式时继续发布会产生不可解释的年度结果。

## Decision 8: 岗位匹配与历史冻结

**Decision**: 新考核按 `(department, position_type, period)` 匹配唯一已发布模板；岗位名不依赖固定系统角色。创建事务中复制模板、指标、实际评分人、会签、导出布局、来源 SHA 和计算版本。模板、组织关系或 RBAC 变化后，已有考核仍使用快照。

**Rationale**: 组织角色是可维护数据，且曾出现 CTO、普通员工和子部门负责人无法进入评分人下拉的缺陷；快照可阻止历史结果漂移。

## Decision 9: 失败必须原子且可见

**Decision**: 解析、诊断和导入分为只读预览与显式提交。任一阻断错误、重复键或职责无法解析都使提交事务整体失败；API 返回稳定错误码、具体来源和 request id，页面清空残缺数据并提供重试。

**Rationale**: 项目宪章要求后端强制授权、数据范围隔离和可操作反馈，不能把部分导入伪装为成功。

## 未解决但已采用的边界

- Excel 仍是业务输入，不能保证文本评分标准足以自动计算每个指标；本 Feature 明确保留人工评分。
- 部门负责人、CTO、PM、PD 的实际用户必须由现有组织关系、评分职责配置或 HR 明确绑定解析；不能通过创建演示账号解决缺失。
- 旧考核可继续走兼容流程；迁移只处理确定性快照，疑似冲突记录进入人工复核，不自动改写结果。
