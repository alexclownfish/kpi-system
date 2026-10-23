# Tasks: PostgreSQL 生产迁移与冷部署

**Input**: spec.md、plan.md、research.md、data-model.md、contracts/migration-cli.md

## Phase 1: 基础与设计

- [x] T001 [P] 明确旧库、附件、回退和无生产清库约束，写入 specs/014-postgres-production-deployment/spec.md。
- [x] T002 [P] 完成 PostgreSQL 驱动、迁移、备份和部署决策，写入 specs/014-postgres-production-deployment/plan.md、research.md、data-model.md。
- [x] T003 对照规格与宪章执行 Spec-Kit Analyze，修复 CRITICAL/HIGH 冲突并记录结果。

## Phase 2: US1 保留历史数据切换数据库 (P1)

- [x] T004 [US1] 在 server/models/database.go 与 server/main.go 实现显式 PG 连接和无种子 init-schema，单测覆盖口令、失败不回退和空库。
- [x] T005 [US1] 在 server/cmd/migrate-sqlite-to-postgres/ 实现全部源业务表的事务迁移、类型处理、目标空库保护、逐表行数和序列校验。
- [ ] T006 [US1] 在 Dockerfile.backend 打包一次性迁移程序；在隔离卷用真实源库副本演练，确认所有表/关联、ID、权限、历史考核和审计记录。
- [ ] T007 [US1] 将签字卷复制、文件数量/哈希核对、失败回退步骤写入 docs/production-postgres-migration.md，并在生产旧环境停写后执行。

## Phase 3: US2 从空目录重新部署 (P1)

- [x] T008 [P] [US2] 在 docker-compose.postgres.yml 配置独立 PG 卷、密码文件、内网服务及健康依赖，验证 docker compose config。
- [x] T009 [US2] 在 scripts/deploy-production.sh 支持空目录 Git clone、--postgres --db-only 和迁移验收标记；模拟测试覆盖提前启动拦截。
- [x] T010 [US2] 在 server/handlers/template.go 和 template_calculation.go 修复 PostgreSQL 字段排序引用；检查其他核心路径的 SQLite 专属 SQL。
- [ ] T011 [US2] 在发布提交后于目标服务器只启动 PG、执行 schema-only 与迁移、恢复签字文件，再启动应用并验证 /health。

## Phase 4: US3 新库业务与备份验收 (P2)

- [x] T012 [US3] 在 server/handlers/backup.go 禁止 PostgreSQL 使用旧 SQLite 备份及恢复 API，验证返回明确错误。
- [x] T013 [US3] 在 docs/production-postgres-migration.md 写出 pg_dump/pg_restore、附件备份、隔离恢复与回退手册。
- [ ] T014 [US3] 在 PostgreSQL 环境进行历史登录、岗位模板、创建考核、评分、确认、签字、导出与权限正反向验收。
- [ ] T015 [US3] 验证业务及附件持久卷在容器重建后保留，记录迁移与备份恢复证据。

## Phase 5: 质量与收敛

- [x] T016 执行 go test ./...、tsc --noEmit、Next.js production build、bash -n、docker compose config；不具备环境时记录未通过门禁。
- [ ] T017 执行 Spec-Kit Converge，对遗留缺口追加任务并完成或明确生产切换阻断项；待真实旧库迁移和生产验收后运行。

## Phase 6: 真实数据与生产收口

- [ ] T018 [US1] 获取旧线上 SQLite 与签字卷的真实只读副本，核对活跃管理员、全量表和附件引用；停止写入后制作独立快照并异机备份。
- [ ] T019 [US1] 用 T018 的真实副本在隔离 PostgreSQL 实例完整演练迁移，逐表比较、抽样历史考核与签字，记录异常修复和回归证据。
- [ ] T020 [US2] 审核发布范围，提交并推送受测 commit；在目标服务器核实旧容器/卷、网关和备份路径后按手册迁移、验收、切流与回退演练。

**证据（2026-10-23）**: Dockerfile 的 `go test ./...`、TypeScript 检查、Next.js 生产构建、部署脚本模拟测试及真实 PostgreSQL 16 隔离演练通过。演练覆盖合成历史考核/评分/确认/补签、36 张表行数与外键/序列、附件哈希、重复迁移拒绝、受限应用账号启动、PG dump/隔离恢复。T006/T007/T011/T014/T015 仍需要**真实旧线上副本或生产权限**，不得以合成数据和健康检查代替。

**依赖**: T003 先于生产实施；T004/T008 先于 T005；T018/T019 先于 T011/T020；T011 先于 T014/T015。生产 T011–T015 需要旧线上数据和操作权限，不能仅凭本地静态检查勾选。

## Phase 7: 用户修订范围 — 仅身份/组织/RBAC 迁移

**变更说明（2026-10-23）**: 用户明确选择新 PG 只迁移九张表记录（`departments`、`job_positions`、`employees`、`roles`、`permissions`、`user_roles`、`role_permissions`、`data_scopes`、`role_data_scopes`），其余表仅建结构且旧记录为零。T006/T007/T014/T019 中要求的“历史考核/签字在新库可见”不再是本次上线门禁；保留旧任务原文作为此前全量方案的历史记录，不视作完成或默认执行。T021–T027 为本次实际门禁。旧 SQLite 全库及签字仍须归档可恢复；宪章 V 例外记录于 plan.md。

- [x] T021 [P] 修订 specs/014-postgres-production-deployment/ 的规格、方案、迁移契约、验收步骤、风险分析及 docs/production-postgres-migration.md，明确仅九表、全部结构、旧库/附件归档、JWT 轮换和宪章 V 例外。
- [x] T022 [US1] 在 server/cmd/migrate-sqlite-to-postgres/ 实现 `--scope identity` 显式九表白名单，保留 `--scope full`；九表源结构、字段、主键、内部 FK、序列及有效超级管理员失败关闭；排除表遗留 FK 异常仅报告、不误拦。
- [x] T023 [US1] 用 go test 和隔离 PostgreSQL 16 演练九表复制、密码哈希及角色/数据范围一致、非九表零记录、排除表外键孤儿容忍、九表 FK 异常拒绝、空目标/重复迁移保护。
- [ ] T024 [US1] 在旧线上停写后取得真实 SQLite 全库和签字快照及异机副本，核对摘要、旧有效超级管理员与附件引用；记录旧 Compose/卷/网关基线并完成旧档案隔离恢复。此任务不等于把旧考核导入新库。
- [ ] T025 [US2] 核查已健康的新 PostgreSQL 容器对应项目、数据库和全部空表；在未启动普通 backend 前执行 schema-only 与真实 `--scope identity`，逐项核对九表行数/ID/关系/序列、有效超级管理员及非九表零记录；未全部通过不创建 `postgres_migration_verified`。首启后的新设置/审计不得误判为旧记录。
- [ ] T026 [US3] 上线前在服务器本机轮换已暴露 JWT 密钥（不输出明文，告知现有会话失效）；放行后用原账号执行权限允许/拒绝、部门岗位和数据范围验收，确认无旧模板/考核，演练新 PG 备份/隔离恢复、旧档案恢复和回退。
- [ ] T027 对上述本次范围重新执行 Spec-Kit Analyze/Converge 与发布门禁，记载真实生产证据；完成后再切网关，不把 healthy、合成测试或旧全量任务的结果冒充仅身份验收。

**依赖**: T021–T023 先于 T025；T024 先于 T025/T026；T025 和 T026 先于切流及 T027 收口。`SEED_HONGXING_CATALOG=false` 时新库无旧模板，新考核流程需另行配置模板后验收，不作为九表迁移已满足项。

## Phase 8: Convergence

- [x] T028 对 FR-007/US3-AC1 的部门层级执行身份迁移前后无父级循环校验，并增加自引用及双部门成环反例；发现循环须在提交前失败，不以仅存在外键目标视为组织层级可用（partial）。

**本次范围验证证据（2026-10-23）**：Dockerfile 的全部 Go 测试通过；隔离 PostgreSQL 16 的 full 与 identity 模式回归通过。身份模式复制九表，确认非九表无旧记录、密码哈希及 `access_customized` 保持、序列正确；拒绝非空目标、缺有效超管、孤儿部门与父级循环；忽略并报告非迁移历史表的孤儿。身份库首次启动后，旧密码登录管理员和普通员工，验证 ALL/SELF、角色接口 200/403，启动前后角色权限与数据范围摘要一致。完整运行镜像迁移演练先已通过；新增登录/层级反例采用 `KPI_TEST_IMAGE_TARGET=builder`（显式运行相同 Go 二进制）避开 Chromium 包镜像下载故障，不作为 PDF 或前端验收。T024–T027 仍待真实生产操作，未提前放行。
