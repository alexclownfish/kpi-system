# Implementation Plan: 荭杏研发绩效模板导入与流程适配 v2

**Branch**: `013-hongxing-template-import-v2` | **Date**: 2026-10-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/013-hongxing-template-import-v2/spec.md`

## Summary

修正现有工作簿解析器对新 Excel 结构的错误假设，建立以 Excel SHA 和来源单元格为核心的版本化导入、诊断和模板目录。导入器必须实际解析上传内容，正确识别流程表、18 套岗位模板及 2 张 HR 汇总表，展开 shared formula，保留实际岗位模板权重和评分职责。按已确认 A 方案，维度内合计异常仅告警，综合权重不为 100、职责缺失或真实 HR 数据行公式断档才阻止发布。新建考核按部门/岗位/周期自动匹配已发布模板，生成评分人与会签快照；评分完成后通过必填指标门禁、总分计算、HR 汇总和历史导出验证。

## Technical Context

**Language/Version**: Python 3.11+（只读工作簿解析脚本）、Go 1.23、TypeScript/React 19、Next.js 15

**Primary Dependencies**: 现有 openpyxl/XML 解析、Gin/GORM/SQLite、现有 API/模板/评分/导出组件；不新增任意公式执行引擎

**Storage**: 既有 SQLite；新增来源版本、公式诊断、模板来源字段和兼容快照字段，提供幂等迁移

**Testing**: Python 源表快照测试、Go 单元/集成/契约测试、Node 前端测试、TypeScript 检查、Next production build、页面手工验收；Docker 构建和服务启动由用户执行

**Target Platform**: 本地和生产 Docker Compose Web 应用；桌面浏览器页面与后端 API

**Project Type**: Web application with a controlled workbook import/diagnostic utility

**Performance Goals**: 给定 21 张表的工作簿在单次校验中完成；重复运行输出稳定；创建考核不产生逐员工无界请求

**Constraints**: Excel 是最终事实源；不静默归一化权重；不执行宏或任意公式；后端授权和数据范围先于序列化；不清空现有数据库卷；历史考核只读快照

**Scale/Scope**: 21 张表、18 套月度模板、137 个指标、9 个源部门；支持继续新增 HR 维护的版本和组织映射

## Constitution Check

### Before Phase 0

- I 后端强制授权：导入预览、提交、发布、考核创建和汇总接口均要求服务端权限；前端只提供反馈。
- II 数据范围隔离：模板、员工、评分任务和汇总按既有 scope 过滤；业务职责类型不替代 RBAC。
- III 明确契约：契约文件定义稳定成功/错误结构、来源定位和 request id；加载失败不能静默保存。
- IV 可测试增量：每个用户故事有独立测试，解析器缺陷和公式断档有回归夹具。
- V 数据/部署安全：迁移幂等、导入原子、旧快照保留；不删除 Compose 卷。
- VI 单一代码源：只修改根目录 `app/`、`components/`、`lib/`、`server/` 和 `scripts/`，不改 `kpi-main/` 历史副本。

结论：通过，无宪章例外。

### After Phase 1 design

数据模型保留 source hash、raw 值、诊断和快照；契约覆盖预览、导入、发布、评分门禁、汇总诊断和审计。shared formula 解析的复杂度由可复现性和审计要求支撑，不引入第二套业务评分引擎。结论：通过。

## Project Structure

### Documentation (this feature)

```text
specs/013-hongxing-template-import-v2/
├── spec.md
├── research.md
├── data-model.md
├── plan.md
├── quickstart.md
├── contracts/template-import-api.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (planned)

```text
scripts/
├── verify_source_workbook.py       # 识别表类型、展开 shared formula、生成诊断
├── build_hongxing_catalog.py       # 从已验证工作簿生成版本化无评分 catalog
└── test_source_workbook.py         # 解析/来源 SHA/公式回归

server/handlers/
├── source_workbook.go              # 校验、导入预览、诊断接口
├── source_workbook_parser.go       # Go 实际解析上传 XLSX 与不可变清单
├── source_workbook_test.go
├── hongxing_catalog.go             # 幂等目录/岗位/模板导入
├── evaluation_template.go          # 模板匹配与职责快照
├── scoring.go / calculation.go     # 评分门禁与计算复核
├── statistics.go                   # HR 汇总和诊断
└── data/hongxing-catalog-v2.json   # 带 source SHA 的无评分业务目录

server/models/
├── models.go                       # 来源、诊断、快照和兼容字段
└── database.go                     # 幂等迁移和初始化约束

server/routes/routes.go             # 受保护路由
lib/api.ts                           # 导入/诊断/模板/汇总客户端
app/templates/page.tsx              # HR 模板来源、异常、版本和发布反馈
app/evaluations/page.tsx             # 自动匹配、职责、评分进度和门禁反馈
app/statistics/page.tsx              # 汇总来源和诊断状态
```

**Structure Decision**: 复用当前前后端目录和既有 Feature 007/008 的模型与接口，解析器保持独立、只读、可离线运行；不新增微服务、不修改历史副本。

## Implementation Phases

### Phase 0 — Source research and parser fixtures

1. 固化源 SHA、工作表清单、18 套模板计数、137 指标计数和 9 个源部门快照。
2. 增加流程表/岗位模板/HR 汇总表的内容识别，不依赖工作表下标。
3. 增加 shared formula master/child 展开、公式结构比较和缺失公式诊断。
4. 增加 HALF_UP 权重、加权得分、等级和维度异常诊断测试。

### Phase 1 — Source catalog and diagnostic model

1. 生成带 SHA、sheet、来源行、raw 值和诊断的 `hongxing-catalog-v2.json`；不写评分或演示员工。
2. 扩展来源版本、公式诊断、部门映射和 HR 规则模型，迁移必须幂等。
3. 实现只读校验、预览、诊断和显式导入接口；阻断错误整体回滚。

### Phase 2 — HR template maintenance and publication

1. 以源模板事实初始化草稿，允许 HR 修正维度内权重、评分职责、不适用策略和会签。
2. 发布时执行综合权重 100、必填评分职责、唯一匹配和真实 HR 数据行公式门禁；维度内合计与空白编号行缺公式仅告警。
3. 模板编辑生成新版本，已发布版本和历史考核不可破坏性覆盖。

### Phase 3 — Evaluation matching and scoring gate

1. 按部门、岗位类型和周期预览/匹配唯一模板。
2. 解析 PM/部门负责人/CTO/产品经理真实员工，创建 item-user 评分任务和会签快照。
3. 接入必填指标门禁、0 分与空白区分、多评分平均、综合权重和等级结果。

### Phase 4 — HR aggregation and historical exports

1. 解析月度/季度/年度 HR 汇总规则，阻止真实数据行的公式断档进入发布。
2. 计算投入占比阈值和周期汇总，保留计算版本、等级和来源哈希。
3. 统一结果、Excel、纵向 PDF 和批量签字导出读取 EvaluationSnapshot，验证模板更新不影响历史。

### Phase 5 — Validation and convergence

1. 运行解析器、Go、前端类型/构建和 API 契约回归。
2. 在私有克隆数据上完成页面验证：导入预览、异常阻止发布、岗位匹配、PM/CTO/部门负责人/PD评分、必填门禁、HR诊断和历史导出。
3. 运行 Spec-Kit Analyze/Converge；未完成项追加 tasks.md，用户启动 Docker 后按 quickstart 执行真实服务验收。

### 2026-10-16 import convergence

- Python 离线解析与 Go 在线解析采用相同权重及真实数据行政策；Go 直接读取 XLSX ZIP/XML，禁用宏和任意公式执行，不要求 Docker 镜像安装 Python。
- 每个上传 SHA 保存不可变解析清单；修订文件不复用固定 catalog 数值，同 SHA 重新校验仅刷新诊断，导入始终读取该来源清单。
- 草稿/发布按显式 mode 在单一事务中执行，返回新增与复用数量；已发布模板、HR 修改和历史考核不因政策修正被覆盖。
- 来源维度告警不因保存未改结构消失；旧策略误阻断仅作确定性修复，真实结构问题仍阻断。

## Complexity Tracking

无宪章违例。shared formula XML 展开、来源版本和诊断模型是必要复杂度，因为仅靠当前单元格值会丢失公式语义并导致 HR 汇总错误；没有引入任意公式执行、第二套评分模型或清库迁移。
