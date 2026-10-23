# Implementation Plan: 部门层级与考核创建一致性

**Date**: 2026-10-01 | **Spec**: [spec.md](spec.md)

## Summary
修复考核页结构化错误显示；统一模式下预览与创建解析，模板成功/职责失败分别反馈。增加部门 parent_id、可维护父关系、安全迁移四子部门及真实子树授权。目录部门的明确岗位前缀作为等价匹配，不改原员工岗位。负责人部门评分人优先合法直属上级，再最近祖先部门职责，最后唯一公司 CTO；排除自己及无关职责，不从权限角色猜人。

## Technical Context
- Go1.23、Gin/GORM、SQLite、Next15/React19/TypeScript，复用现有模式，无新依赖。
- 规模沿用分页目录；组织树使用 SQLite recursive CTE UNION 去重及遍历 visited 防环。
- 测试：Go单元/HTTP回归、错误解析unit、TS/lint/build；普通Docker由用户构建。
- 历史快照和任务不改，EVALUATION_WORKFLOW_MODE不改变。

## Constitution Check
I/II：所有新接口授权、预览员工范围；子树作用域真实实现、候选仅可见授权人员。
III：字符串/对象错误转文本、具体缺失原因、提交状态。
IV：先写复现回归。V：parent_id可空迁移、独立初始化marker、事务、保留旧员工历史和HR编辑。
VI：正式根目录，复用原模板/职责/目录函数，业务职责与RBAC分离。设计前后均通过。

## Project Structure
- server/models/models.go / department_hierarchy.go：父关系、祖先和子树查询。
- server/handlers/department.go / department_hierarchy.go：维护校验、候选查询、版本初始化。
- server/handlers/evaluation_template.go / evaluation_responsibility_resolver.go / kpi.go：标准岗位、同模式预览、负责人解析。
- server/handlers/employee.go / permissions.go / evaluation_responsibility.go：合法上级、循环验证、子树边界。
- server/handlers/department_hierarchy_test.go：迁移、授权、预览、创建任务回归。
- app/departments/page.tsx / employees/page.tsx / evaluations/page.tsx、lib/api.ts：父部门、上级候选、创建状态/错误。
- tests/unit/api-errors.test.mjs：错误文本契约。

## Decisions
1. 部门层级marker hongxing-hierarchy-v1，与目录marker独立；目录开启时启动补齐，已marker不重写HR编辑。重名/已有不同parent拒绝，幂等事务；原18模板不改。
2. 仅原表目录部门可识别完整部门名或去掉末尾“部/部门”的前缀，尾部只接受工程师、负责人、部门负责人；精确自定义模板优先，其次标准模板。
3. 上级候选接口按员工可见范围限制；工程师同部门，负责人祖先部门有效DEPARTMENT_MANAGER职责或公司CTO。直属上级明确、有效且合法才优先，否则错误不静默绕过。直属循环拒绝。
4. DEPARTMENT_MANAGER负责人解析不使用本部门或公司通用部门负责人；父级每层唯一候选，多个报歧义；父级缺失使用唯一公司CTO。其它指标职责仍原逻辑，多人平均允许，自己被排除且无替补不生成任务。
5. 预览采用当前模式解析，返回模板/scorers/ready/blocking_errors；weighted-v2模板按新职责解析，未enabled时同时显示模式阻塞且创建409，不再产生新模板/旧流程混合记录。职责失败200且ready=false；模板失败404详细文本。既有预览字段保留。
6. 旧流程仅负责人部门评分和DIRECT_MANAGER调用安全规则；PM/产品/CTO遗留逻辑保持，不迁移既有快照。

## Complexity Tracking
无例外；无需新的角色/工作流引擎或员工岗位ID迁移。
