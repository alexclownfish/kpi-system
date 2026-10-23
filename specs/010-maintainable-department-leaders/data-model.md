# Data Model
- Department.leader_id: nullable indexed employee ID，必须本部门在职；更新省略保持/null清空。
- Department.parent_id: 沿用009，可空，不能自身/子孙。
- 明确负责人身份不绑定RBAC；历史快照/任务不更新。
- 调岗/停用后保留leader_id，但解析报失效，不自动换人。
