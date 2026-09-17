# Data Model: 用户角色与权限管理

## Role

现有字段：`id`, `code`, `name`, `description`, `is_system`, timestamps。

新增/强化：

- `requires_manager`: 是否要求该角色用户设置直属上级，默认 true。
- `code`: 服务端生成且不可修改；自定义角色使用随机稳定代码。
- `name`: 去空格后 2–50 字，大小写不敏感唯一。
- `is_system`: 系统角色保护标志，创建后不可修改。

派生字段：`permission_count`, `user_count`, `data_scope`, `assignable`。

## Permission

继续由代码维护稳定目录。角色页面按 `resource` 分组展示，不允许管理端新增或改写权限代码。

## RolePermission

更新角色时在事务中替换全部绑定，并验证查看类基础权限依赖。

## RoleDataScope

MVP 每个角色恰好一条，可选 `SELF`、`DIRECT_SUBORDINATES`、`DEPARTMENT`、`ASSIGNED`、`ALL`。`DEPARTMENT_TREE` 暂不开放。

## UserRole

业务约束为 `user_id` 唯一。历史存在多条时优先保留当前 `Employee.Role` 对应角色，否则保留创建时间最早的一条并记录修复日志。

## AuditLog

详情至少记录角色名称、权限增删、数据范围变化、受影响用户数量或目标用户。

## State and Delete Rules

- 系统角色：只读，可复制。
- 自定义角色：可编辑；用户数为 0 时可删除。
- 超级管理员：至少保留一个有效用户绑定。
