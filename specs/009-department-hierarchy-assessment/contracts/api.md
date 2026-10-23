# API Contracts
- POST/PUT /api/departments[/:id]：保留name/description，新增parent_id null表示根，省略更新保持原值；400无效父/环，404缺失，403权限拒绝。
- GET /api/departments/:id/manager-candidates?position=...&employee_id=...：department:view/employee:create/employee:edit其一，员工范围过滤；data安全Employee列表；400参数/循环，404部门，500查询错误。
- GET /api/templates/match-preview：原员工参数；kpi:view并校验员工数据范围；200 data含template/scorers/ready/blocking_errors，400参数，404无模板，403超范围，500读取异常。
- POST /api/evaluations：原请求；岗位等价匹配一致，合法上级职责解析；weighted-v2模板未enabled时409 WORKFLOW_WRITES_DISABLED，预览ready=false且说明开关，禁止生成混合新旧数据；旧v1兼容路径保持。
- GET部门列表：新增parent_id，敏感员工字段不暴露。
- PUT /api/employees/:id：未提交字段保持原值；manager_id显式null清空（要求上级的角色仍拒绝）；仅改上级不能清空姓名/邮箱/岗位/在职状态。
