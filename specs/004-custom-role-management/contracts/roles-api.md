# Roles API Contract

所有接口均需认证，错误响应使用 `{ "error": "可操作的中文提示" }`。

## GET `/api/roles`

权限：`role:view` 或 `employee:assign_role`。查询支持 `search`, `type`, `page`, `pageSize`。返回权限、数据范围、绑定人数、系统角色标志和是否可分配。

## GET `/api/roles/:id`

权限：`role:view`。返回完整角色、权限集合、数据范围和受数据范围过滤的绑定用户摘要。

## POST `/api/roles`

权限：`role:create`。

```json
{
  "name": "绩效复核员",
  "description": "复核本部门最终绩效",
  "data_scope_code": "DEPARTMENT",
  "requires_manager": true,
  "permission_ids": [1, 8, 12]
}
```

成功返回 201；重复名称、非法权限依赖、越权授权返回 400/403。

## POST `/api/roles/:id/clone`

权限：`role:create`。请求提供新名称和可选说明，复制权限、数据范围及组织属性。

## PUT `/api/roles/:id`

权限：`role:edit`。请求结构同创建；系统角色返回 403；权限和数据范围在事务中整体替换。

## DELETE `/api/roles/:id`

权限：`role:delete`。系统角色或仍有绑定用户返回 409。

## GET `/api/roles/:id/users`

权限：`role:view`。支持分页和搜索，并应用调用者员工数据范围。

## PUT `/api/employees/:id/roles`

权限：`employee:assign_role`。继续接受 `{ "role_code": "..." }`，增加授权上限和最后超级管理员保护。

## PUT `/api/roles/:id/users`

权限：`employee:assign_role`。请求 `{ "user_ids": [12, 13, 14] }`。全部用户校验通过后一次事务提交。
