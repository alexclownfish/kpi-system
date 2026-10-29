# API Contract
GET /api/departments/leader-candidates?page=1&pageSize=100&search=...
需要 department:create 或 department:edit；返回分页 {data:[{id,name,email,position,department_id,is_active}],total,totalPages,...}，仅本人员工数据范围内在职人员，不返回密码。
POST /api/departments 支持 leader_id 指定已有在职候选；授权范围内任命。PUT /api/departments/:id 支持替换或null；目标部门与被选员工均须在操作者管理范围；401/403权限，400无效/停用，404部门不存在。
POST成功保留data；PUT成功另加 previous_leader_direct_reports 数量，由页面显示交接提醒；旧关联保留并审计旧/新leader。
GET /api/departments 和 /:id 返回 leader 摘要（id,name,position,is_active）用于跨部门姓名，不改变employees成员集合。本人所属部门或管理范围内部门可展示负责人摘要；其他部门不展示负责人摘要。
GET /api/departments/:id/manager-candidates 保持原权限，允许本部门人员及明确指定的跨部门负责人，候选用户最小字段，禁止任意外部人员。
