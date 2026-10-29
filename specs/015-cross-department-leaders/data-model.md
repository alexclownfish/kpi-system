# Data Model
无新增表或列。Department.leader_id nullable，引用在职 Employee；多个部门可引用同一员工。Employee.department_id 为单一归属，manager_id 为显式直属上级。Department.Leader 仅关联读取，用于跨部门姓名显示。
DEPARTMENT集合=本人所属部门 UNION 本人担任leader的部门，要求本人在职；TREE在此基础递归子部门。SELF/ASSIGNED/直属下属保持现有定义。
更换leader不写employee/roles/user_roles/任务/快照，返回previous_leader_direct_reports（本部门上级仍为旧leader的在职员工数）。

EvaluationScoringTask 唯一约束从 (evaluation_id,item_id,scorer_user_id) 升级为 (evaluation_id,item_id,scorer_user_id,scorer_type)。新索引先创建成功，再在事务内移除旧索引；两种数据库均验证幂等、旧分数/状态不变及同职责重复拒绝。迁移不改历史任务或快照，不为旧考核补造之前被去重的任务。新考核同一人两个职责生成两个任务，按原模板平均/权重规则计分。
