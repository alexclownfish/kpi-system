# Research

- Decision: 嵌入纯业务 JSON，不运行外部 Excel 文件或固定 ID SQL。Rationale: 克隆部署无需 Downloads 文件，复用业务校验。
- Decision: 一次事务及唯一记录。Rationale: 防止启动覆盖 HR 修改，改名或删除不会生成重复项。
- Decision: 部门岗位目录与自由文本兼容。Rationale: 直接提供原表选项，不强制迁移旧员工。
- Decision: 保留旧员工，删除演示创建代码。Rationale: 不再初始化不等于删除所有存量人员。
- Decision: 用户本地恢复仅使用 localhost 现有卷。Rationale: 不混合测试用户与日常数据。

无阻塞设计未知项；无需外部研究。
