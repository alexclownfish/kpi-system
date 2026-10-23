# Implementation Plan: 可维护部门层级与负责人

**Date**: 2026-10-01 | **Spec**: [spec.md](spec.md)

## Summary
复用009层级维护，增加nullable Department.leader_id作为明确业务负责人。部门管理编辑上级/负责人，列表独立列；候选/保存/评分统一引用部门负责人，旧职责仅在未明确配置时兼容。保持历史快照、RBAC和部署开关不变。

## Technical Context
Go1.23/Gin/GORM/SQLite，Next15/React19/TypeScript，无新依赖。现有nullable迁移和分页目录；前端以当前部门已加载员工填负责人候选，不新增全员接口。测试为Go、unit、TS/lint/build、隔离页面验证。普通Compose用户自行构建。

## Constitution Check
I/II：复用department:edit授权；负责人必须CanAccessEmployee、同部门在职；上级候选仍按员工范围。III：保存提交/成功/失败及失效负责人可见。IV：先写失败回归。V：nullable列迁移，初始化不覆盖HR，历史不动。VI：复用根目录现有组件/职责函数，无RBAC猜人。设计前后均PASS，无例外。

## Project Structure
- server/models/models.go：Department.LeaderID。
- server/handlers/department.go：leader_id可维护、范围/在职/所属校验、审计。
- server/handlers/department_hierarchy.go：显式负责人有效性、负责人身份、候选与祖先解析。
- server/handlers/evaluation_responsibility_resolver.go / evaluation_template.go：统一负责人来源，新旧创建预览一致。
- server/handlers/department_leader_test.go：保存/范围/失效/候选/评分/历史回归。
- app/departments/page.tsx、lib/api.ts：层级/负责人字段维护与列表。
- tests/unit/department-hierarchy-form.test.mjs：父候选排除自身/后代，负责人UI契约。

## Decisions
1. nullable leader_id，无自动填充。创建部门不允许设置现有员工为负责人（不存在本部门员工），编辑后设置。省略更新保持，null清空；读取保留失效ID以诊断。
2. 父部门负责人来自该部门明确leader_id，未设置时兼容有效DEPARTMENT_MANAGER职责；同一层明确值优先，失效值阻塞，不悄悄回退。CTO仍须公司显式职责。
3. 部门负责人身份：明确leader_id等于员工ID或009标准负责人岗位；支持任意自定义部门和岗位，不读取RBAC角色。
4. 工程师DEPARTMENT_MANAGER优先本部门leader_id；负责人该职责转为上级，循环/自己/停用拒绝。旧职责及其它评分类型不改；新快照记录来源。
5. UI上级列表排除自身及全部后代，后台保护保留；负责人选择来自部门详情已加载在职员工。失效已选负责人显示失效选项，可清空或替换，不隐藏成未配置。
6. 初始层级marker保持，HR保存不被重复启动覆盖。此次不改日常数据库/账号/开关/普通Compose；只在隔离副本验证。

## Complexity Tracking
无宪章例外。没有技术未知项，不需要外部研究或代理。
