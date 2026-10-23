# 一次性迁移 CLI 契约

输入：只读 SQLite 一致性副本路径及 SHA-256、显式目标库名和空库确认；PostgreSQL 连接参数沿用 `PGHOST`、`PGPORT`、`PGUSER`、`PGDATABASE`、`PGPASSWORD_FILE`。本次生产必须显式传入 `--scope identity`，只复制九表 `departments`、`job_positions`、`employees`、`roles`、`permissions`、`user_roles`、`role_permissions`、`data_scopes`、`role_data_scopes`；默认/既有 `--scope full` 仅供另行批准的全量迁移使用。目标全 schema 已由受限应用角色初始化且所有业务表为空；一次性迁移容器独立挂载管理员口令，常规后端绝不持有该口令。

输出：报告 `scope=identity`、九表各自行数、排除表源端行数、范围内主键/外键/序列校验、部门父级无循环和有效超级管理员校验；`MIGRATION_RESULT=committed` 或非零退出码。报告及日志不得包含明文凭据和密码哈希。操作员还须在普通后端首启前独立查询目标九表数量、全部非九表零行及活跃 `super_admin`，通过后才创建迁移验收标记。

拒绝条件：源库不可读/摘要不符、目标任一业务表已有行、任一范围内源表/列无法映射、九表数据值/约束不能迁移、九表行数/关联/序列/有效超级管理员校验失败。仅身份模式应报告被排除表中的历史 FK 异常，但不能用其误拦九表；九表内部异常仍须拒绝。失败后不启动应用；不得自动删除旧库或覆盖目标中已有数据。旧 SQLite 全库和签字附件须另行归档、核验，绝不能将本模式描述为全量历史迁移。

备份 API：PostgreSQL 模式的旧页面创建备份及恢复操作返回明确错误；全量备份/恢复改由维护窗口执行命令，而非把旧 SQLite SQL 文件当作 PG 备份。
