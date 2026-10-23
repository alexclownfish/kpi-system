# Data Model

- JobPosition: ID, DepartmentID (`not null`), Name (`not null`), CreatedAt, UpdatedAt；DepartmentID/Name 唯一；无员工、密码或权限绑定。
- BusinessInitialization: Key (`primary key`), SourceSHA256, CreatedAt。Key=`hongxing-catalog-v1`。与整个目录同事务。
- 复用 Department、KPITemplate、KPIDimension、KPIItem、AuditLog。模板 monthly/version1/published/weighted-v2，导出默认 portrait。
- 状态：无记录 → 验证定义 → 原子创建或复用 → 写记录与审计；错误回滚。已有记录直接退出，不改变数据。
