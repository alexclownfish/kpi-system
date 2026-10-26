# 生产环境从 SQLite 迁移到 PostgreSQL：本次仅身份/组织/RBAC

**2026-10-23 用户修订范围**：目标目录 `/mnt/datadisk0/project/kpiSystem` 已有健康的 `kpi-system-pg-postgres-1` 容器。本次新 PG **仅迁移九张表的数据**：`departments`、`job_positions`、`employees`、`roles`、`permissions`、`user_roles`、`role_permissions`、`data_scopes`、`role_data_scopes`。`init-schema` 会建立全部应用表结构；旧模板、考核、评分、确认、签字元数据、审计等记录不会进入新库。旧 SQLite **全库**和签字文件仍必须停写后完整封存、异机备份并验证可恢复，旧服务及卷保留用于回退。`--scope full` 是保留的另一种全量模式，**本次绝不省略 `--scope identity` 参数**。这是用户明确要求的宪章 V 例外，详见 specs/014-postgres-production-deployment/plan.md。

**当前停点**：仅凭 `postgres healthy` 不可创建 `secrets/postgres_migration_verified`，也不可启动 `backend`/`frontend`/`nginx-gateway`。先确认已发布的代码包含 `--scope identity`，取得可恢复旧库/附件归档，再迁移、核对。用户在聊天中贴出的 `JWT_SECRET` 已暴露，正式启动前须在服务器本机轮换，**不要再次打印或发送密钥**；旧登录会话会失效。以下命令在目标 Ubuntu 服务器执行；先用旧快照隔离演练，绝不对源库或旧卷写入。

## 0. 发布与旧环境预检

**本次实际源库补充（2026-10-26）**：旧 SQLite 来自用户本地 Mac 的 Docker backend，不在目标 Ubuntu 服务器。用户已将本地停止写入后生成的独立快照传至 `/mnt/datadisk0/project/kpiSystem/migration-archive/20261026-160647/kpi.sqlite.snapshot`，并确认源端/目标端 SHA-256 一致。以下旧线上路径只适用于旧服务确实在服务器上的情况，不应在本次目标服务器重新查找或创建旧库。附件复制/引用核验及旧档案恢复未全部取证，不能仅凭数据库摘要将 T024 勾为完成。

一次性迁移程序的 `--scope identity`、相关测试和本手册必须先经测试、提交并推送到 Git；服务器不会得到开发机尚未推送的改动。记录并锁定包含身份模式的发布 commit；确认 SSH 可读 `git@github.com:alexclownfish/kpi-system.git`。现有 PostgreSQL 容器已健康，不要为此重复初始化或删除它的卷。

示例旧路径仅来自此前部署记录，必须按实际服务器核实，不要直接假定它仍在使用：

    OLD_DIR=/mnt/datadisk0/project/kpi-system/kpi-system
    test -f "$OLD_DIR/docker-compose.yml" || { echo '先查明旧部署路径'; exit 1; }
    cd "$OLD_DIR"
    docker compose ps
    docker compose config --services
    OLD_CID=$(docker compose ps -q backend)
    test -n "$OLD_CID"
    docker inspect "$OLD_CID" --format '{{range .Mounts}}{{println .Destination .Name .Source}}{{end}}'

核对 /app/db、/app/uploads/evaluation-confirmations 的真实挂载和旧服务在用的 Compose 项目名；不要用新目录是否为空判断旧数据是否存在。记录网关当前配置、旧域名、旧 Git commit 和容器 ID。保留一份不在同一数据盘上的可恢复备份位置。

## 1. 停写并制作旧库、附件快照

安排维护窗口并停止旧后端的写入；保存容器 ID 后再停服务。旧服务在整个迁移和切流前保持停写。

    cd "$OLD_DIR"
    OLD_CID=$(docker compose ps -q backend)
    test -n "$OLD_CID"
    docker compose stop backend
    SNAP=/mnt/datadisk0/project/kpiSystem/migration-archive/$(date -u +%Y%m%d-%H%M%S)
    install -d -m 700 "$SNAP" "$SNAP/sqlite-dir" "$SNAP/signoffs"
    docker cp "$OLD_CID":/app/db/. "$SNAP/sqlite-dir/"
    docker cp "$OLD_CID":/app/uploads/evaluation-confirmations/. "$SNAP/signoffs/"
    test -s "$SNAP/sqlite-dir/kpi.db"

用 Python 标准库 SQLite backup API 从停止写入的完整目录再生成单文件一致性快照；迁移程序拒绝带 `-wal`/`-shm` 的直接拷贝。若旧目录或附件位置不同，先按 `docker inspect` 的实际挂载修正，不要运行空目录迁移。`PRAGMA integrity_check` 应通过；旧库**排除表**中的历史外键孤儿不要在此预检误判为九表迁移失败，九表自身关联由迁移器单独检查。

    python3 - "$SNAP/sqlite-dir/kpi.db" "$SNAP/kpi.sqlite.snapshot" <<'PY'
    import sqlite3, sys
    from urllib.parse import quote
    src = sqlite3.connect('file:' + quote(sys.argv[1]) + '?mode=ro', uri=True)
    dst = sqlite3.connect(sys.argv[2])
    src.backup(dst)
    check = dst.execute('PRAGMA integrity_check').fetchone()[0]
    dst.close(); src.close()
    if check != 'ok':
        raise SystemExit('SQLite integrity_check failed')
    PY
    sha256sum "$SNAP/kpi.sqlite.snapshot" > "$SNAP/sqlite.sha256"
    (cd "$SNAP/signoffs" && find . -type f -print0 | sort -z | xargs -0 -r sha256sum) > "$SNAP/signoffs.sha256"
    sha256sum -c "$SNAP/sqlite.sha256"
    find "$SNAP/signoffs" -type f | wc -l

在快照中确认至少一个有效超级管理员，以及旧数据库引用的纸质签字文件都已进入旧档案。签字文件**只归档，不复制到新 PG 的签字卷**。若任一检查失败，先查明旧服务的实际挂载，不要启动迁移：

    python3 - "$SNAP/kpi.sqlite.snapshot" "$SNAP/signoffs" <<'PY'
    import pathlib, sqlite3, sys
    db = sqlite3.connect(sys.argv[1])
    signoffs = pathlib.Path(sys.argv[2])
    admins = db.execute('SELECT count(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id JOIN employees e ON e.id=ur.user_id WHERE r.code=? AND e.is_active=1', ('super_admin',)).fetchone()[0]
    if admins < 1:
        raise SystemExit('源库中没有有效超级管理员；先明确账号恢复方案')
    tables = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
    references = []
    if 'evaluation_confirmations' in tables:
        references += [row[0] for row in db.execute("SELECT attachment_stored_name FROM evaluation_confirmations WHERE attachment_stored_name<>''")]
    if 'evaluation_signoff_supplements' in tables:
        references += [row[0] for row in db.execute("SELECT attachment_stored_name FROM evaluation_signoff_supplements WHERE attachment_stored_name<>''")]
    missing = [name for name in references if not (signoffs / pathlib.Path(name).name).is_file()]
    if missing:
        raise SystemExit(f'签字附件缺失 {len(missing)} 个；先核对旧卷和路径')
    print(f'有效超级管理员 {admins} 个；数据库引用签字附件 {len(references)} 条，文件均已备份')
    db.close()
    PY

再把整个 SNAP 目录复制到另一块磁盘或异机备份，并校验摘要。不能只保留应用页面导出的旧 SQL：它未覆盖全部新业务表。迁移前不要清理旧 Docker 卷，也不要重设原管理员密码。

## 2. 核对当前 PG、更新到包含身份模式的发布提交

用户当前 `docker compose ps -a` 仅显示 `kpi-system-pg-postgres-1` 健康；**保持其运行，不要 `down -v`、不要直接 `up` 整套服务**。先锁定正确 Compose 项目、已有目标卷及代码版本。若 `ci-build/source` 已是仓库，不重复克隆；若有本地改动先人工处理，不在服务器强制覆盖。

    ROOT=/mnt/datadisk0/project/kpiSystem
    test -d "$ROOT/ci-build/source/.git" || git clone git@github.com:alexclownfish/kpi-system.git "$ROOT/ci-build/source"
    git -C "$ROOT/ci-build/source" status --short
    git -C "$ROOT/ci-build/source" pull --ff-only
    git -C "$ROOT/ci-build/source" rev-parse HEAD
    grep -n 'scopeIdentity' "$ROOT/ci-build/source/server/cmd/migrate-sqlite-to-postgres/main.go"
    export KPI_SOURCE_DIR="$ROOT/ci-build/source"
    dc() { docker compose --project-directory "$ROOT" --env-file "$ROOT/.env" -f "$ROOT/docker-compose.yml" -f "$ROOT/docker-compose.postgres.yml" "$@"; }
    dc config --quiet
    dc ps -a
    docker volume ls --format '{{.Name}}' | grep '^kpi-system-pg_' || true
    test ! -e "$ROOT/secrets/postgres_migration_verified" || { echo '已存在迁移放行标记；先确认来源和是否曾启动应用'; exit 1; }

核对 `COMPOSE_PROJECT_NAME=kpi-system-pg`、`PG_USER=kpi_app`、`PG_ADMIN_USER=kpi_admin`、`BOOTSTRAP_ADMIN_EMAIL=`、`SEED_HONGXING_CATALOG=false`、`EVALUATION_WORKFLOW_MODE=audit`、`CORS_ALLOWED_ORIGINS=https://kpi.auracor.net`；确保数据库仅为容器内 `5432/tcp`，无宿主机公开端口。现有 `JWT_SECRET` 已暴露，暂不要回显它，按第 4 节在放行前轮换。确认旧服务和新 PG 不是同一个 Compose 项目/卷。

## 3. 仅建全 schema，然后只迁移九表数据

第 2 节的 `dc` 只作用于新 PG 项目。首先**验证目标数据库当前所有业务表均为空**；健康的 postgres 不代表空库。下面 SQL 仅检查，不删表不改数据；报错时必须停止排查，严禁自动清库。确认后构建含身份迁移功能的后端镜像（Dockerfile 中执行 Go 测试），再执行不 seed 的 `init-schema`；不能先启动普通后端，否则它会初始化默认业务数据。

    dc exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/kpi-admin-secrets/postgres_admin_password)"; exec psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' <<'SQL'
    DO $$ DECLARE t record; n bigint;
    BEGIN
      FOR t IN SELECT tablename FROM pg_tables WHERE schemaname='public' LOOP
        EXECUTE format('SELECT count(*) FROM public.%I',t.tablename) INTO n;
        IF n <> 0 THEN RAISE EXCEPTION '目标表 % 已有 % 行；停止', t.tablename,n; END IF;
      END LOOP;
    END $$;
    SQL
    test -s "$ROOT/secrets/postgres_password"
    dc exec -T --user 0 postgres sh -s < "$ROOT/ci-build/source/scripts/provision-postgres-role.sh"
    dc build backend
    dc run --rm --no-deps backend /app/kpi-server init-schema
    SOURCE_SHA256=$(awk '{print $1}' "$SNAP/sqlite.sha256")
    PG_DB=$(awk -F= '$1=="PG_DATABASE"{print $2}' "$ROOT/.env")
    PG_ADMIN=$(awk -F= '$1=="PG_ADMIN_USER"{print $2}' "$ROOT/.env")
    set -o pipefail
    dc run --rm --no-deps -v "$SNAP:/migration-source:ro" -v "$ROOT/pg-admin-secrets:/run/kpi-migration-admin:ro" -e PGUSER="$PG_ADMIN" -e PGPASSWORD_FILE=/run/kpi-migration-admin/postgres_admin_password backend /app/kpi-migrate-sqlite-to-postgres --scope identity --source /migration-source/kpi.sqlite.snapshot --source-sha256 "$SOURCE_SHA256" --expect-database "$PG_DB" --confirm-empty-target | tee "$ROOT/migration-report.log"

必须看到 `MIGRATION_RESULT=committed`、`MIGRATION_REPORT_JSON` 的 `scope=identity`，九表逐表行数、主键、内部外键、序列与源库一致，部门父级无循环，至少一个有效超级管理员。报告中的排除表源端行数是归档参考，**不是**迁入数据。任何失败都**不要创建迁移验收标记**、不要启动应用、不要清除目标 PG 卷；先分析失败并在独立空目标中重新演练。迁移程序只在一次性命令里挂载管理员密码；常规 backend 不挂载该目录。迁移需本次新 PG 实例的管理员权限以在事务中处理循环外键，绝不可指向已有共享数据库。

在普通 backend **首次启动前**，用以下只读 SQL 独立核对：所有非九表均为零行、源端/目标九表行数相同、有效超级管理员不少于 1。`init-schema` 建了全部表结构；如有某张应用表缺失，必须排查。下面检查不得在首次启动以后作为“所有非九表永远为零”的断言，因为系统默认设置和新登录审计可以产生**新记录**。

    dc exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/kpi-admin-secrets/postgres_admin_password)"; exec psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' <<'SQL'
    DO $$ DECLARE t record; n bigint;
    BEGIN
      FOR t IN SELECT tablename FROM pg_tables WHERE schemaname='public'
               AND tablename NOT IN ('departments','job_positions','employees','roles','permissions','user_roles','role_permissions','data_scopes','role_data_scopes') LOOP
        EXECUTE format('SELECT count(*) FROM public.%I',t.tablename) INTO n;
        IF n <> 0 THEN RAISE EXCEPTION '非迁移表 % 已有 % 行；停止', t.tablename,n; END IF;
      END LOOP;
      SELECT count(*) INTO n FROM employees e JOIN user_roles ur ON ur.user_id=e.id
        JOIN roles r ON r.id=ur.role_id WHERE r.code='super_admin' AND e.is_active=true;
      IF n < 1 THEN RAISE EXCEPTION '目标库没有有效超级管理员'; END IF;
    END $$;
    SELECT 'departments',count(*) FROM departments UNION ALL
    SELECT 'job_positions',count(*) FROM job_positions UNION ALL
    SELECT 'employees',count(*) FROM employees UNION ALL
    SELECT 'roles',count(*) FROM roles UNION ALL
    SELECT 'permissions',count(*) FROM permissions UNION ALL
    SELECT 'user_roles',count(*) FROM user_roles UNION ALL
    SELECT 'role_permissions',count(*) FROM role_permissions UNION ALL
    SELECT 'data_scopes',count(*) FROM data_scopes UNION ALL
    SELECT 'role_data_scopes',count(*) FROM role_data_scopes;
    SQL

## 4. 核验旧归档、轮换 JWT，再放行应用

旧签字材料**不复制**到目标签字卷，因为目标不含对应的旧考核与签字元数据。保留第 1 节的整个 `SNAP` 并异机备份；至少核对数据库摘要、签字文件数量/摘要，确认旧库与附件可在隔离环境恢复。若旧应用还有其他业务附件目录，须通过旧容器挂载与代码检查一并归档，不能仅凭默认路径断言全部附件已备齐。

    sha256sum -c "$SNAP/sqlite.sha256"
    if test -s "$SNAP/signoffs.sha256"; then (cd "$SNAP/signoffs" && sha256sum -c "$SNAP/signoffs.sha256"); fi
    test -s "$ROOT/migration-report.log"
    grep 'MIGRATION_RESULT=committed' "$ROOT/migration-report.log"
    grep '"scope":"identity"' "$ROOT/migration-report.log"

**放行前**在服务器本机轮换已在聊天中暴露的 `JWT_SECRET`。下面命令仅改 `.env`，不会把新值打印到终端；旧 JWT 会话全部失效，用户需重新登录。运行前确认已完成旧库/附件异机归档与第 3 节 SQL 验证。

    python3 - "$ROOT/.env" <<'PY'
    import pathlib, secrets, sys
    path = pathlib.Path(sys.argv[1])
    lines = path.read_text().splitlines()
    if sum(line.startswith('JWT_SECRET=') for line in lines) != 1:
        raise SystemExit('JWT_SECRET 配置不是唯一一行，未修改')
    replacement = 'JWT_SECRET=' + secrets.token_urlsafe(48)
    path.write_text('\n'.join(replacement if line.startswith('JWT_SECRET=') else line for line in lines) + '\n')
    path.chmod(0o600)
    print('JWT_SECRET 已轮换；旧会话需要重新登录（不显示密钥）')
    PY
    unset JWT_SECRET

仅当**九表核对、非九表零行、有效超级管理员、旧库及附件可恢复、JWT 轮换**全部完成，操作员才手工创建标记并启动应用：

    install -m 600 /dev/null "$ROOT/secrets/postgres_migration_verified"
    "$ROOT/ci-build/source/scripts/deploy-production.sh" --postgres --skip-pull
    curl -f http://127.0.0.1:8088/health
    dc ps

历史账号使用**原密码**登录，原密码哈希已在九表迁入，不应再执行首次管理员引导。先在 `EVALUATION_WORKFLOW_MODE=audit` 检查部门/岗位层级、用户角色、权限与数据范围，至少执行管理员允许和普通员工拒绝各一例。检查旧考核、模板和签字**不可见**；后端初次运行可新建系统设置/默认权限，登录及管理行为可产生新审计，因此上线后不能再要求“所有非九表永远零行”，而应确认这些行是新运行产生，旧历史记录未导入。当前 `SEED_HONGXING_CATALOG=false`：旧岗位模板不会自动出现在新库；若要发起新考核，需要另行配置或初始化模板并做新流程验收，不能将其作为身份迁移已完成的证据。完成页面验收与备份恢复后再决定是否将工作流从 audit 切为 enabled；不要为通过健康检查而提前开启。

## 5. 新库备份/恢复与网关切换

新 Compose 默认只在目标宿主机 127.0.0.1:8088 暴露统一 Nginx。若现有外部 API Gateway 与目标宿主机不是同一网络命名空间，不能继续使用原来的 172.16.230.14:13808（后端）和 :13003（前端）上游。应将 ROOT/.env 的 KPI_HTTP_BIND 改为目标宿主机**实际私网 IP**（如核实后仍是 172.16.230.14），然后再次运行 "$ROOT/ci-build/source/scripts/deploy-production.sh" --postgres --skip-pull，使运行中的网关以已发布镜像重建并检查该私网 IP:8088 的健康状态。将外部 Nginx 的 /api/、/health、/ 统一代理到该私网 IP:8088；只允许网关来源访问该端口。先 nginx -t，再平滑 reload。若网关直接运行于同一宿主机，可保留 127.0.0.1:8088。域名、TLS、CORS 来源必须一致。

在另一维护窗口冻结新后端写入，备份当前 PG 与**新**签字卷（可能为空）；这些备份不同于第 1 节封存的**旧** SQLite 与历史签字档案，两套均需离机保存。PG 归档不要落在唯一的数据盘上。下面的密码只在容器内部读取，命令行和输出均不显示明文：

    BACKUP=/mnt/datadisk0/project/kpiSystem/verified-backup/$(date -u +%Y%m%d-%H%M%S)
    install -d -m 700 "$BACKUP" "$BACKUP/signoffs"
    NEW_CID=$(dc ps -q backend)
    test -n "$NEW_CID"
    dc stop backend
    dc exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/kpi-admin-secrets/postgres_admin_password)"; exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$BACKUP/kpi.dump"
    docker cp "$NEW_CID":/app/uploads/evaluation-confirmations/. "$BACKUP/signoffs/"
    test -s "$BACKUP/kpi.dump"
    sha256sum "$BACKUP/kpi.dump" > "$BACKUP/kpi.dump.sha256"
    (cd "$BACKUP/signoffs" && find . -type f -print0 | sort -z | xargs -0 -r sha256sum) > "$BACKUP/signoffs.sha256"
    dc start backend

在另一个**独立** Compose 项目和空 PG 卷上演练 pg_restore；先核对不存在同名旧演练卷。恢复命令中的 -p 覆盖 .env 项目名，仅启动 postgres，不接入生产网关或生产卷：

    docker volume ls --format '{{.Name}}' | grep '^kpi-restore-check_' && { echo '演练卷已存在，先检查'; exit 1; }
    dr() { docker compose -p kpi-restore-check --project-directory "$ROOT" --env-file "$ROOT/.env" -f "$ROOT/docker-compose.yml" -f "$ROOT/docker-compose.postgres.yml" "$@"; }
    dr up -d --wait postgres
    dr exec -T --user 0 postgres sh -s < "$ROOT/ci-build/source/scripts/provision-postgres-role.sh"
    sha256sum -c "$BACKUP/kpi.dump.sha256"
    dr exec -T postgres sh -ec 'export PGPASSWORD="$(cat /run/kpi-admin-secrets/postgres_admin_password)"; exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --role="$KPI_APP_USER" --exit-on-error' < "$BACKUP/kpi.dump"
    dr run --rm --no-deps -v "$BACKUP:/backup:ro" --entrypoint sh backend -ec 'target=/app/uploads/evaluation-confirmations; cp -a /backup/signoffs/. "$target/"; cd "$target"; if test -s /backup/signoffs.sha256; then sha256sum -c /backup/signoffs.sha256; fi'

以**`pg_dump` 制作时**的 PG 表数量为基准核对隔离恢复库，不能机械对比迁移报告：应用启动后可以产生新系统设置、审计或新业务记录。核对员工、部门、岗位、角色、权限、数据范围及最新序列；确认旧考核/旧模板/旧签字仍未被导入。用 `kpi_app` 查询恢复后的业务表，确认对象所有权与日常权限。若新签字卷已有文件，恢复后哈希必须全部通过；目标签字卷为空也属正常。另在隔离环境测试第 1 节**旧 SQLite 全库与旧签字归档**可恢复，以确保历史资料和回退点仍在。不要在生产 PG 项目上执行恢复演练；不要使用 `dr up` 启动其它服务。两套备份均归档到异机。旧页面 SQLite SQL 备份/恢复接口在 PG 模式下会明确拒绝，不能替代以上完整归档。

## 6. 回退

切流前失败：保持旧 SQLite 全库和签字附件卷原样，排查目标 PG；不要删除旧卷。切流后若验证失败，先冻结新服务写入并记录新库备份，再把外部网关指回旧上游、启动旧 backend；旧库仍有旧考核、模板、签字历史，新库只迁了九表且可能已产生新写入，两边内容有意不同。切换后在新库发生的写入**不会自动回流**，须先确认差异并制定人工处理计划。不可执行 `docker compose down -v` 或删除旧数据库、旧签字卷。轮换 JWT 后的旧会话已失效，回退时亦需用户重新登录。

## 7. 构建阶段并发补签测试失败的恢复

2026-10-26 服务器后端镜像构建在 `RUN go test ./...` 的 `TestSignoffSupplementConcurrentAppendKeepsEachAttachment` 出现 `database table is locked`。这是测试夹具共享缓存内存 SQLite 的锁冲突；不是生产 PostgreSQL 缺表，也不是已上传的快照损坏。夹具已改成独立临时磁盘库并关闭连接，且并发测试无论成功失败都等待全部响应，避免后台请求污染后续测试。Dockerfile 的测试步骤保留。

按顺序更新源码、设置同一部署上下文并重建即可；不要重新复制 `.env`、重新导出快照或删除 PG 卷：

    git -C /mnt/datadisk0/project/kpiSystem/ci-build/source pull --ff-only
    ROOT=/mnt/datadisk0/project/kpiSystem
    SNAP="$ROOT/migration-archive/20261026-160647"
    export KPI_SOURCE_DIR="$ROOT/ci-build/source"
    dc() { docker compose --project-directory "$ROOT" --env-file "$ROOT/.env" -f "$ROOT/docker-compose.yml" -f "$ROOT/docker-compose.postgres.yml" "$@"; }
    dc config --quiet && dc build backend

构建通过后从第 3 节的 `dc run --rm --no-deps backend /app/kpi-server init-schema` 接着执行身份迁移；该命令仅创建结构。若此前迁移曾成功提交，必须先核实报告，不能将迁移命令重复作用于非空目标。失败构建本身不代表 schema/迁移已执行，不能据此创建放行标记。
