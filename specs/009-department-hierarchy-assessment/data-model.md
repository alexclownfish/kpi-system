# Data Model
- Department.parent_id：nullable uint，引用现有Department，不能自己/后代，删除父部门有子部门时拒绝。
- BusinessInitialization.key：新增唯一 hongxing-hierarchy-v1，事务完成后记录，重复跳过。
- Employee.position/manager_id：保留现有字段，目录等价岗位仅匹配时转换；manager不能自己/后代，负责人跨部门必须祖先负责人职责或公司CTO。
- Preview：template，scorers兼容map；ready boolean，blocking_errors string[]。缺少职责不删除已匹配模板。
- Historical evaluations/snapshots/tasks：无修改、无迁移。
