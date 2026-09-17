# KPI System Constitution

## Core Principles

### I. 后端强制授权

所有受保护资源和写操作 MUST 在 Go 后端通过 Permission 中间件或等价的服务端校验授权。
前端菜单、按钮和路由守卫 MUST 与权限保持一致，但不得作为安全边界。未被明确授予的权限
MUST 默认拒绝。超级管理员的创建、变更、停用和删除 MUST 由具备系统角色管理权限的用户执行。

### II. 数据范围隔离

员工、考核、邀请评分和报表查询 MUST 在后端应用用户的数据范围。`SELF`、
`DIRECT_SUBORDINATES`、`DEPARTMENT`、`DEPARTMENT_TREE`、`ASSIGNED` 和 `ALL`
的行为 MUST 在规格和测试中明确。不得先返回超范围数据再由前端过滤。尚未完整实现的数据范围
MUST 明确记录限制，禁止以近似行为冒充完整支持。

### III. 明确的接口契约与用户反馈

API MUST 返回稳定、可理解的成功和错误结构；敏感字段（包括密码和密码哈希）MUST 永不出现在
响应中。所有页面写操作 MUST 展示提交中、成功和失败状态，禁止只记录控制台而让用户面对静默
失败。重复提交、校验失败、权限拒绝和服务异常 MUST 有可操作的反馈。

### IV. 可测试的增量交付

每个 Feature MUST 由可独立验收的用户故事组成，并为权限边界、数据隔离、状态流转或 API
契约变更提供自动化测试。缺陷修复 MUST 添加能够复现原问题的回归测试。实现任务 MUST 标明
真实文件路径和验收方法，不得使用“完善功能”等无法判定完成状态的描述。

### V. 数据与部署安全

数据库初始化和种子数据 MUST 幂等。Schema 变更 MUST 保留现有业务数据并提供兼容或迁移说明。
测试和日常重启不得删除 Compose 数据卷；任何会清空 SQLite 数据或持久化卷的操作 MUST 获得
明确授权。Docker Compose MUST 提供可重复的构建、启动、健康检查和停止方式。

### VI. 单一代码源与最小复杂度

正式业务代码以仓库根目录的 `app/`、`components/`、`lib/` 和 `server/` 为唯一来源。
`kpi-main/` 等历史副本不得与正式代码并行修改，除非 Feature 明确要求迁移或删除。实现 MUST
优先复用现有模式，避免为 MVP 引入复杂动态权限表达式、无需求支撑的抽象或第二套实现。

## Technical Constraints

- 前端技术基线为 Next.js 15、React 19 和 TypeScript；后端为 Go 1.23、Gin、GORM 和 SQLite。
- 身份认证使用 JWT，密码 MUST 使用 bcrypt 或更强的单向密码哈希存储。
- Role、Permission 和 Data Scope MUST 保持数据驱动；业务代码不得以页面角色名称替代权限校验。
- 高风险操作（角色分配、密码重置、权限或系统配置变更、关键数据删除）MUST 写入审计记录。
- 新增或变更 API MUST 在 Feature 的 `contracts/` 或 Plan 中记录请求、响应和错误行为。
- 文档中的角色、权限、运行命令和访问地址 MUST 与当前实现一致。

## Development Workflow and Quality Gates

1. 非紧急开发 MUST 按 `specify → clarify → plan → tasks → analyze → implement → converge`
   流程推进；紧急缺陷修复也 MUST 在合并前补齐回归规格或测试证据。
2. `spec.md` 描述用户价值和验收结果，不得混入框架或文件级实现方案；技术选择归入 `plan.md`。
3. 实现前 MUST 运行 Analyze 并解决 CRITICAL/HIGH 级规格冲突或给出经批准的例外理由。
4. 提交前 MUST 通过与改动相称的质量门禁，最低包括：
   - 后端：`go test ./...`；
   - 前端：`tsc --noEmit` 和 Next.js production build；
   - 部署变更：`docker compose config`、构建启动及 `/health` 检查；
   - 权限或数据范围变更：至少一个允许场景和一个拒绝场景的回归测试。
5. `quickstart.md` MUST 包含可重复的人工验收步骤；临时测试数据 MUST 使用唯一标识并安全清理。
6. Feature 完成后 MUST 运行 Converge；存在未满足项时追加任务并继续实现，直至无高风险缺口。

## Governance

本宪章优先于 Feature 规格、实现计划和个人编码习惯。任何违反 MUST 条款的实现不得以“已有代码”
或“时间紧”为理由直接通过。宪章修订必须说明影响范围、迁移需求和版本变更理由，并采用语义化
版本：原则删除或不兼容重定义递增 MAJOR，新增原则或实质扩展递增 MINOR，文字澄清递增 PATCH。
每个 Plan 必须完成 Constitution Check；代码审查和 Converge 必须复核适用条款。确需例外时，
必须在 `plan.md` 的 Complexity Tracking 中记录原因、风险、替代方案和批准结论。

**Version**: 1.0.0 | **Ratified**: 2026-09-17 | **Last Amended**: 2026-09-17
