# Quickstart: 迁移验收入口

1. 阅读 docs/production-postgres-migration.md，先确认旧服务路径、Compose 项目名、旧数据库和签字卷，并取得一致性副本。
2. 从发布提交克隆仓库到 ci-build/source；运行 scripts/deploy-production.sh --postgres --db-only，仅启动 PG。不要直接运行 docker compose up -d --build。
3. 构建后端镜像；执行 kpi-server init-schema；对 SQLite 完整副本运行迁移程序。逐表行数、关系与序列报告不全绿即停止。
4. 将签字附件复制进新卷，对比文件数和哈希；确认历史签字元数据与文件均在。只有通过后才创建迁移验收标记并启动应用。
5. 用原账号登录，查看历史考核与签字；新建隔离测试考核走评分、员工确认、签字、导出；检查角色允许/拒绝场景。
6. 对 PG 和签字目录做备份，并在隔离环境恢复；容器重建后抽样数据和附件。记录结果后再切外部网关。

验收阻断：没有旧库全量文件、附件快照、迁移逐表核对或可恢复备份时，不应切换生产流量。全部可复制命令及回退步骤见生产迁移手册。
