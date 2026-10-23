# Research

- Decision: 非超级管理员系统角色可维护授权，但不能更改稳定属性。
  Rationale: 角色 code 参与兼容业务逻辑；用户只需单独修改权限。
  Alternatives: 复制角色仍需要重新分配用户；解除全部系统身份限制风险过高。
- Decision: 使用 Role.access_customized 持久化种子跳过标记。
  Rationale: 当前启动 FirstOrCreate 每条默认权限会恢复被删除权限；空集不能用记录数量判断是否定制。
  Alternatives: 所有角色只初始化一次会妨碍未定制角色获得后续必要目录升级；每权限删除标记过于复杂。
- Decision: 更新前验证已有目标角色完整权限与范围，禁止低权限管理员通过缩减提交篡改高权限角色；请求仍严格校验授予上限。
  Rationale: 只检查新权限不能保护既有高权限角色。
- Decision: 缺评分职责不依靠角色名称自动创建职责；模式不默默启用。
  Rationale: 一个角色可能多人，公司/部门/岗位范围决定实际评分对象。截图不是授权指定真实评分人。
- Evidence: role.go 的 IsSystem 拒绝；roles/page.tsx 隐藏编辑；database.go 每启动补默认角色权限/范围；预览与创建的硬阻塞已有回归。
