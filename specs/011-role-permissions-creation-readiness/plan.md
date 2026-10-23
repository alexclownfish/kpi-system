# Implementation Plan: 独立角色权限维护与考核创建准备

**Date**: 2026-10-02 | **Spec**: [spec.md](spec.md)

## Summary

允许非 super_admin 的系统角色编辑权限、说明、数据范围，保护身份、名称及 requires_manager；复用现有 PUT /roles/:id 与权限依赖校验。新增 access_customized 标记，事务保存后种子初始化不再回填其权限、范围。创建预览暴露 workflow_mode，页面展示业务模式及职责维护入口，保留现有硬性阻塞与评分任务工厂。

## Technical Context

- Language: Go 1.23 / TypeScript / React 19 / Next.js 15。
- Dependencies: Gin、GORM、SQLite、现有 Shadcn 与 Lucide。
- Storage: Role 增加 default:false 的内部 access_customized 字段，无数据删除。
- Tests: Go memory SQLite 回归、Node 单元检查、tsc、lint、Next production build、隔离克隆页面操作。
- Platform: 本地与 Docker Compose；正常服务由用户自行重建。
- Scope: server/models、server/handlers/role.go 与 evaluation_template.go、app/roles 与 evaluations、lib/api.ts。
- Constraints: 不动真实授权、密码、历史任务和环境开关；评分职责和环境设置需用户确认。

## Constitution Check

- I PASS: 保留 role:edit 中间件，更新前校验目标角色权限上限、提交权限及范围，super_admin 禁止编辑。
- II PASS: 使用原有数据范围体系，补齐 DEPARTMENT_TREE 目录，不扩大实际数据可见性。
- III PASS: 复用稳定错误提取，保存状态与提示明确。
- IV PASS: 新增系统角色允许/拒绝、身份保护、角色隔离及种子重启保留测试。
- V PASS: 加字段兼容迁移，默认仍保留未定制角色的种子升级能力；不删除卷。
- VI PASS: 仅根目录正式代码，无新框架。
- Post-design review: 全部通过，无例外。

## Project Structure

- specs/011-role-permissions-creation-readiness/{spec,plan,research,data-model,quickstart,tasks}.md
- specs/011-role-permissions-creation-readiness/contracts/access-control.md
- server/models/{models.go,database.go,database_test.go}
- server/handlers/{role.go,role_edit_test.go,evaluation_template.go,department_hierarchy_test.go}
- app/{roles,evaluations}/page.tsx
- lib/api.ts
- tests/unit/role-editor.test.mjs

## Phases

1. Tests first: 授权/隔离/保护/持久化及预览模式回归。
2. US1: 标记迁移、种子规则、后端更新、前端编辑。
3. US2: 创建准备反馈与权限感知维护入口，重用既有创建一致性测试。
4. 完整质量门禁、隔离页面验证、验收记录及 converge。

## Complexity Tracking

无宪章例外。无需外部研究或代理；所有技术选择沿用仓库已验证实现。
