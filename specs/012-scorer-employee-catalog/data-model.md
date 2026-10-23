# Data Model

无 schema 变更。

- Employee：既有 id/name/email/is_active 等，稳定集合按 id 升序分页。
- Candidate catalog：Employee[]，操作者可见范围；页面只传 is_active=true，无 role 条件。
- UI：loading 开始清空 items/departments/employees；全部成功填入；任意失败记录 loadError；finally 结束 loading。
- 操作约束：loading 或 loadError 禁止新增、编辑、选择、保存；空 employees 禁止评分人选择和保存。save 函数也检查门禁。
- 编辑中选中员工被停用时不静默替换人员，后端保存校验为最终边界。
