# Tasks: 荭杏业务初始化

## Phase 1: Setup
- [X] T001 只读核对原表并通过 `scripts/build_hongxing_catalog.py` 生成 `server/handlers/data/hongxing-catalog-v1.json`，排除评分及人员。

## Phase 2: Foundation
- [X] T002 在 `server/models/models.go`、`server/models/database.go` 添加 DepartmentID/Name 唯一且非空的 JobPosition 和主键初始化记录迁移。

## Phase 3: US1 原表业务目录
**Independent Test**: 新库九部门十八岗位十八模板，逐指标一致、无员工考核。
- [X] T003 [US1] 在 `server/handlers/hongxing_catalog_test.go` 先添加来源、计数、原始权重、无模拟评分及发布校验回归。
- [X] T004 [US1] 在 `server/handlers/hongxing_catalog.go` 事务初始化原表目录，复用原权重校验、版本化来源、竖向导出、审计。

## Phase 4: US2 安全更新
**Independent Test**: 重复初始化、HR更改删除、既有同岗位模板保护及失败回滚。
- [X] T005 [US2] 在 `server/handlers/hongxing_catalog_test.go` 回归幂等、删除不复建、同名部门歧义、模板复用、事务回滚、无人员写入。
- [X] T006 [US2] 在 `server/main.go` 统一启动和 init-business；在 `server/models/database.go` 移除所有演示创建函数；在 `docker-compose.yml`、`.env.example` 配置目录开关。

## Phase 5: US3 管理员和岗位
**Independent Test**: 管理员真实登录；授权岗位查询和员工表单建议可用。
- [X] T007 [US3] 在 `server/handlers/hongxing_catalog.go`、`server/routes/routes.go` 实现岗位目录接口并在 `server/handlers/hongxing_catalog_test.go` 覆盖允许/拒绝、非法ID及缺失部门。
- [X] T008 [US3] 在 `lib/api.ts`、`app/employees/page.tsx` 按部门加载岗位建议，防过期响应、明确错误、兼容自定义岗位，并加载全部部门。
- [X] T009 [US3] 按 `specs/008-hongxing-business-initialization/quickstart.md` 备份现有库并恢复本地管理员，初始化目录，实际验证登录、权限和读取；凭据留在忽略 secrets 文件，不改变其它账号。

## Phase 6: Quality
- [X] T010 在 `docs/production-deployment.md` 更新初始化和旧环境说明，运行 Go全套、TS、lint、production build、源数据比较、Compose配置及健康；在 `specs/008-hongxing-business-initialization/acceptance.md` 记录实际证据并 Converge。

## Dependencies

T001 → T002 → T003 → T004 → T005 → T006 → T007 → T008 → T009 → T010。先目录MVP再维护保护和页面验收。测试先于对应实现。无并行代理，文档和只读核对可与无关检查并行。

## Phase 7: Convergence

Scoped review found two integration gaps after initialization. Existing broader workflow tasks stay in feature007.

- [X] T011 [US1] Replace truncated template-page department fetching with a complete paginated department catalog in `lib/api.ts`, `app/templates/page.tsx` and `app/employees/page.tsx`; add >100-department and read-failure regressions in `tests/unit/department-catalog.test.mjs`, per FR-001/FR-005 (partial).
- [X] T012 [US2] Remove JobPosition rows transactionally when an otherwise deletable department is removed in `server/handlers/department.go`; add a regression preserving other departments and their positions in `server/handlers/hongxing_catalog_test.go`, per FR-003 and Constitution V (partial).

## Phase 8: Convergence

- [X] T013 [US2] Reject deletion of departments with linked templates before removing positions in `server/handlers/department.go`; verify refusal and isolated position cleanup in `server/handlers/hongxing_catalog_test.go`, per FR-003 and Constitution V (partial).
- [X] T014 [US1] Use the complete department catalog in `app/evaluation-responsibilities/page.tsx` so all nine seeded departments can be selected when configuring real scorer assignments, per FR-001/FR-008 (partial).
