# 生产环境从 SQLite 全量迁移到 PostgreSQL

适用场景：线上目标目录 /mnt/datadisk0/project/kpiSystem 是空目录，但旧生产 SQLite、员工、考核和签字附件必须保留。本手册是**停写迁移**，不是直接在空目录运行 docker compose up。以下命令在目标 Ubuntu 服务器执行；先在测试机用旧库副本演练一遍。旧 Compose 项目和卷在验收完成前不得删除。

## 0. 发布与旧环境预检

本次 PG 代码、一次性迁移程序及部署脚本必须先经测试、提交并推送到 Git；当前本地未提交改动不会被服务器 git clone 获取。记录并锁定发布 commit。确认 SSH 可读 git@github.com:alexclownfish/kpi-system.git。

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

用 Python 标准库 SQLite backup API 从停止写入的完整目录再生成单文件一致性快照；迁移程序拒绝带 -wal/-shm 的直接拷贝。若旧目录或附件位置不同，先按 docker inspect 的实际挂载修正，不要运行空目录迁移。

    python3 - "$SNAP/sqlite-dir/kpi.db" "$SNAP/kpi.sqlite.snapshot" <<'PY'
    import sqlite3, sys
    from urllib.parse import quote
    src = sqlite3.connect('file:' + quote(sys.argv[1]) + '?mode=ro', uri=True)
    dst = sqlite3.connect(sys.argv[2])
    src.backup(dst)
    check = dst.execute('PRAGMA integrity_check').fetchone()[0]
    orphans = dst.execute('PRAGMA foreign_key_check').fetchall()
    dst.close(); src.close()
    if check != 'ok' or orphans:
        raise SystemExit('SQLite integrity/FK check failed')
    PY
    sha256sum "$SNAP/kpi.sqlite.snapshot" > "$SNAP/sqlite.sha256"
    (cd "$SNAP/signoffs" && find . -type f -print0 | sort -z | xargs -0 -r sha256sum) > "$SNAP/signoffs.sha256"
    sha256sum -c "$SNAP/sqlite.sha256"
    find "$SNAP/signoffs" -type f | wc -l

在快照中确认原超级管理员可继续登录，以及所有数据库引用的纸质签字文件都已进入快照。若任一检查失败，先查明旧服务的实际挂载，不要启动迁移：

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

## 2. 从空目标目录拉代码，只启动 PostgreSQL

以下目标根路径在首次部署时不得已有不明数据。若发现同名 Compose 卷，先停下查明来源。

    ROOT=/mnt/datadisk0/project/kpiSystem
    mkdir -p "$ROOT/ci-build"
    git clone git@github.com:alexclownfish/kpi-system.git "$ROOT/ci-build/source"
    git -C "$ROOT/ci-build/source" rev-parse HEAD
    docker volume ls --format '{{.Name}}' | grep -E 'kpi-system(-pg)?_' || true
    "$ROOT/ci-build/source/scripts/deploy-production.sh" --postgres --db-only --skip-pull --domain https://kpi.auracor.net

脚本在 ROOT 下创建 .env、docker-compose.yml、docker-compose.postgres.yml、secrets/postgres_password（受限应用角色）与 pg-admin-secrets/postgres_admin_password（仅 PG 容器和一次性迁移容器可读），只启动 postgres，并创建受限应用角色。使用独立 Compose 项目 kpi-system-pg；数据库没有宿主机公开端口。检查 .env 的 COMPOSE_PROJECT_NAME=kpi-system-pg、PG_USER=kpi_app、PG_ADMIN_USER=kpi_admin、BOOTSTRAP_ADMIN_EMAIL 留空、SEED_HONGXING_CATALOG=false、EVALUATION_WORKFLOW_MODE=audit、CORS_ALLOWED_ORIGINS=https://kpi.auracor.net；不要把密码文件内容发到聊天或日志。

## 3. 仅建 schema，然后迁移全部业务表

下面的 dc 函数只作用于新 PG 项目。先构建后端镜像（Dockerfile 中执行 Go 测试），再执行不 seed 的 schema 命令；不能先启动普通后端，否则它可能初始化业务数据。

    export KPI_SOURCE_DIR="$ROOT/ci-build/source"
    dc() { docker compose --project-directory "$ROOT" --env-file "$ROOT/.env" -f "$ROOT/docker-compose.yml" -f "$ROOT/docker-compose.postgres.yml" "$@"; }
    dc config --quiet
    dc build backend
    dc run --rm --no-deps backend /app/kpi-server init-schema
    SOURCE_SHA256=$(awk '{print $1}' "$SNAP/sqlite.sha256")
    PG_DB=$(awk -F= '$1=="PG_DATABASE"{print $2}' "$ROOT/.env")
    PG_ADMIN=$(awk -F= '$1=="PG_ADMIN_USER"{print $2}' "$ROOT/.env")
    set -o pipefail
    dc run --rm --no-deps -v "$SNAP:/migration-source:ro" -v "$ROOT/pg-admin-secrets:/run/kpi-migration-admin:ro" -e PGUSER="$PG_ADMIN" -e PGPASSWORD_FILE=/run/kpi-migration-admin/postgres_admin_password backend /app/kpi-migrate-sqlite-to-postgres --source /migration-source/kpi.sqlite.snapshot --source-sha256 "$SOURCE_SHA256" --expect-database "$PG_DB" --confirm-empty-target | tee "$ROOT/migration-report.log"

必须看到 MIGRATION_RESULT=committed 和 MIGRATION_REPORT_JSON，逐表行数与源库一致、外键与序列核对无错误。任何失败都**不要创建迁移验收标记**、不要启动应用、不要清除目标 PG 卷；先分析失败并重新在独立空目标中演练。迁移程序只在这一条一次性命令里挂载管理员密码；常规 backend 不挂载该目录。迁移需本次新 PG 实例的管理员权限以在事务中处理循环外键，绝不可指向已有共享数据库。

## 4. 复制并验证签字附件

先确认新签字卷为空；在后端一次性容器中复制文件并按旧快照 manifest 逐一比对哈希、数量。这个命令不会启动应用服务器。

    dc run --rm --no-deps -v "$SNAP:/migration-source:ro" --entrypoint sh backend -ec 'target=/app/uploads/evaluation-confirmations; test -z "$(find "$target" -mindepth 1 -print -quit)"; cp -a /migration-source/signoffs/. "$target/"; source_count=$(find /migration-source/signoffs -type f | wc -l); target_count=$(find "$target" -type f | wc -l); test "$source_count" -eq "$target_count"; cd "$target"; if test -s /migration-source/signoffs.sha256; then sha256sum -c /migration-source/signoffs.sha256; fi'

检查历史签字记录引用的文件确实存在。若旧应用还有其他业务附件目录，也须通过旧容器挂载与代码检查列入快照和恢复，不能仅凭本手册默认路径断言全部附件已备齐。

确认迁移报告与附件核对结果后，由操作员手动放行：

    install -m 600 /dev/null "$ROOT/secrets/postgres_migration_verified"
    "$ROOT/ci-build/source/scripts/deploy-production.sh" --postgres --skip-pull
    curl -f http://127.0.0.1:8088/health
    dc ps

历史账号应使用**原密码**登录；迁移保留密码哈希，不应再执行首次管理员密码引导。先在 EVALUATION_WORKFLOW_MODE=audit 检查历史员工、角色、模板、考核、审计和签字。确认没有启动时自动种子或修复带来的异常后，再把 ROOT/.env 的该值改为 enabled 并重建 backend 服务：

    sed -i 's/^EVALUATION_WORKFLOW_MODE=audit$/EVALUATION_WORKFLOW_MODE=enabled/' "$ROOT/.env"
    grep '^EVALUATION_WORKFLOW_MODE=' "$ROOT/.env"
    "$ROOT/ci-build/source/scripts/deploy-production.sh" --postgres --skip-pull

验收新考核：岗位自动匹配模板、项目经理/产品经理等评分职责、各必填指标、加权总分、员工确认、签字材料、PDF/表格导出和权限允许/拒绝。重建容器后再抽样历史数据与附件。未完成这些步骤时不要切换公网流量。

## 5. 网关切换与备份恢复演练

新 Compose 默认只在目标宿主机 127.0.0.1:8088 暴露统一 Nginx。若现有外部 API Gateway 与目标宿主机不是同一网络命名空间，不能继续使用原来的 172.16.230.14:13808（后端）和 :13003（前端）上游。应将 ROOT/.env 的 KPI_HTTP_BIND 改为目标宿主机**实际私网 IP**（如核实后仍是 172.16.230.14），然后再次运行 "$ROOT/ci-build/source/scripts/deploy-production.sh" --postgres --skip-pull，使运行中的网关以已发布镜像重建并检查该私网 IP:8088 的健康状态。将外部 Nginx 的 /api/、/health、/ 统一代理到该私网 IP:8088；只允许网关来源访问该端口。先 nginx -t，再平滑 reload。若网关直接运行于同一宿主机，可保留 127.0.0.1:8088。域名、TLS、CORS 来源必须一致。

在另一维护窗口冻结新后端写入，备份 PG 和签字卷；PG 归档不要落在唯一的数据盘上。下面的密码只在容器内部读取，命令行和输出均不显示明文：

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

以迁移报告为基准核对恢复库逐表行数，再查员工、模板、考核、签字关联与序列，并用 kpi_app 查询恢复后的业务表以确认对象所有权与日常权限。恢复后的签字文件哈希必须全部通过。不要在生产 PG 项目上执行恢复演练；不要使用 dr up 启动其它服务。把备份和签字文件归档到异机。旧页面 SQLite SQL 备份/恢复接口在 PG 模式下会明确拒绝，不能替代以上完整归档。

## 6. 回退

切流前失败：保持旧 SQLite 和附件卷原样，排查目标 PG；不要删除旧卷。切流后若验证失败，先冻结新服务写入并记录新库备份，再把外部网关指回旧上游，启动旧 backend；旧库只包含切换前数据，切换后在新库发生的写入**不会自动回流**，须先确认可接受的数据差异并制定人工处理计划。不可执行 docker compose down -v 或删除旧数据库、旧签字卷。
