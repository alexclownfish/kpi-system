# Implementation Plan: 用户角色与权限管理

**Branch**: `main` | **Date**: 2026-09-17 | **Spec**: [spec.md](spec.md)

## Summary

在现有数据驱动 RBAC 基础上补齐自定义角色 CRUD、权限与单一数据范围配置、角色复制、用户分配和审计页面。继续使用现有 `Role`、`Permission`、`UserRole`、`RolePermission` 与 `RoleDataScope`，只增加角色组织属性及必要约束；同时清除业务流程对固定角色字符串的依赖，使自定义角色真正按权限与数据范围工作。

## Technical Context

**Language/Version**: Go 1.23；TypeScript、React 19、Next.js 15
**Primary Dependencies**: Gin、GORM、SQLite；现有 shadcn/ui 组件与 Axios API 层
**Storage**: 现有 SQLite，使用兼容 AutoMigrate 和幂等数据修复
**Testing**: Go handler/model tests；TypeScript 检查；Next production build；Docker Compose 冒烟测试
**Target Platform**: Docker Compose 部署的 Web 应用
**Project Type**: Next.js 前端 + Go API
**Performance Goals**: 100 个角色、单角色 1000 个绑定用户时保持常规交互可用
**Constraints**: 保留现有数据与卷；系统角色不可变；权限默认拒绝；一个用户一个主角色
**Scale/Scope**: MVP 角色少于 100，权限目录约 50–100 项

## Constitution Check

- 后端接口使用 `role:*` 或 `employee:assign_role` 强制授权，前端只控制展示。
- 角色绑定用户明细通过员工数据范围过滤。
- 创建、更新、复制、删除、分配均返回稳定反馈并记录审计。
- 覆盖越权授权、系统角色保护、已使用角色删除和最后超级管理员保护。
- 不删除现有表和卷；迁移采用兼容 AutoMigrate 和幂等修复。
- 仅修改正式代码目录并复用现有 RBAC 表。

所有门禁通过，无需宪章例外。

## Architecture Decisions

### 1. 用户只有一个主角色

保留 `user_roles` 关系表，但通过服务层和数据库约束确保每个用户只有一个有效绑定。角色更换使用事务删除旧绑定并创建新绑定。MVP 不合并多角色权限，避免权限与数据范围取并集导致不可预测的越权。

### 2. 权限与数据范围分别建模

- `role_permissions`: 角色可执行什么操作。
- `role_data_scopes`: 角色能看到哪些业务对象，MVP 恰好一条。
- 授权判断继续由 `UserHasPermission` 和 `DataScopeForUser` 负责。
- 页面权限矩阵只组合已有权限目录，不允许创建权限代码。

### 3. 系统角色只读，自定义角色可维护

`Role.IsSystem=true` 的角色不可编辑和删除，可复制为 `IsSystem=false` 的角色。自定义角色代码由服务端生成，例如 `custom_<随机标识>`，避免名称改动影响稳定标识。

### 4. 权限提升防护

角色管理员只能授予自己拥有的权限，且目标数据范围不能宽于自身范围。系统角色保护和“最后一个超级管理员”检查独立执行，不能仅依赖按钮隐藏。

### 5. 清除固定角色名称分支

- 前端 `isHR/isManager` 判断替换为 `hasPermission(...)`、`data_scope` 和组织关系。
- 后端邀请、待处理数量、员工筛选等固定 `hr/manager` 分支改为权限/数据范围判断。
- `Employee.Role` 暂保留为主角色代码兼容字段，但 `user_roles` 为授权来源；认证响应增加角色名称与角色对象。

### 6. 角色的组织属性

新增 `RequiresManager` 表达该角色用户在创建/编辑时是否必须选择直属上级。它只负责组织校验，不参与权限判断。

## Project Structure

```text
specs/004-custom-role-management/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/roles-api.md
└── checklists/requirements.md

server/
├── models/models.go
├── models/database.go
├── handlers/role.go
├── handlers/permissions.go
├── handlers/employee.go
├── handlers/auth.go
├── handlers/invitation.go
├── handlers/kpi.go
├── handlers/role_test.go
└── routes/routes.go

app/roles/page.tsx
app/employees/page.tsx
components/sidebar.tsx
components/employee-selector.tsx
lib/api.ts
lib/auth-context.tsx
lib/access-control.ts
```

**Structure Decision**: 沿用当前前后端目录，不引入第二套权限服务或新的应用包。

## Delivery Slices

1. P1 角色 CRUD：列表、创建、复制、编辑自定义角色、系统角色保护。
2. P1 用户分配：单用户/批量绑定、最后超级管理员保护、员工页面接入动态角色。
3. P1 去硬编码：权限/数据范围替换固定角色分支，确保自定义角色可实际工作。
4. P2 审计与影响查看：绑定人数、用户明细、审计记录与删除保护。

## Post-Design Constitution Check

设计未引入角色名授权、前端授权或多角色范围合并；权限变更和高风险操作均有后端校验、事务和审计。通过。
