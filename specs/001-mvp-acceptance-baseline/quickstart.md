# Quickstart: MVP 验收与回归基线

## 1. 前置条件

- Docker 和 Docker Compose 可用。
- Node.js 20 及项目依赖已安装。
- Playwright Chromium 已按项目说明安装。
- 当前目录为仓库根目录。
- 使用本地或隔离验收环境，不要指向生产系统。

## 2. 启动系统

```bash
docker compose up -d --build
docker compose ps
```

等待健康检查成功：

```bash
curl --fail http://localhost/health
```

预期返回 `200`，JSON 中 `status` 为 `OK`。

## 3. 设置验收身份

本地种子 HR 默认为：

```bash
export KPI_BASE_URL=http://localhost
export KPI_TEST_HR_EMAIL=sunba@company.com
export KPI_TEST_HR_PASSWORD='本地验收密码'
```

不要把密码写入仓库、命令脚本或验收报告。

## 4. 执行验收

```bash
npm run acceptance:mvp
```

预期结果：

- 健康接口、登录页面和 HR 认证通过。
- 临时部门负责人创建并可登录，Data Scope 为部门范围。
- 临时普通员工创建并可登录，Data Scope 为本人范围。
- 普通员工访问未授权员工创建和公司统计接口被正确拒绝。
- 员工和负责人查询结果符合数据范围。
- 缺少密码、缺少直属上级、重复邮箱得到明确错误。
- 临时员工按普通员工、负责人顺序删除，遗留数为 0。

## 5. 人工页面复核

1. 打开 `http://localhost/auth/login`，使用 HR 测试身份登录。
2. 进入员工管理，点击“添加员工”。
3. 输入唯一邮箱，创建普通员工，确认按钮出现“提交中...”。
4. 创建成功后确认页面出现成功提示且列表刷新。
5. 使用重复邮箱再次提交，确认页面显示后端错误而不是静默失败。
6. 使用普通员工登录，直接访问 `/employees` 和 `/statistics`，确认被跳转到允许页面或显示拒绝。
7. 使用创建返回的精确 ID 删除人工测试员工。

## 6. 失败处理

- 功能失败：查看 Playwright HTML 报告、截图和 trace 中对应的场景 ID。
- 清理失败：根据报告列出的临时记录 ID 手动核对；不要使用批量邮箱删除或清空数据库。
- 服务失败：运行 `docker compose logs --tail=100 backend frontend nginx-gateway`。
- 不要使用 `docker compose down -v`，该命令会删除本地持久化数据。

## 7. 停止服务

如需停止但保留数据：

```bash
docker compose down
```

不得为普通验收添加 `-v`。

## 8. 2026-09-17 实现验证记录

| 门禁 | 结果 | 证据摘要 |
|---|---|---|
| `go test ./...` | PASS | Go 1.23 builder 容器中 handlers/models/routes 全部通过 |
| `tsc --noEmit` | PASS | 本地 TypeScript 5 类型检查通过 |
| Next.js production build | PASS | Docker 构建成功，生成 18 个页面 |
| `docker compose up -d --build` | PASS | backend、frontend、nginx-gateway 均保持运行 |
| `/health` | PASS | Nginx 统一入口返回 `status=OK` |
| `npm run acceptance:mvp` 等价入口 | PASS × 3 | 每次 12 个场景通过，2 条临时员工记录全部删除 |
| 命名卷保留 | PASS | `kpi-system_kpi-db`、`kpi-system_kpi-exports` 仍存在 |
| 未参与实现开发者 10 分钟复核 | PENDING | 需要由同伴按本文从第 1 节开始计时并填写验证人和耗时 |

最近一次结构化结果为 `.artifacts/acceptance-result.json`（本地忽略文件），脱敏 HTML 报告为
`.artifacts/acceptance-result.html`。报告不包含密码、Authorization、Cookie、完整 JWT 或 bcrypt 哈希。
