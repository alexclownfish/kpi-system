# Research
- Decision: 明确部门leader_id；原列表空是父负责人仅来自评分职责，角色不代表业务职责。Rationale: HR只需维护一处，支持自定义部门。Rejected: RBAC角色/岗位关键词自动推断，会多人歧义和误派。
- Decision: 明确负责人优先、未设置才兼容旧职责。Rationale: 保持旧配置，新的组织定义可解释；失效负责人阻塞避免悄悄改派。
- Decision: 显示上级列与负责人列，过滤后代父候选。Rationale: 原上级显示在部门名称下不明显；后端已有环校验。
- No unresolved technical unknowns; local code inspected, no research delegation required.
