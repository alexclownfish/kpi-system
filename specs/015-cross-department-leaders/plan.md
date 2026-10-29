# Implementation Plan: 跨部门兼任负责人

**Date**: 2026-10-28 | **Spec**: spec.md

## Technical Context
Go 1.23/Gin/GORM，Next.js15/React19/TS；PG生产、SQLite测试兼容。不新增表；升级评分任务唯一索引以区分同一人的不同评分职责。复用 departments.leader_id（多个部门可引用同一员工），Employee.DepartmentID 不变。适用 Linux Docker / macOS。

## Implementation Baseline (开发前)
部门页面候选仅本部门员工；CreateDepartment拒绝负责人；UpdateDepartment/explicitDepartmentLeader限制同部门；managerAssignmentError 普通员工拒绝跨部门；DEPARTMENT和TREE只查询所属部门。已有评分任务和快照固定。

## Decisions / Constitution Check
后端权限与目标部门范围一起校验；新增负责人候选仅返回当前授权范围内的在职员工的必要字段。普通员工上级候选补入被明确任命的外部负责人，但不暴露其他外部人员。组织范围不是功能权限：仅DEPARTMENT和TREE合并负责部门；SELF/ASSIGNED/直属下属不扩大。所有写入保留审计，无新增表/列、初始化或清库；仅以新四列唯一索引替换旧三列索引，不修改旧任务记录，遵循宪章 I–VI。测试先复现再修复。无扩展 hooks；无额外代理或研究未知项。

## Implementation
- server/models/department_hierarchy.go: 可复用的用户管理部门集合SQL（本人所属 + 本人任命部门，TREE递归）
- server/models/models.go: Department.Leader *DepartmentLeaderSummary 手动批量读取的只读摘要（id/name/position/is_active），不建立员工GORM关联且不泄露密码
- server/handlers/department.go: 创建/编辑候选/负责人授权、target scope、交接计数和审计；GET批量填充负责人摘要（仅范围允许的部门）
- server/handlers/department_hierarchy.go: 跨部门显式负责人有效性、上级候选和循环/自评保护；自动补上级只处理负责人在本部门的原场景
- server/handlers/evaluation_responsibility_resolver.go、scoring_task_factory.go：按考核/指标/评分人/职责生成并幂等去重，保留同一人的两种职责。
- server/models/models.go、database.go：创建idx_evaluation_item_scorer_duty四列唯一索引后删除旧idx_evaluation_item_scorer，幂等升级；索引创建失败保留旧约束，旧任务/分数不变。已有考核不补造历史缺失任务。
- server/handlers/permissions.go: 员工和考核统一部门集合；职责管理也复用集合
- server/routes/routes.go, lib/api.ts, app/departments/page.tsx: 新候选接口、分页加载、姓名展示、交接提醒和错误反馈
- tests: 后端集成正反例、前端交互与类型/构建、隔离PG查询兼容；不操作日常Docker

## Validation
Docker builder 运行完整 go test ./...；TypeScript+生产build；隔离数据库通过HTTP handler集成测试回归创建员工、生成任务、评分确认及历史保持；浏览器仅使用模拟API验证部门页面，不视为生产服务验收；权限允许/拒绝，quickstart页面步骤。无法证实项记录真实限制不提前通过。
