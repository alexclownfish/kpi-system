# Specification Quality Checklist: 荭杏研发绩效模板导入与流程适配 v2

**Purpose**: 在进入实现前验证源表事实、评分流程、数据安全、历史冻结和页面验收要求是否完整、清晰、可测试。

**Created**: 2026-10-14

**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] 未将框架、文件实现或具体代码方案写入用户价值要求；技术方案集中在 `plan.md`。[Spec §Scope and source precedence]
- [x] 明确 Excel 是最终事实源、Markdown 是解析索引，并记录了 SHA 和来源范围。[Spec §Scope and source precedence]
- [x] 面向 HR、评分人、管理员和验收人员描述用户价值。[Spec §User Scenarios & Testing]
- [x] 已完成用户故事、边界、功能要求、实体、成功标准和假设。[Spec §Requirements]

## Requirement Completeness

- [x] 覆盖了 21 张工作表、18 套岗位模板、137 个指标和 9 个源部门的可验证结果。[Spec FR-002/FR-003, SC-001]
- [x] 覆盖流程表在首位、岗位顺序变化、算法工程师空部门映射和重名歧义。[Spec §Edge Cases]
- [x] 覆盖 shared formula 主/子节点、数组公式、公式断档和 HR 汇总诊断。[Spec FR-004/FR-015]
- [x] 覆盖模板实际权重、评分职责、会签职责、必填指标、不适用和 0 分。[Spec FR-005–FR-009]
- [x] 覆盖岗位自动匹配、实际评分人解析、缺失职责原子拒绝和历史快照。[Spec FR-010–FR-012]
- [x] 覆盖月度/季度/年度汇总、投入占比阈值、等级和导出历史稳定性。[Spec FR-014, User Story 5]

## Requirement Clarity

- [x] 综合权重、加权得分、总分和等级公式及舍入方式明确。[Spec FR-006]
- [x] “维度内权重异常”与“综合权重发布门禁”分开定义，禁止静默归一化。[Spec FR-008]
- [x] PM、部门负责人、CTO、产品经理被定义为业务职责类型，与 RBAC 角色解耦。[Spec FR-009]
- [x] 研发负责人、普通工程师和产品岗位的默认会签职责明确，模板显式配置优先。[Spec §Scope and source precedence]
- [x] 文本评分标准的自动化边界明确为首期展示并人工录分，不猜测分数。[Spec §Assumptions]

## Requirement Consistency

- [x] Excel 模板权重优先于流程表概括权重，冲突会生成诊断，消除了两套权重同时生效的歧义。[Spec FR-005, Research Decision 4]
- [x] 模板发布异常、评分完成门禁和 HR 公式断档均定义为可阻断状态，不会部分成功。[Spec FR-008, FR-013, FR-015]
- [x] 新模板版本与历史考核快照隔离，模板维护不会重算历史结果。[Spec FR-012, FR-019]
- [x] 旧考核、旧导出和已完成考核补充签字材料的兼容边界已写明。[Spec FR-018, §Assumptions]

## Acceptance Criteria Quality

- [x] 成功标准可用计数、哈希、公式对照、发布结果、状态门禁和导出校验码验证。[Spec §Success Criteria]
- [x] 关键失败场景要求稳定错误码、request_id、来源单元格和可操作重试。[Spec FR-016, FR-020]
- [x] 自动化、页面和 Docker 健康检查步骤均在 `quickstart.md` 中可重复执行。[quickstart.md §5]

## Scope, Dependencies & Assumptions

- [x] Scope 明确不执行宏/任意公式、不自动猜分、不清空数据库、不凭空创建员工。[Spec FR-018, §Assumptions]
- [x] 依赖既有权限、数据范围、评分任务、结果导出和签字补充模型，并记录兼容迁移边界。[plan.md §Constitution Check, data-model.md]
- [x] Docker 构建由用户执行，验收使用私有测试数据和唯一前缀，清理规则明确。[quickstart.md §6]

## Notes

- 本清单确认的是需求质量，不代表业务代码已经实现。
- 下一步应按 `tasks.md` 从源表解析回归开始，先修复当前解析器对前 9 张表的错误假设，再进入 API 和页面实现。
