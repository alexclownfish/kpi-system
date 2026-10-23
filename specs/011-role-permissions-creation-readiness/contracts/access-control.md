# Contracts

## PUT /api/roles/:id

既有 RoleMutationRequest：name、description、data_scope_code、requires_manager、permission_ids。
需要 role:edit；更新前校验目标角色现有权限和范围在操作者上限内，再校验新授权。
普通系统角色可修改 description、权限、范围；name/requires_manager 改动返回400。
super_admin 返回403；其他系统角色不可删除（原DELETE不变）。
成功200 {message,data}；原用户绑定不变；成功与拒绝写 audit。
权限与范围及 access_customized 更新同一事务，异常无部分结果。

## GET /api/templates/match-preview?employee_id=:id&period=monthly

实际路由沿用现有 template preview；返回 data 新增 workflow_mode。
ready=false 时模板与已解析评分人保留，blocking_errors 标出环境开关或缺失职责。
权限与范围沿用现有校验，模板匹配失败404，超范围403。
不允许请求修改环境、角色、评分职责或历史考核。
