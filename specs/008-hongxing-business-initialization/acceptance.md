# 荭杏业务初始化验收 (2026-10-01)

## Scope and Spec Kit

完成 specify → clarify（无阻塞歧义）→ plan → tasks → analyze → implement → converge。
规格清单16/16，初始8条需求覆盖10任务；Converge补充并完成4个关联任务，共14项。
最终核对8条需求、9个验收场景、4个成功标准、7项设计决定及6条适用宪章原则，无本增量未满足项。不代表feature006/007的其余任务已完成。

## Source

只读核对 `/Users/simon/Downloads/荭杏研发项目绩效考核表.xlsx`。
SHA256: `c3f8749e6ebc046cff88a3b5d052ee7a81fad4766c2678907e6d0be01b6997c0`。
原表九部门 × 部门负责人/工程师，共十八模板、138指标。重新提取与内嵌JSON完全一致。
JSON不含 H列评分、synthetic、员工、考核或职责分配。保留维度/指标权重、评分人映射及原文指引，按方案A校验。

## Automated evidence

- 初始新增测试在实现前因缺少初始化功能失败；部门保护测试复现有模板的部门被错误删除（200），修复后通过。
- `go test ./...` 全套通过；新目录测试覆盖九部门十八岗位十八模板、138指标逐项校验、发布校验、无员工/考核/职责、重复初始化、HR改名停用删除保护、复用已存在模板、事务回滚、重名歧义、显式关闭和非法开关、岗位查询授权及异常、部门删除清理/关联保护。
- `tests/unit/department-catalog.test.mjs`：205部门跨三页完整加载、参数符合后端上限、错误传播通过。
- `tests/unit/template-weights.test.mjs`：十八源模板与小数舍入、范围、综合合计通过。
- TypeScript、定向ESLint、最终production build通过；仅保留模板页原有 fetchTemplates effect依赖警告。
- `docker compose config --quiet`、`git diff --check`通过。

## Actual localhost operation

操作前备份：`secrets/local-before-hongxing-20261001.db`（权限600，被Git和Docker构建忽略）。
使用已测试的临时 `init-business` 程序补入日常Compose现有库，不替换运行中的服务、不重建镜像、不修改.env。

首次恢复时宿主机 secrets 新文件未出现在既有容器挂载目录；改用容器临时凭据副本完成已授权恢复，随后移除容器临时文件。保留本地交付凭据 `secrets/local-admin-recovery-20261001.password`（权限600）供用户登录后更换密码，不在报告或日志记录密码。

真实 `/api/auth/login`：200，`yangwenzhe@howxing.com`，role=super_admin；role:edit/system:edit存在，/me、部门、员工、十八模板详情读取成功。
重复运行时给出不存在的初始化密码文件，仍成功；相同临时密码再次登录200，表明已有有效管理员不重置密码、不重复初始化。

当前普通数据库：14部门（4旧部门+9原表部门+系统管理）、18岗位、21模板（3旧+18原表）、138个新增指标、8员工（7原有+1恢复管理员）。原有7员工姓名、邮箱、密码哈希、角色、部门及在职状态与备份相同；未删除旧员工。未来不会重新创建演示员工。

浏览器保留原HR会话，localhost/templates按结构部门过滤，负责人及工程师两套原表模板可见。
截图：`/private/tmp/kpi-login-diagnosis.TXJpug/localhost-hongxing-templates.png`。
不把API登录验证描述为已切换浏览器管理员会话。

## Deployment boundary

日常backend健康，frontend和gateway运行，入口80/localhost。停止旧18092前端、18089隔离后端及编译容器；保留测试数据和证据。
用户需再执行 `docker compose up -d --build` 才加载本轮新增岗位接口/表单建议、完整部门下拉及删除保护代码。本轮没有代用户重建普通服务；这些新UI交互的日常运行时验收尚待重建后完成，不能将编译成功当成页面交互验证。

不启用新的流程写入开关、不生成真实评分职责、不迁移或改写历史考核。请参考quickstart进行页面验证。
