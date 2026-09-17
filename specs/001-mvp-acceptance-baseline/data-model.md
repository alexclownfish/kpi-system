# Data Model: MVP 验收与回归基线

本 Feature 不增加生产数据库表。以下实体仅存在于验收运行器的内存、报告和临时业务记录中。

## AcceptanceRun

表示一次完整验收执行。

| Field | Type | Rules |
|---|---|---|
| runId | string | 唯一，格式包含时间和随机后缀，不含秘密 |
| startedAt | timestamp | 执行开始时间 |
| finishedAt | timestamp/null | 结束时写入 |
| baseUrl | URL | 指向统一入口，不含认证信息 |
| status | enum | `running`、`passed`、`failed`、`cleanup_failed` |
| scenarios | AcceptanceScenarioResult[] | 至少包含所有 P1 场景 |
| temporaryRecords | TemporaryRecord[] | 仅记录非敏感标识和精确 ID |
| cleanupStatus | enum | `pending`、`complete`、`failed` |

### State transitions

```text
running → passed
running → failed
running → cleanup_failed
failed  → cleanup_failed
```

只有所有必需场景通过且清理完成时，最终状态才能是 `passed`。

## AcceptanceScenarioResult

| Field | Type | Rules |
|---|---|---|
| id | string | 对应用户故事和场景，例如 `US1-AC1` |
| title | string | 人类可读名称 |
| actor | string | `hr_admin`、`department_manager`、`employee` 或 `operator` |
| status | enum | `pending`、`passed`、`failed`、`skipped` |
| startedAt | timestamp | 场景开始时间 |
| durationMs | integer | 非负整数 |
| expected | string | 不含秘密的预期结果 |
| actual | string/null | 失败时的非敏感摘要 |
| evidence | string[] | 截图、trace 或报告引用；不得包含密码/token |

## TestIdentity

| Field | Type | Rules |
|---|---|---|
| purpose | enum | `seed_hr`、`temporary_manager`、`temporary_employee` |
| employeeId | integer/null | 临时身份创建后写入；种子身份可在登录响应中获取 |
| email | string | 临时身份必须包含本次 runId |
| role | string | 规范角色代码 |
| dataScope | string/null | 登录后验证 |
| secretSource | enum | `environment` 或 `generated`；实际密码不进入模型和报告 |

## TemporaryRecord

| Field | Type | Rules |
|---|---|---|
| resource | string | 当前为 `employee`，未来可扩展 |
| id | integer | 从创建响应取得，清理必须使用该值 |
| label | string | 非敏感名称或邮箱 |
| dependsOn | integer[] | 普通员工依赖负责人 ID |
| cleanupOrder | integer | 依赖方数值更小，先清理 |
| cleanupStatus | enum | `registered`、`deleted`、`failed` |
| cleanupError | string/null | 脱敏后的失败摘要 |

## Relationships

```text
AcceptanceRun 1 ── * AcceptanceScenarioResult
AcceptanceRun 1 ── * TestIdentity
AcceptanceRun 1 ── * TemporaryRecord
Temporary employee ──manager_id──> Temporary manager
```

## Validation Rules

- 临时邮箱和名称必须包含 runId，避免与现有业务数据冲突。
- 所有成功创建的临时记录必须在继续下一步骤前登记清理信息。
- 清理顺序必须满足依赖关系：普通员工先于其负责人。
- 报告序列化前必须过滤密码、Authorization、Cookie、JWT 和密码哈希。
- 清理完成后再次按 ID 查询应返回不存在；仅收到删除成功响应不足以判定清理完成。
