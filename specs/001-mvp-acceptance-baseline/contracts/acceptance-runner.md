# Contract: MVP Acceptance Runner

## Command

```bash
npm run acceptance:mvp
```

可选参数由 Playwright CLI 提供，例如 headed、指定 reporter 或只执行单个场景。项目脚本必须提供
适合本地默认运行的配置，不要求调用者记忆测试文件路径。

## Required environment

| Variable | Required | Default | Description |
|---|---:|---|---|
| `KPI_BASE_URL` | no | `http://localhost` | Nginx 统一入口 |
| `KPI_TEST_HR_EMAIL` | no | `sunba@company.com` | 本地种子 HR，仅用于验收环境 |
| `KPI_TEST_HR_PASSWORD` | yes | none | HR 密码；不得写入报告或提交到仓库 |

运行器在缺少必需变量时必须在创建数据前终止，并给出变量名称，不回显其值。

## Preconditions

1. 前后端和网关已启动。
2. `GET /health` 返回 `200` 且 JSON `status` 为 `OK`。
3. 登录页面返回成功并包含登录表单。
4. HR 测试身份可登录并具有 `employee:create` 权限。

任何前置条件失败时不得创建临时业务记录。

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | 所有必需场景通过且清理完成 |
| 1 | 一个或多个功能/权限/数据范围场景失败 |
| 2 | 配置、环境或服务前置条件失败 |
| 3 | 功能场景结束但临时数据清理失败 |

## Console output

每个场景输出一行阶段结果：

```text
[PASS] US1-AC1 创建普通员工并使用初始密码登录 (1234 ms)
[FAIL] US2-AC1 普通员工访问员工管理 API: expected 403, received 500
[CLEANUP] employee id=42 deleted
```

不得输出请求的 `Authorization`、Cookie、明文密码、完整 token 或数据库密码哈希。

## Structured result

机器可读结果必须至少包含：

```json
{
  "runId": "acceptance-20260917-abcdef",
  "status": "passed",
  "startedAt": "2026-09-17T10:00:00Z",
  "finishedAt": "2026-09-17T10:02:00Z",
  "scenarios": [
    {
      "id": "US1-AC1",
      "title": "创建普通员工并使用初始密码登录",
      "status": "passed",
      "durationMs": 1234
    }
  ],
  "cleanup": {
    "status": "complete",
    "recordsCreated": 2,
    "recordsDeleted": 2
  }
}
```

实现可以增加字段，但不得改变上述字段含义或写入认证秘密。
