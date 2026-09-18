# 生产环境部署

当前系统使用 SQLite，不需要手工执行初始化 SQL。后端启动时使用 GORM `AutoMigrate` 建表，并幂等初始化系统角色、权限、数据范围、默认绩效规则和关闭状态的公开注册配置。

## 1. 准备配置

```bash
cp .env.example .env
openssl rand -base64 48
```

把生成值写入 `.env` 的 `JWT_SECRET`，并修改：

- `CORS_ALLOWED_ORIGINS`：线上访问域名，例如 `https://kpi.example.com`。
- `KPI_HTTP_BIND`：有外部反向代理时使用 `127.0.0.1`；项目直接提供 HTTP 时使用 `0.0.0.0`。
- `KPI_HTTP_PORT`：外部反向代理连接的本机端口，示例为 `8088`。
- `BOOTSTRAP_ADMIN_EMAIL`、`BOOTSTRAP_ADMIN_NAME`：首次超级管理员账号。

## 2. 准备一次性管理员密码

```bash
mkdir -p secrets
chmod 700 secrets
openssl rand -base64 24 > secrets/bootstrap_admin_password
chmod 600 secrets/bootstrap_admin_password
```

请安全保存该密码。`secrets/` 和 `.env` 均已被 Git 与 Docker 构建上下文排除。

## 3. 启动

```bash
docker compose config
docker compose up -d --build
docker compose ps
docker compose logs --tail=200 backend
curl http://127.0.0.1:8088/api/health
```

生产环境默认不会创建演示部门、员工或默认密码账号，也不会开启公开注册。

## 4. 首次登录后清理初始化凭据

确认管理员可以登录后：

1. 从 `.env` 删除 `BOOTSTRAP_ADMIN_EMAIL`、`BOOTSTRAP_ADMIN_NAME` 和 `BOOTSTRAP_ADMIN_PASSWORD_FILE` 三行。
2. 删除 `secrets/bootstrap_admin_password`，保留空的 `secrets/` 目录。
3. 执行 `docker compose up -d --force-recreate backend`。

后端检测到已有有效超级管理员后也会跳过初始化，但移除凭据可以降低主机文件泄漏风险。

## 5. HTTPS

项目内置 Nginx 只提供 HTTP。推荐由宿主机 Nginx、Caddy、云负载均衡或 Cloudflare 终止 HTTPS，再反向代理至 `http://127.0.0.1:8088`。不要直接把该 HTTP 端口暴露到公网。

## 6. 持久化和备份

Compose 使用以下命名卷：

- `kpi-db`：SQLite 数据库。
- `kpi-exports`：导出文件。
- `kpi-signoffs`：员工纸质签字归档。
- `kpi-backups`：系统内生成的 SQL 备份。

升级可使用 `docker compose up -d --build`，不要执行 `docker compose down -v`，后者会删除数据库及附件卷。生产环境还应定期将这些数据复制到异机或对象存储。
