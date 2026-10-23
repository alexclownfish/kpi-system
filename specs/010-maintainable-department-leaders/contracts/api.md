# API Contracts
- GET /api/departments及/:id：新增leader_id，沿用安全employees列表；无密码字段。
- POST /api/departments：新增leader_id null；非空400“请先创建部门并添加员工，再设置部门负责人”。
- PUT /api/departments/:id：department:edit；leader_id省略保持/null清空；非法/停用/非本部门400，不可见403；审计同时记录parent_id/leader_id。
- GET /api/departments/:id/manager-candidates：既有授权和范围不变；部门负责人候选认可祖先部门明确负责人，不要求重复职责；无合法人选仍为空，可返回message说明负责人配置缺失或失效。前端员工页展示该说明。
- PUT /api/employees/:id 与创建考核：使用相同上级有效性；模板部门负责人评分认可明确leader_id，失效则阻塞，不自评分。
