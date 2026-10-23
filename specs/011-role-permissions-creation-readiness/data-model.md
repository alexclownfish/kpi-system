# Data Model

## Role

- 既有 id/code/name/is_system/requires_manager 保留。
- access_customized: bool，default false，内部状态不接受外部赋值。
- 成功更新同一事务将该标记设为 true 并替换 permission / data_scope 关联。
- 非超级管理员定制后，重启种子不再添加任何默认授权和默认范围。
- super_admin 禁止更新；仍由默认目录保障恢复入口。

## Preview

- workflow_mode: disabled | audit | enabled。
- 原 ready、blocking_errors、template、scorers 不变。
- mode 是状态展示，不是权限，也不是可编辑设置。

## State Transitions

- 默认系统角色 -> 成功保存 -> 定制角色；失败不改变任何授权。
- 创建准备 blocked -> 显式配置流程及职责 -> ready -> 创建 snapshot/tasks。
- 历史考核与评分人不重新解析或变更。
