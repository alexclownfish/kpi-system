# 验收记录

**Date**: 2026-10-02
**Scope**: Feature 011；正常服务与隔离测试结果明确分离。

## 原因

- 页面和后端都禁止所有系统角色编辑。
- seedAccessControl 每启动补回默认权限和范围，无法保留取消操作。
- 已有 UserRole 用户在权限缺失时仍回退旧 Employee.Role；空角色可能重获 HR 权限。
- 普通 backend 读到 EVALUATION_WORKFLOW_MODE=audit；.env 没有该开关，Compose 默认 audit。
- 用户截图 PM / PRODUCT_MANAGER 缺失是业务评分职责缺失，不是模板无法匹配。
- 正常网关为80；18090旧测试网关转发到已停止测试后端，不能用它验收普通 Compose 更新。

## Spec Kit

- specify / clarify: 需求边界清晰，16/16质量条目通过；实际人员配置授权仍待用户确认。
- plan / tasks: 10任务，7需求覆盖，无需新依赖或研究代理。
- analyze: FR-001 -> T003/T004/T006；FR-002 -> T004/T005/T002；FR-003 -> T003/T002；FR-004 -> T004/T006；FR-005 -> T007/T008；FR-006 -> T007/T009/T010；FR-007 -> T006/T008/T010。零 CRITICAL/HIGH 冲突，未映射任务零。

## 自动化

- 修复前新增测试失败：系统角色403、空绑定回退旧HR权限、缺少定制标记。
- Go 全量 go test ./... -count=1：handlers/models/routes全部通过。
- 新回归覆盖：普通系统角色更新、写权限依赖、稳定属性拒绝、super_admin保护、缺权限403、超目标授权403、空权限无回退、保留未迁移兼容、范围上限、启动两次不恢复权限/范围。
- 旧测试补齐真实 RoleDataScope 夹具，而非放松拒绝默认规则。
- Node --test tests/unit/*.mjs：5/5通过。
- tsc --noEmit：通过；角色/考核/lib API目标 lint通过。
- 临时源码 Next production build：通过，原模板页 useEffect fetchTemplates依赖警告仍存在。
- git diff --check：通过。
- docker compose config --quiet：通过，无正常服务重建。

## 页面与数据验收（仅隔离克隆）

- frontend18096 -> backend18097，两者仅回环。原80服务无重建、无权限/职责/密码变更。
- 实际点击项目经理编辑，取消 report:department后保存成功；权限10->9，产品经理仍10，全部其他角色与用户绑定和范围逐项不变。
- 重启18097后API比对一致，取消权限未恢复。
- 软件应用开发工程师王绍品，模板13，解析DEPARTMENT_MANAGER/PM/PRODUCT_MANAGER，预览ready=true且显示enabled。
- 页面创建2020年9月隔离考核9，成功提示；7条评分任务按item_id/type/user_id逐项与模板预览完全一致。
- 普通系统角色名称与直属上级要求禁用，超级管理员无编辑按钮，系统角色无删除按钮。
- 手机390x844：pageWidth390、dialog clientWidth/scrollWidth356，零横向溢出；截图已检查。
- 临时18096服务停止，18097和Go验证容器停止，验收标签已关闭，浏览器尺寸恢复；不删除数据副本。

## Evidence

- `/private/tmp/kpi-hierarchy-runtime.aCSIPx/role-permissions-saved.jpg`
- `/private/tmp/kpi-hierarchy-runtime.aCSIPx/role-editor-mobile.jpg`
- `/private/tmp/kpi-hierarchy-runtime.aCSIPx/creation-readiness.jpg`
- `/private/tmp/kpi-hierarchy-runtime.aCSIPx/evaluation-created.jpg`
- `/private/tmp/kpi-hierarchy-runtime.aCSIPx/verify-role-readiness.mjs`：回环端口断言、私密凭据不输出、角色隔离与任务逐项对比。

## Remaining Environment Prerequisites

代码可交付，但普通环境不能宣称已可创建。用户需确认启用enabled以及实际PM/PRODUCT_MANAGER评分职责人员及作用域；普通服务重建由用户执行。不需要本地git pull；不清数据库或更换管理员。
