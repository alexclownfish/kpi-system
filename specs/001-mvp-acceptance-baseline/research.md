# Research: MVP 验收与回归基线

## Decision 1: 使用 Playwright 统一浏览器和 API 验收

**Decision**: 使用 Playwright Test。浏览器上下文验证真实页面交互和反馈，APIRequestContext 验证
认证、权限、数据范围、健康检查和清理。

**Rationale**: Feature 同时要求验证 UI 反馈与服务端安全边界。单一运行器可以共享身份、临时数据
和报告，同时提供失败截图、trace 和明确步骤名称。

**Alternatives considered**:

- 仅使用 Bash + curl：轻量，但无法可靠验证浏览器提示、跳转和菜单状态。
- 仅使用 Go `httptest`：适合处理器回归，但不能覆盖 Nginx、前端和真实认证链路。
- 只保留人工检查：不能满足可重复执行和失败定位要求。

## Decision 2: 使用现有 Compose 环境而不是临时空数据库

**Decision**: 验收连接现有本地 Compose 环境，在运行前检查服务健康，不执行数据库重置或
`docker compose down -v`。

**Rationale**: 规格要求证明验收不会破坏现有数据。使用持久化环境可以直接验证唯一命名、精确
清理和重复运行能力。

**Alternatives considered**:

- 每次创建空数据库：隔离性高，但无法证明对已有数据安全。
- 复制数据库文件：SQLite 卷复制在不同 Docker 环境中复杂，并增加误操作风险。

## Decision 3: 临时身份采用依赖顺序和反向清理

**Decision**: 每次运行生成 `codex-acceptance-<timestamp>-<random>` 前缀；先创建部门负责人，再
创建以其为直属上级的普通员工。记录每个返回 ID，在 teardown 中按普通员工、负责人顺序删除。

**Rationale**: 员工删除受下属关系约束，反向清理符合数据库关系。随机前缀允许失败后再次运行，
不会覆盖已有记录。

**Alternatives considered**:

- 使用固定邮箱：重复运行和失败恢复容易发生冲突。
- 复用种子负责人：减少一个临时记录，但不能独立验证负责人创建与初始密码登录。
- 按邮箱批量删除：目标范围不够精确，违反数据安全原则。

## Decision 4: 清理属于验收结果的一部分

**Decision**: 所有创建成功的临时记录立即登记到清理栈。无论测试通过或失败都执行清理；清理
失败时整体结果失败，并报告记录 ID、邮箱后缀和建议人工处理步骤。

**Rationale**: 将清理视为 best-effort 会导致长期污染和后续假失败。清理状态必须与功能状态
同等可见。

**Alternatives considered**:

- 忽略清理异常：无法达到零遗留目标。
- 测试结束后清空数据库：会破坏已有数据和种子账号。

## Decision 5: 权限验证必须检查响应内容

**Decision**: 拒绝场景检查预期 HTTP 状态和响应中不含受保护实体；允许场景检查角色、Permission、
Data Scope 和结果集合边界。

**Rationale**: 仅检查非 2xx 可能把服务故障误判为正确拒绝；只检查页面菜单无法证明 API 安全。

**Alternatives considered**:

- 任何失败响应都算拒绝成功：会掩盖 500、代理错误和后端崩溃。
- 只检查登录响应：不能证明查询端真正应用数据范围。

## Decision 6: 报告默认面向人，同时支持结构化结果

**Decision**: 控制台输出分阶段的人类可读结果，Playwright 同时生成 HTML/JSON/JUnit 中至少一种
机器可读报告。日志过滤 Authorization、Cookie、密码和完整 token。

**Rationale**: 本地维护者需要快速定位，未来 CI 需要机器判定；二者不应要求两套测试。

**Alternatives considered**:

- 只输出原始 HTTP 日志：信息冗余且容易泄露秘密。
- 只输出通过/失败：无法满足失败阶段定位。

## Decision 7: UI 定位优先使用可访问语义

**Decision**: 优先使用页面标题、Label、Button 和 Dialog 的可访问名称。仅在组件没有稳定语义时
增加少量 `data-testid`，不得依赖 CSS 层级或动态 class。

**Rationale**: 可访问定位同时提升可维护性和页面可访问性，样式调整不会频繁破坏验收。

**Alternatives considered**:

- CSS/XPath：对布局和样式变化敏感。
- 为所有元素增加测试 ID：侵入性高且降低规格可读性。
