# 现有 PostgreSQL 生产环境：构建镜像后部署

适用环境是已迁移完成的 `/mnt/datadisk0/project/kpiSystem`、项目 `kpi-system-pg`。构建机负责拉代码、构建和推送；生产运行只需要一份 Compose、原 `.env`、原 `secrets/` 和 `pg-admin-secrets/`。不重复迁移，不初始化或重置管理员。

## 1. 构建并推送（每次代码更新）

在有完整 Git 仓库的构建机操作，工作区先保持干净；服务器原来的 `ci-build/source` 也可以作为构建工作区，但构建和部署是两个独立步骤。以下使用此前提供的镜像仓库地址，若拼写或项目路径不同请替换。

```bash
cd /mnt/datadisk0/project/kpiSystem/ci-build/source
docker login horber.auracor.net:18182

bash scripts/build-docker-images.sh \
  --prefix horber.auracor.net:18182/hxkj/kpi \
  --tag 20261027-01 \
  --platform linux/amd64 \
  --with-nginx \
  --push
```

脚本默认先 `git pull --ff-only`，然后构建并推送三个镜像，不启动生产容器：

```text
horber.auracor.net:18182/hxkj/kpi-backend:20261027-01
horber.auracor.net:18182/hxkj/kpi-frontend:20261027-01
horber.auracor.net:18182/hxkj/kpi-nginx:20261027-01
```

在 Mac 构建给 x86 Linux 服务器使用时，保留 `--platform linux/amd64`；ARM 服务器应改为对应平台。构建自己尚未推送的本地修改时加 `--skip-pull`。每次使用新的版本号，不覆盖已发布版本；记录对应 Git 提交。前后端和**项目内 Nginx**一起发布，外层 HTTPS Nginx 不在这三个镜像内。

## 2. 生产环境一次性切换为单文件运行配置

先确认生产仍是之前核实的 PG 项目和卷；此模板不适用于空服务器或重新迁移。原 `.env`、密码文件、数据库和签字卷全部保留，禁止覆盖为示例配置。若当前生产 Compose 自定义了数据库端口或其他挂载，切换前将所需配置保留进新文件；模板默认不向宿主机开放数据库端口。

在**已有 Git 仓库的同一服务器**，模板路径如下；若构建机与运行机分开，只把此模板传到运行机即可，无需在运行机安装 Git 或保留源码。

```bash
cd /mnt/datadisk0/project/kpiSystem
cp docker-compose.yml "docker-compose.before-images-$(date +%Y%m%d-%H%M%S).yml"
cp ci-build/source/deploy/production/docker-compose.yml docker-compose.yml
```

修改 `docker-compose.yml` 中 backend、frontend、nginx-gateway 的三个 `image:`，填入刚推送的完整地址及相同版本，例如上述 `20261027-01`。PostgreSQL 保持 `postgres:16-alpine`，本次不升级数据库大版本。

新文件已内置 PG 配置，因此**不再叠加** `docker-compose.postgres.yml`。项目名是原来的 `kpi-system-pg`，五个卷明确指向原完整卷名且标记 external；卷不存在会报错，不能生成一个空库。不要更改项目名、卷名或执行 `down -v`。

核对原 `.env`：

- `COMPOSE_PROJECT_NAME=kpi-system-pg`；不要留会覆盖此选择的 `COMPOSE_FILE`，也不要在 shell 中设置它，否则普通 Compose 可能继续加载其他文件。
- 保留原 JWT_SECRET 和两个 PG 密码文件，不生成新密码，不填写 BOOTSTRAP 初始化参数。新模板显式禁用管理员初始化，保留已有账号密码。
- `CORS_ALLOWED_ORIGINS` 必须包含实际来源 `https://kpi.auracor.net:18182`；保留原 SEED_HONGXING_CATALOG 和 EVALUATION_WORKFLOW_MODE 的业务选择。
- 保留运行机所需的 KPI_HTTP_BIND、KPI_HTTP_PORT。外层 Nginx 在另一个 Docker 容器，通过 `172.16.230.14:8088` 转发时，应用网关必须监听该内网地址或 `0.0.0.0`，不能只监听 `127.0.0.1`。无需把上游改成容器内 localhost。

首次应用前验证并预拉镜像，提前发现仓库权限或版本号错误：

```bash
docker login horber.auracor.net:18182
docker compose config --quiet
docker compose pull backend frontend nginx-gateway
docker compose up -d
```

新模板将导出卷挂到 `/app/public/exports`，匹配当前代码的 PDF/Excel 实际写入目录。此前旧容器可写层中的临时导出文件不会自动搬进卷，需保留的请在首次替换前取出；签字卷路径不变。导出文件仍按程序规则过期清理，下载链接也会过期，不是永久档案。

## 3. 此后每次发布

构建机推送新版本；运行机只修改三个应用镜像的版本号，然后：

```bash
cd /mnt/datadisk0/project/kpiSystem
docker compose up -d
```

三个应用镜像配置 `pull_policy: always`，此命令会拉取指定镜像、重建需要更新的服务；不构建、不拉 Git、不需要导出 KPI_SOURCE_DIR。日常保持同一文件中的三镜像版本同步，网关随版本更新会重新解析 backend 地址。首次仓库认证失败需 `docker login`，不应通过现场构建绕过失败。

## 4. 验证和排错

```bash
docker compose ps
docker compose logs --tail=30 backend
curl -fsS http://127.0.0.1:8088/health
```

若 KPI_HTTP_BIND 仅绑定内网 IP，健康请求改为该 IP。确认后端 healthy、日志没有管理员初始化失败，用原 `yangwenzhe@howxing.com` 和当前密码登录，查看部门/角色、导出一份 PDF、查看已有签字材料。重建后再次核对相同账号和材料。

普通重启在运行目录执行 `docker compose restart`。若仅替换后端而网关出现 502，可执行 `docker compose restart nginx-gateway` 重新解析上游。升级后出现数据库迁移或业务问题时，保留日志与版本，不直接改回旧镜像绕过 schema 兼容性评估。

本地配置与发布脚本验证：`python3 tests/production-image-deployment.py`。带实际镜像的隔离验收：

```bash
KPI_TEST_BACKEND_IMAGE=你的本地PG后端镜像 \
KPI_TEST_FRONTEND_IMAGE=你的本地前端镜像 \
KPI_TEST_NGINX_IMAGE=你的本地项目网关镜像 \
python3 tests/production-image-deployment.py --integration
```

只创建独立测试项目/卷，不操作生产或日常开发栈。镜像须事先构建/拉取；临时凭据目录默认位于用户主目录，可用 `KPI_TEST_SCRATCH_PARENT` 指定 Docker 可共享的目录，不输出口令或登录令牌。
