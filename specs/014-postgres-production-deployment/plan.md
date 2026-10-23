# Implementation Plan: PostgreSQL 生产迁移与冷部署

**Branch**: main | **Date**: 2026-10-23 | **Spec**: specs/014-postgres-production-deployment/spec.md

## Summary

保留旧线上 SQLite 全库及签字文件的一致性归档；在新 Compose 项目中先启动 PostgreSQL，仅创建目标全部业务表结构，再以一次性迁移程序的 `--scope identity` 模式仅复制、验证九张身份/组织/RBAC 表记录。其余表保持空表；启动正式应用前复核九表、有效超级管理员和零旧考核。失败时保留旧系统和旧卷。`--scope full` 作为既有全量迁移能力保留，但不用于本次生产执行。

## Technical Context

**Language/Version**: Go 1.23、TypeScript/Next.js 15、Bash、Docker Compose v2<br>
**Primary Dependencies**: GORM SQLite/PostgreSQL 驱动、Gin、PostgreSQL 16<br>
**Storage**: 原 SQLite 全库与签字归档；新 PostgreSQL 独立卷；目标签字卷初始为空<br>
**Testing**: go test ./...、bash -n、docker compose config、隔离库身份迁移演练、生产九表/空白业务/权限验收<br>
**Target Platform**: Linux 生产服务器和本地 macOS 开发环境<br>
**Project Type**: Go API + Next.js 前端 + Nginx 网关<br>
**Constraints**: 不得覆盖源数据库、旧 Docker 卷或附件；数据库端口不对公网开放；口令不写入 Git/日志；只迁九表记录但归档旧全库与附件；已曝光 JWT 须轮换<br>
**Scale/Scope**: 目标全 schema；数据仅九表 `departments`、`job_positions`、`employees`、`roles`、`permissions`、`user_roles`、`role_permissions`、`data_scopes`、`role_data_scopes`

## Constitution Check

- 原则 V：迁移前冻结旧服务写入，保留可恢复的 SQLite 全库和签字附件副本；先演练后切流；不使用 `down -v`。用户已明确只迁九表，不执行原先宪章要求的“全表与附件进入新库”验收；此为需显式记录的例外，而非全量迁移已完成。
- 原则 IV：数据库驱动、迁移工具、排序兼容、部署脚本和备份防误用均需测试；无法实测的门禁明确标为待线上验收，不以静态检查冒充。
- 原则 VI：在正式 server/ 与现有 Compose 上新增显式 PostgreSQL 模式，不建立第二套业务实现。
- 接口契约：保留既有 API；旧备份/恢复页面在 PostgreSQL 下明确拒绝并提示维护命令，不返回假成功。
- 设计复核：九表缺目标结构/字段、九表关系或行数不符、有效超级管理员缺失、普通应用首次启动前任何非九表有记录、源库/附件归档不可恢复时停止切换。启动后新增默认设置和审计须区分来源；用户范围例外详见 Complexity Tracking。

## Project Structure

- server/models/database.go：驱动选择、schema-only。
- server/main.go：init-schema 命令。
- server/cmd/migrate-sqlite-to-postgres/：一次性迁移工具，支持 `--scope identity` 与保留的 `--scope full`。
- server/handlers/backup.go：PostgreSQL 下禁止旧 SQLite 备份/恢复。
- server/handlers/template*.go：跨引擎排序兼容。
- docker-compose.yml 与 docker-compose.postgres.yml：基础 Compose 和 PG overlay。
- scripts/deploy-production.sh：空目录拉取及分阶段启动。
- specs/014-postgres-production-deployment/：规格、设计、任务及验收。
- docs/production-postgres-migration.md：生产迁移、备份、回退 runbook。

**Structure Decision**: 复用正式应用源，不改前端业务接口；本次迁移工具只复制九表记录，旧签字文件只归档/核验，不复制进新业务卷。新系统不显示旧考核及签字；新考核模板需另行配置或初始化后才可使用。

**数据库角色隔离**: `kpi_admin` 的口令仅在 PostgreSQL 容器与一次性迁移容器可见；应用后端使用独立的非超级用户 `kpi_app`。因正式服务仍运行 AutoMigrate，该应用角色暂需 `public` schema 建表权限；不能误称为纯读写账号。隔离恢复使用 `pg_restore --no-owner --role=kpi_app` 保持表归属与后端权限一致。

## Sequencing

1. 预检旧服务/卷，冻结写入，制作可恢复的旧 SQLite 全库和签字附件归档，并离机保存。
2. 保持当前独立 Compose 项目的 PostgreSQL 服务运行；检查目标库为空，执行 schema-only 初始化，无 seed、无应用服务。
3. 在空目标库事务中执行 `--scope identity`；逐表核对九表行数、主键、关系、序列、有效超级管理员；在普通后端首次启动前确认排除表零记录。旧库非迁移表的孤儿外键不应误拦九表迁移，但九表内部关系必须完整。
4. 本机轮换已曝光的 JWT 密钥（旧会话失效）；人工核对归档和迁移报告后才创建放行标记。先以 audit 模式启动，验证原账号/权限与旧考核不可见；`SEED_HONGXING_CATALOG=false` 表示旧模板不会自动进入新库。
5. 新库执行 `pg_dump`/隔离恢复并核对；旧库与附件归档也须可恢复。再切外部 Nginx，保留旧环境为回退点。

## Complexity Tracking

**宪章 V 显式例外**：宪章要求切换引擎时完成全表、关联、序列和附件的迁移验收；用户在 PostgreSQL 容器已启动后明确修订为“仅用户数据、必要表结构、角色权限”，即九表记录进新库、其他旧记录（包括考核和附件）不进入。原因是用户希望以新业务起点上线，同时保留登录与权限。风险：新系统看不到旧考核、模板、签字与审计，旧库/附件归档成为唯一历史来源；回退后新库新写入不能自动合并。替代方案：沿用 `--scope full` 完整迁移，不符合用户当前选择。批准结论：只对本次迁移接受用户明确限定的九表范围，条件是完整封存且可恢复旧库与附件、逐项核验九表/空白业务、上线前明示历史数据不在新库；若无法满足，禁止切流。此例外不修改宪章正文或未来迁移默认义务。
