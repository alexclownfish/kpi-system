# Contract: API Acceptance Scenarios

所有路径相对于 `KPI_BASE_URL`，通过 Nginx 统一入口访问。

## Authentication and health

| ID | Actor | Request | Expected |
|---|---|---|---|
| PRE-01 | operator | `GET /health` | 200，`status=OK` |
| PRE-02 | anonymous | `GET /auth/login` | 200，存在登录表单 |
| PRE-03 | HR | `POST /api/auth/login` | 200，含 token、Permission、Data Scope |

## Employee creation

| ID | Actor | Request | Expected |
|---|---|---|---|
| US1-AC1 | HR | 创建普通员工，含临时负责人 ID | 201，角色为 employee，返回精确 ID |
| US1-AC2 | HR | 创建部门负责人，无直属上级 | 201，业务角色为 manager，登录角色规范化为 department_manager |
| US1-AC3A | HR | 创建员工但缺少密码 | 400，明确密码要求，不创建记录 |
| US1-AC3B | HR | 使用已创建临时邮箱再次创建 | 409 或明确的 4xx 冲突响应，不创建第二条记录 |
| US1-AC3C | HR | 创建普通员工但缺少直属上级 | 400，明确上级要求，不创建记录 |

## Permission and scope

| ID | Actor | Request | Expected |
|---|---|---|---|
| US2-AC1A | employee | `POST /api/employees` | 403，响应不含员工列表或敏感数据 |
| US2-AC1B | employee | `GET /api/statistics/dashboard` | 403，响应不含公司统计 |
| US2-AC2 | employee | `GET /api/employees` | 200，仅包含本人记录 |
| US2-AC3 | department_manager | `GET /api/employees` | 200，仅包含负责人所属部门记录 |
| US2-AC4 | HR | 修改或删除超级管理员 | 403，目标记录不变 |

拒绝场景必须命中预期 4xx；网络错误、代理错误和 5xx 均判定为失败。

## Cleanup

| Order | Actor | Request | Expected |
|---:|---|---|---|
| 1 | HR | 删除临时普通员工 ID | 200，随后按 ID 查询不存在 |
| 2 | HR | 删除临时部门负责人 ID | 200，随后按 ID 查询不存在 |

若原功能场景失败但 HR token 仍有效，仍执行清理。认证失效时允许重新登录一次 HR 后重试清理。
任何清理失败都必须记录 ID，并使用退出码 3 结束。
