# Employee catalog contract

既有 GET /api/employees，需既有员工查看授权，保留 ApplyEmployeeScope。

Query：page 从 1 开始，pageSize=100；每页保留 is_active、department_id、search 等调用者过滤。评分职责仅 is_active=true，无角色过滤。

Response：data: Employee[]、total、page、pageSize、totalPages、hasNext、hasPrev。按 employees.id ASC；空 data 可为 null，totalPages=0。

employeeApi.getCatalog(params?: Omit<PaginationParams, 'page' | 'pageSize'>): Promise<Employee[]>。任意页失败整个调用 reject，绝不返回部分集合。保留 HTTP 401/403/500 行为。

UI：失败 Alert + 表格错误文案，刷新重试；加载/失败禁用新增、编辑、保存和员工选择。空名单有提示且禁止保存。

不新增接口、不扩大权限、不承诺并发人员变化下的事务快照。
