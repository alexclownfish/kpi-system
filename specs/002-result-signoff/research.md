# Research: 绩效结果打印签字与归档

## 决策 1：签字材料使用受保护文件存储

**Decision**: 文件写入 `SIGNOFF_UPLOAD_DIR`（默认 `./uploads/evaluation-confirmations`），数据库仅保存随机文件名和元数据，通过鉴权接口下载。

**Rationale**: 避免大二进制写入 SQLite，也避免签字材料经 `public/` 被匿名访问；适配现有 Docker 持久卷。

**Alternatives considered**: SQLite BLOB 会放大备份和锁竞争；公开静态目录不满足隐私要求；对象存储超出当前 MVP 部署范围。

## 决策 2：快照采用规范 JSON 与 SHA-256

**Decision**: 后端按稳定字段顺序构造快照 DTO，JSON 序列化后计算 SHA-256；相同校验码复用最新版本，变化后版本号递增。

**Rationale**: 可以证明签字关联的内容，且不依赖 Excel 文件本身作为事实来源。

**Alternatives considered**: 对 XLSX 文件哈希会因生成时间等元数据变化而不稳定；只存总分不足以证明逐项内容。

## 决策 3：在线确认使用专用接口

**Decision**: 新增在线确认接口，不再由前端通过通用 `PUT /evaluations/:id` 直接把状态改为 completed。

**Rationale**: 专用接口能验证本人、状态、异议、重复确认，并在一个事务中写入最终分、快照、确认记录和状态。

**Alternatives considered**: 扩展通用更新 handler 容易继续接受越权字段和非法状态跃迁。

## 决策 4：第一阶段只生成 XLSX 打印版

**Decision**: Excel 设置 A4、打印区域、适配宽度、重复表头和签字区域；不引入 PDF 生成依赖。

**Rationale**: 当前已使用 excelize，能快速交付可打印表格；PDF 生成需要字体打包和额外视觉验证。

**Alternatives considered**: HTML 转 PDF 需要浏览器运行时；Go PDF 库需要处理中文字体和分页。
