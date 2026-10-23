# Implementation Plan: PostgreSQL 生产迁移与冷部署

**Branch**: main | **Date**: 2026-10-23 | **Spec**: specs/014-postgres-production-deployment/spec.md

## Summary

保留旧线上 SQLite 全库及签字文件的一致性副本；在全新 Compose 项目中先启动 PostgreSQL，仅创建目标 schema，再以一次性迁移程序逐表复制、验证并提交数据；最后启动正式应用、切换网关、演练完整业务及备份。失败时保留旧系统，不删除旧卷。SQLite 仍是现有开发环境的默认引擎，生产 PostgreSQL 必须显式启用且连接失败不得回退。

## Technical Context

**Language/Version**: Go 1.23、TypeScript/Next.js 15、Bash、Docker Compose v2<br>
**Primary Dependencies**: GORM SQLite/PostgreSQL 驱动、Gin、PostgreSQL 16<br>
**Storage**: 原 SQLite 文件；新 PostgreSQL 独立卷；签字、导出、备份各自持久卷<br>
**Testing**: go test ./...、bash -n、docker compose config、隔离库迁移演练、真实业务验收<br>
**Target Platform**: Linux 生产服务器和本地 macOS 开发环境<br>
**Project Type**: Go API + Next.js 前端 + Nginx 网关<br>
**Constraints**: 不得覆盖源数据库、旧 Docker 卷或附件；数据库端口不对公网开放；口令不写入 Git/日志；用户明确要求迁移真实数据<br>
**Scale/Scope**: 所有非 SQLite 系统表、索引/主键与关系、业务附件和全流程验证

## Constitution Check

- 原则 V：迁移前冻结旧服务写入，保留可恢复的 SQLite 一致性副本和附件副本；先演练后切流；不使用 down -v。
- 原则 IV：数据库驱动、迁移工具、排序兼容、部署脚本和备份防误用均需测试；无法实测的门禁明确标为待线上验收，不以静态检查冒充。
- 原则 VI：在正式 server/ 与现有 Compose 上新增显式 PostgreSQL 模式，不建立第二套业务实现。
- 接口契约：保留既有 API；旧备份/恢复页面在 PostgreSQL 下明确拒绝并提示维护命令，不返回假成功。
- 设计复核：上述设计符合宪章；若源库表没有目标对应结构、迁移核对不符或生产环境无法接入旧卷，则停止切换。

## Project Structure

- server/models/database.go：驱动选择、schema-only。
- server/main.go：init-schema 命令。
- server/cmd/migrate-sqlite-to-postgres/：一次性全量迁移工具。
- server/handlers/backup.go：PostgreSQL 下禁止旧 SQLite 备份/恢复。
- server/handlers/template*.go：跨引擎排序兼容。
- docker-compose.yml 与 docker-compose.postgres.yml：基础 Compose 和 PG overlay。
- scripts/deploy-production.sh：空目录拉取及分阶段启动。
- specs/014-postgres-production-deployment/：规格、设计、任务及验收。
- docs/production-postgres-migration.md：生产迁移、备份、回退 runbook。

**Structure Decision**: 复用正式应用源，不改前端业务接口；一次性迁移工具只负责数据库，附件作为文件卷单独复制与校验。

**数据库角色隔离**: `kpi_admin` 的口令仅在 PostgreSQL 容器与一次性迁移容器可见；应用后端使用独立的非超级用户 `kpi_app`。因正式服务仍运行 AutoMigrate，该应用角色暂需 `public` schema 建表权限；不能误称为纯读写账号。隔离恢复使用 `pg_restore --no-owner --role=kpi_app` 保持表归属与后端权限一致。

## Sequencing

1. 预检旧服务、旧卷及数据库版本，取得旧库和附件的可恢复副本。
2. 在独立 Compose 项目下仅启动 PostgreSQL；执行 schema-only 初始化，无 seed、无应用服务。
3. 在空目标库事务中迁移所有源业务表；核对行数、关系、序列，失败整批回滚。
4. 恢复签字附件并核对；再构建和启动应用，验证历史登录、考核、模板、导出。
5. 生成 PostgreSQL 与附件备份并演练恢复；切换外部 Nginx；保留旧环境作为回退点。

## Complexity Tracking

无宪章例外。首次部署的 --db-only 阶段是保留旧数据所必需；不能以直接 compose up 替代。
