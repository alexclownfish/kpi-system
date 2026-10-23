# 生产环境部署

当前系统使用 SQLite，不需要手工执行初始化 SQL。后端启动时使用 GORM `AutoMigrate` 建表，并幂等初始化系统角色、权限、数据范围、默认绩效规则和关闭状态的公开注册配置。

默认还会初始化《荭杏研发项目绩效考核表.xlsx》的业务目录：ID 工业设计部、结构部门、电子部门、软件应用开发部、算法开发部、运控部、测试部门、嵌入式开发部、产品部门。每部门包含“部门负责人”和“工程师”岗位，以及两套已发布月度模板，共十八套、138个指标。保留原始维度/指标权重、评分类型和指引，按方案 A 校验综合权重100%。初始导出为竖向。

目录定义已随后端内嵌，部署机器无需 Excel 或 Python，也不需逐条执行 SQL。不会导入表内评分、测试考核、虚构员工或默认弱密码账号。PM、CTO、产品经理是权限角色/评分职责，不是凭空生成的员工。

## 1. 准备配置

```bash
cp .env.example .env
openssl rand -base64 48
```

把生成值写入 `.env` 的 `JWT_SECRET`，并修改：

- `CORS_ALLOWED_ORIGINS`：线上访问域名，例如 `https://kpi.example.com`。
- `EVALUATION_WORKFLOW_MODE`：升级已有数据库时先使用 `audit`（只读审计）；完成迁移演练并确认职责配置后，再改为 `enabled` 才允许新考核写入新版任务流程。`disabled` 可完全关闭审计入口。
- `SEED_HONGXING_CATALOG`：默认 `true`，补齐原表业务目录；非荭杏部署或隔离测试设置 `false`。旧 `SEED_DEMO_DATA` 已弃用，设置为 true 也不再创建张三、王五等账号。
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

### 已有环境升级与目录保护

`docker compose up -d --build` 会保留现有数据，并首次补入原表目录。已有同名部门会复用；同部门/岗位/月度已有模板则保留其状态和内容，不覆盖。完成后写一次性版本记录，之后重启不恢复 HR 已修改、停用或删除的目录和模板。来源表发生变化时，不会静默覆盖现有模板，应由 HR 复制并发布新版本，或另做明确数据升级。

旧演示员工不会在升级中自动删除。需要清理时先备份并确认具体账号，不能用删除数据库卷的方式代替。真实员工和实际评分职责仍需配置，不会把模拟验证账号搬进业务数据库。岗位建议在创建/编辑员工时按所选部门读取，允许自定义岗位。升级不会擅自把考核流程开关从 audit 改为 enabled。

需要只初始化而不启动监听端口时，可以在服务已构建后执行 `docker compose run --rm --no-deps backend /app/kpi-server init-business`。使用相同 .env 和持久化卷，管理员恢复仍要求显式文件凭据，不是重置既有管理员密码的命令。正常部署无需额外执行此命令。

## 4. 首次登录后清理初始化凭据

确认管理员可以登录后：

1. 从 `.env` 删除 `BOOTSTRAP_ADMIN_EMAIL`、`BOOTSTRAP_ADMIN_NAME` 和 `BOOTSTRAP_ADMIN_PASSWORD_FILE` 三行。
2. 删除 `secrets/bootstrap_admin_password`，保留空的 `secrets/` 目录。
3. 执行 `docker compose up -d --force-recreate backend`。

后端检测到已有有效超级管理员后也会跳过初始化，但移除凭据可以降低主机文件泄漏风险。

## 5. HTTPS

项目内置 Nginx 只提供 HTTP。推荐由宿主机 Nginx、Caddy、云负载均衡或 Cloudflare 终止 HTTPS，再反向代理至 `http://127.0.0.1:8088`。不要直接把该 HTTP 端口暴露到公网。

## 6. 统一考核流程迁移

升级现有数据库后，在管理员权限下先打开考核审计页执行预览。确认模板快照、评分职责和历史评分没有歧义后，再执行迁移预演；预演只生成不可变批次，不会创建任务或删除旧数据。人工复核项不能直接执行自动迁移。

迁移执行会再次校验来源快照 checksum。若模板快照在预演后发生变化，批次会拒绝执行，需要重新预演。迁移是追加式的：旧评分、邀请、快照和导出文件不会被删除。出现问题时应先把 `EVALUATION_WORKFLOW_MODE` 改回 `audit`，保留数据库卷并导出备份，再人工处理；不要使用 `docker compose down -v`。

完成迁移后，将 `.env` 中的 `EVALUATION_WORKFLOW_MODE` 改为 `enabled` 并重建 backend。新建考核会在创建时解析评分职责并冻结模板快照；后续维护职责或模板不会改写已创建考核。

## 7. 持久化和备份

Compose 使用以下命名卷：

- `kpi-db`：SQLite 数据库。
- `kpi-exports`：导出文件。
- `kpi-signoffs`：员工纸质签字归档。
- `kpi-backups`：系统内生成的 SQL 备份。

### 数据库访问方式

本项目使用 SQLite，数据库文件在后端容器内的 `/app/db/kpi.db`，不是独立的数据库服务，因此没有可映射的数据库端口。生产环境应通过页面/API 和 E2E 测试访问业务；需要离线查看数据时，管理员先在“备份”页面生成 SQL 备份，再下载到本地使用 SQLite 工具打开或导入。不要把 SQLite 文件直接暴露到公网，也不要在后端运行期间从另一个容器并发写入同一个数据库文件。

本地开发若确实需要直接查看数据库，可以在停止本地服务后对数据库卷做只读副本，再用 SQLite 工具打开副本；不要对运行中的生产卷执行文件替换或直接编辑。

升级可使用 `docker compose up -d --build`，不要执行 `docker compose down -v`，后者会删除数据库及附件卷。生产环境还应定期将这些数据复制到异机或对象存储。

## 8. 一键拉取、构建和重部署

在完整 Git 仓库根目录执行：

```bash
./scripts/deploy-production.sh \
  --prefix horber.auracor.net:18182/hxkj \
  --push
```

脚本会依次执行 `git pull --ff-only`、构建带时间与提交 SHA 标签的 backend/frontend 镜像、可选推送、重建 Nginx、执行 `docker compose up -d --no-build` 和 `/health` 检查。数据卷不会删除。

已由 CI 完成 checkout 或需要离线构建时使用：

```bash
./scripts/deploy-production.sh --skip-pull
```

部署脚本固定读取当前仓库根目录的 `.env` 和 `secrets/`。新克隆仓库不包含这两项，必须从受控备份恢复。若现有 Compose 项目名不是 `kpi-system`，通过 `COMPOSE_PROJECT_NAME=<现有项目名>` 运行，避免连接到新建空数据卷。

### CI 构建区与运行区分离

推荐线上目录：

```text
/mnt/datadisk0/project/kpiSystem/
├── docker-compose.yml
├── .env
├── secrets/
└── ci-build/
    ├── deploy-production.sh
    └── source/
```

`ci-build/source` 是 Git 仓库，负责拉代码和构建镜像；上一层保存线上 `.env`、`secrets` 和数据卷运行配置。仓库中的 `docker-compose.yml` 支持通过 `KPI_SOURCE_DIR` 指向 `ci-build/source`，并通过 `KPI_BACKEND_IMAGE`、`KPI_FRONTEND_IMAGE`、`KPI_NGINX_IMAGE` 使用版本化镜像。

首次切换到该结构：

```bash
cd /mnt/datadisk0/project/kpiSystem

cp ci-build/source/docker-compose.yml ./docker-compose.yml
cp ci-build/source/scripts/deploy-production.sh ./ci-build/deploy-production.sh
chmod +x ci-build/deploy-production.sh
```

之后统一执行：

```bash
cd /mnt/datadisk0/project/kpiSystem

COMPOSE_PROJECT_NAME=kpi-system \
./ci-build/deploy-production.sh \
  --prefix horber.auracor.net:18182/hxkj \
  --push
```

脚本从 `ci-build/deploy-production.sh` 启动时会自动识别同目录下的 `source/`，并在父目录执行 Compose。不要从缺少 Dockerfile 的 `ci-build` 目录直接构建。

如果运行区尚无 `.env`、`secrets` 和 `docker-compose.yml`，脚本会在第一次部署时自动复制 Compose、生成 JWT 和一次性管理员密码。此时必须显式提供域名和管理员邮箱：

```bash
cd /mnt/datadisk0/project/kpiSystem

./ci-build/deploy-production.sh \
  --prefix horber.auracor.net:18182/hxkj \
  --domain https://kpi.auracor.net \
  --admin-email yangwenzhe@howxing.com \
  --push
```

管理员密码只写入 `secrets/bootstrap_admin_password`，不会打印到终端。首次登录确认成功后，应删除 `.env` 中的三项 `BOOTSTRAP_ADMIN_*` 和该密码文件，再重建 backend。
