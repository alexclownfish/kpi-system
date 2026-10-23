# Implementation Plan: 评分职责候选员工完整加载

**Branch**: 当前工作区 | **Date**: 2026-10-02 | **Spec**: [spec.md](spec.md)
**Input**: `specs/012-scorer-employee-catalog/spec.md`

## Summary

employeeApi 增加 getCatalog，复用 departmentApi 逐页读取模式，每页 100，保留过滤条件和服务端数据范围。评分职责页面改用完整目录并增加失败/空状态和操作门禁。员工分页按 id 升序，确保固定名单分页稳定。不新增接口或依赖，不改业务配置。

## Technical Context

**Language/Version**: TypeScript、React 19、Next.js 15、Go 1.23
**Primary Dependencies**: 既有 Axios、Radix Select、Gin/GORM
**Storage**: 既有 SQLite，无迁移
**Testing**: Node 回归、Go 全量、tsc、ESLint、Next production build、隔离页面真实操作
**Target Platform**: 桌面和移动浏览器
**Project Type**: Web application
**Performance Goals**: 205 人 3 页请求，无逐员工请求
**Constraints**: 不绕过授权、不显示残缺目录、不重建正式环境
**Scale/Scope**: 仅评分职责页面消费新目录；测试 0、17、100、101、205 人

## Constitution Check

研究前和设计后均通过。I/II：保留既有接口权限及 ApplyEmployeeScope。III：失败提示和刷新重试，禁用误操作。IV：测试先行，复现 500 超限回退 10 人。V：无迁移，克隆数据测试。VI：只改根目录源码，复用现有模式。无例外。

## Project Structure

### Documentation

`specs/012-scorer-employee-catalog/`：spec、plan、research、data-model、contracts、quickstart、tasks、acceptance。

### Source Code

```text
lib/api.ts
app/evaluation-responsibilities/page.tsx
server/handlers/employee.go
server/handlers/employee_catalog_test.go
tests/unit/employee-catalog.test.mjs
```

**Structure Decision**: 保留现有结构，先测试再目录和门禁，最后质量门禁、页面验收、converge。

## Complexity Tracking

无违例；无未知技术选择，不需委派研究。
