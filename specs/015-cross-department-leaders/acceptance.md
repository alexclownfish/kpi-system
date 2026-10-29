# Feature 015 验证记录

日期：2026-10-28。验证对象：当前工作区源码，尚未部署线上。

## 当前交付

- 一个在职账号可兼任多个明确指定部门的负责人；创建部门即可任命。
- 所属部门、岗位和角色权限保持原样；普通员工可选择明确任命的跨部门负责人为直属上级。
- 模板的部门负责人和 CTO 职责独立解析；同一人同一指标的两种职责生成两条任务，仍按既定模板公式计算。
- DEPARTMENT / DEPARTMENT_TREE 合并所属与负责部门；其他数据范围保持原定义。功能权限不由任命自动授予。
- 更换/清空负责人返回旧负责人仍关联的员工数量并记录审计，不改员工上级、旧考核任务、分数或签字快照。

## 自动化验证

| 验证 | 结果 | 证据 / 范围 |
|---|---|---|
| Go 全套 | PASS | 隔离 builder 内 `go test ./... -count=1`；handlers、models、routes、SQLite→PG迁移CLI均通过 |
| PostgreSQL 16 回归 | PASS | 专用无宿主端口的临时PG容器；`TestCrossDepartmentLeader*` 与 `TestPostgresScoringTaskDutyIndexUpgrade` 全部通过 |
| SQLite 回归 | PASS | Go全套内包含跨部门兼任、权限拒绝、候选过滤、自评/循环、数据范围与新旧流程 |
| 索引升级 | PASS | SQLite和PG均从旧三列唯一索引升级，连续执行两次生产迁移；旧任务状态/版本/分数保留，不同职责允许、同职责重复拒绝 |
| 全流程HTTP handler集成 | PASS | 两部门创建普通员工与真实模板→创建考核→总结/自评→同一CTO按两职责分别评分→缺一必填评分时拒绝推进→总分90→员工确认completed |
| 组织交接回归 | PASS | 负责人替换后员工manager_id、完成状态、总分、任务评分人及签字快照不变；旧评分任务访问保留；11月新考核使用新负责人和原CTO分别生成任务 |
| 启动修复回归 | PASS | 兼任者上级不被旧部门负责人自动修复逻辑改写 |
| TypeScript | PASS | `npx tsc --noEmit` |
| 前端生产构建 | PASS | `npm run build`，22页构建成功，含类型和lint检查 |
| 前端表单回归 | PASS | `node tests/unit/department-hierarchy-form.test.mjs` |
| 浏览器交互 | PASS，4/4 | `tests/e2e/cross-department-leaders.spec.ts`；真实Chrome及本次Next构建，API全模拟，无业务数据写入 |

浏览器用例：第2页跨部门候选选择与创建提交；跨部门姓名显示与清空后的3人交接提示；只读账号不加载候选；候选接口失败显示错误并禁用保存。

## 可重复命令

宿主有Go时：`cd server && go mod tidy && go test ./... -count=1`。本次使用现有builder镜像内复制的源码，依赖整理与Dockerfile.backend相同，不改宿主依赖文件。

浏览器验证先 `npm run build`，再在独立终端运行 `npm run start -- --hostname 127.0.0.1 --port 18159`，随后：

```bash
KPI_BASE_URL=http://127.0.0.1:18159 npx playwright test tests/e2e/cross-department-leaders.spec.ts --reporter=line
```

PG回归只针对一次性数据库执行；同时设置 `KPI_TEST_DISPOSABLE_POSTGRES=1` 与 `KPI_TEST_POSTGRES_DSN`，运行：

```bash
go test ./handlers ./models -run 'TestCrossDepartmentLeader|TestPostgresScoringTaskDutyIndexUpgrade' -count=1
```

测试自动创建独立schema并清理。不得把此环境变量指向日常或生产数据库。本次PG没有连接现有Compose项目或数据卷。

## 发布与页面验收边界

- 用户按自己的流程构建并更新前后端镜像；此次没有重启日常或生产Compose；发布到仓库不等于已部署线上。
- 后端启动自动替换评分任务唯一索引，无需手工SQL；不新增表/列，不清理数据，不自动任命CTO。
- 历史考核保持原快照，不补造过去被去重的职责；双职责任务在新建考核时生效。生成双职责任务后，旧版本三列索引无法容纳它们，回退旧镜像需要配套的数据方案。
- 自动化HTTP集成测试未运行完整登录网关；浏览器测试使用模拟API。线上网关、真实账号权限配置及更新后的人工页面验收尚待用户执行，不能视为已验收。
- 页面步骤见 quickstart.md。

## Spec-Kit Converge

结论：Converged。已逐项核对7条功能需求、4条成功标准、3个用户故事、9项实现设计及6条宪章原则，无未满足的开发项，无CRITICAL/HIGH发现，不追加收敛任务。规格检查清单5/5通过。开发和自动化验证完成，人工生产验收独立保留。
