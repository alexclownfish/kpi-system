# Research: 统一绩效结果导出

## Decision 1: 导出版式归属于考核模板

**Decision**: 岗位只决定推荐或实际选择的考核模板，导出字段和签字栏由考核模板决定。

**Rationale**: 同一岗位可能在不同周期使用不同考核模板，同一模板也可能服务多个岗位。把版式直接绑定岗位会复制规则并产生优先级冲突。

**Alternatives considered**: 直接绑定岗位会产生双重来源；导出时手工选择版式无法确保批量一致和审计可复现。

## Decision 2: MVP 使用受控 JSON 值对象

**Decision**: 在模板上保存带 `version` 的导出布局 JSON，由后端规范化和校验。

**Rationale**: 当前每个模板只需要一个有效版式。独立布局表、共享继承和复杂版本管理会扩大 MVP 范围。

**Alternatives considered**: 独立布局表适合未来跨模板复用；任意 HTML/Excel 模板安全风险高且难以跨格式一致。

## Decision 3: 结果定稿时冻结布局

**Decision**: 结果快照 payload 包含规范化后的布局副本；已有快照优先使用该副本。

**Rationale**: 模板后续修改不能改变已签字或待签字材料。布局参与快照校验码后，数据和版式共同可追溯。

**Alternatives considered**: 每次读取模板当前配置无法复现历史；单独布局版本表对当前范围过重。

## Decision 4: Excel 与 PDF 共用文档语义模型

**Decision**: 先由快照和布局生成统一的基本信息、动态列、明细行、总结和签字栏模型，再由两个 renderer 输出文件。

**Rationale**: 共用模型保证字段和值一致，同时允许 Excel 列宽与 PDF 分页采用各自排版能力。

**Alternatives considered**: 用 HTML 转 Excel 会损失可编辑性；保留两套字段拼接正是当前不一致的根因。

## Decision 5: 预设与兼容默认

**Decision**: 提供 `final_signoff` 和 `full_process` 两个预设；空配置、无效旧配置和不含布局的旧快照回退 `final_signoff`。

**Rationale**: 让现有数据零迁移可用，也为管理员提供明确起点。

**Alternatives considered**: 启动时批量写回模板会制造不必要生产数据变更。
