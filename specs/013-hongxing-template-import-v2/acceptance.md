# Phase 2 源表解析验收记录

日期：2026-10-14<br>
源文件：`/Users/simon/Downloads/荭杏研发项目绩效考核表 (1).xlsx`<br>
SHA-256：`50bcf21db2fb357a5a471ecef459353b974355c2b7191e3f920fa708c3735fb3`

本阶段只实现离线、只读的源表解析和 catalog 构建。没有启动 Docker，没有写入 SQLite、模板、员工、评分或生产数据。

## 验证命令

使用工作区提供的 bundled Python（openpyxl 3.1.5）：

```bash
PYTHONPATH=$HOME/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/lib/python3.12/site-packages \
$HOME/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3.12 \
scripts/test_source_workbook.py \
'/Users/simon/Downloads/荭杏研发项目绩效考核表 (1).xlsx'
```

输出：

```text
PASS Hongxing parser: 21 sheets, 18 templates, 137 indicators, 9 departments
PASS shared formulas expanded; quarterly I17/J17 gap and annual scope diagnostics detected
```

重复运行 `verify_source_workbook.py` 和 `build_hongxing_catalog.py` 后，JSON 字节内容完全一致。目录生成命令如下，输出文件由调用方指定：

```bash
PYTHONPATH=$HOME/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/lib/python3.12/site-packages \
$HOME/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3.12 \
scripts/build_hongxing_catalog.py \
'/Users/simon/Downloads/荭杏研发项目绩效考核表 (1).xlsx' \
--output /private/tmp/hongxing-workbook-analysis/catalog-v2-new.json
```

## 结果

| 检查项 | 结果 |
| --- | --- |
| 工作表 | 21 张，流程 1、岗位模板 18、HR 汇总 2 |
| 源部门 | 9 个；部门名称和来源单元格保留在 `source_metadata` |
| 指标 | 137 个；岗位/层级/周期、指标行、评分职责和评分标准均可追溯 |
| 计算 | 18 套模板逐项舍入后的综合权重均为 100；使用 Decimal HALF_UP |
| shared formula | 通过 XML master/child、`si`、`ref` 展开，并保留原始属性 |
| 季度明细断档 | `HR汇总（季度）!I17`、`J17` 缺公式，被诊断为真实数据行断档 |
| 季度年度公式 | `K40:K49` 共 10 条源公式引用 `D:J`，诊断为业务应核对的 D:G 范围异常 |
| 维度内权重异常 | 10 项，保留原值和来源行，不静默归一化 |

维度异常明细：结构负责人“交付及时性”70、“交付质量”130；电子负责人“交付质量”120、“协同与变更”70；软件应用负责人“交付质量”73、“协同与变更”140；产品负责人和产品工程师均“交付及时性”210、“协同与变更”90。以上仅作为 warning 保留来源值，不单独阻止发布，不修改源文件。

`05-算法开发-工程师` 的部门单元格在当前 SHA 文件中实际有值，因此本次结果记录为 `fallback=false`。如果后续版本为空，解析器会按岗位组映射到“算法开发部”，同时保留 raw 值为空和映射原因。

## 当前范围和后续门槛

本记录只覆盖 Spec-Kit T001、T002、T005、T006–T013。Go 数据模型、服务端导入/发布接口、模板页面、评分流程和 Docker 页面验收仍未在本阶段执行；后续任务必须先消费本阶段 catalog 和诊断，再进行导入事务与业务回归。

## T024–T026 前端导入预览验收（2026-10-14）

已在 `lib/api.ts` 增加来源工作簿校验、来源诊断、HR 汇总诊断和草稿/发布导入客户端，并统一复用既有错误消息与请求标识提取。

`app/templates` 新增“导入新版工作簿”页签：选择 `.xlsx` 后按实际上传内容执行只读校验，展示 SHA、解析器版本、21 张工作表/18 套模板/137 个指标/9 个部门等计数；可按需加载完整来源诊断和 HR 汇总公式诊断。综合权重不为 100 的阻断诊断会禁用“导入并发布”；维度内权重异常仅 warning，发布前需明确确认告警并保留原值。校验、诊断和导入均有加载状态、成功提示、错误请求标识和重试入口；导入失败会清空残缺预览，成功后显示实际新增/复用数量并关闭旧操作。

静态验证：

```bash
npx tsc --noEmit
```

结果：通过。真实 API 和页面点击验收需由用户启动 Docker 按 quickstart 验证。

## 服务端来源版本 MVP（T014–T023，2026-10-14）

已完成 T014–T019 的最小后端闭环：新增 `SourceWorkbook`、`SourceSheet`、`DepartmentMapping`、`ImportDiagnostic`、`FormulaCell` 和 `HRAggregationRule` 兼容模型，并在启动迁移中以新增表方式接入，不删除旧模板、考核、用户或附件。来源 SHA 唯一约束阻止重复版本；模板保存 v2 来源 SHA、诊断 JSON 和 `source_blocked`。来源版本按实际上传 XLSX 保存不可变解析快照；维度内权重异常仅作为 warning 保留原值，综合权重异常、未知评分职责或真实 HR 数据公式断档仍由发布校验阻断。草稿保持 draft/inactive，HR 在模板列表显式发布新版本。

新增受保护接口：`POST /api/source-workbooks/validate`、`GET /api/source-workbooks/:id/diagnostics`、`GET /api/source-workbooks/:id/hr-diagnostics`、`POST /api/source-workbooks/:id/import`。验证接口按实际上传内容解析并保存非敏感元数据、来源工作表分类、岗位预览和 catalog 诊断；响应统一包含 `request_id`，诊断按严重级别和顺序返回，支持 `workbook-<id>` 不透明 ID。导入接口完成来源 SHA、重复版本、显式部门映射、公式诊断落库、draft/publish 阻断和幂等模板目录复用；真实页面验收仍需用户启动 Docker。

## US3 岗位匹配、职责解析与快照验收（T034–T042，2026-10-14）

已完成服务端核心闭环：

- `findPublishedTemplate` 现在要求部门、岗位和周期命中唯一的已发布模板；空岗位、无匹配、非法周期和部门层级跨级继承均明确拒绝；同键多模板返回 `TEMPLATE_MATCH_AMBIGUOUS`，不会按版本或姓名排序静默选择。
- 评分人解析只读取 `EvaluationResponsibility` 业务职责，并按部门/岗位/公司作用域、有效期和在职状态过滤；普通员工即使没有对应 RBAC 角色也可以作为 PM、CTO、产品经理或部门负责人评分人。必填指标缺少职责或职责员工不可用时返回结构化 `SCORER_RESOLUTION_INCOMPLETE`，创建事务回滚。
- 新版快照记录模板版本、源 catalog/SHA、部门、岗位、指标与逐指标评分职责、默认会签顺序、导出版式、计算版本和 SHA-256 checksum。创建后修改模板、部门、岗位、职责或 RBAC 不会改写快照。

新增定向测试：`evaluation_template_v2_test.go`、`evaluation_responsibility_v2_test.go`、`evaluation_snapshot_v2_test.go`。由于宿主机没有 Go/gofmt，尚未执行 Go 编译；已通过 `git diff --check`，需要在 Docker builder 或安装 Go 的环境运行定向测试与 `gofmt`。前端评分人列表和创建页面（T037/T041）仍需完成浏览器回归。
## US4 评分、计算和必填门禁闭环（T043–T051，2026-10-14）

补充了新版评分闭环的服务端与页面契约测试：评分任务只允许快照分配的评分人写入，过期版本和重复提交均保持原任务不变；有效 0 分、空白、不适用及不适用理由分别验证；18 套 catalog 岗位样例逐一生成计算快照，软件应用开发负责人内部维度权重异常在综合权重仍为 100% 时可正常计算、但仍由来源诊断阻止发布；必填门禁失败不改变考核状态，完整评分进入 `awaiting_employee_confirmation` 并记录 `confirmation_gate_rejected`、`evaluation_calculated` 审计；页面契约覆盖进度、未完成清单、0 分、N/A 理由、贡献值、计算版本和错误反馈。

可执行验证：

```bash
node tests/unit/scoring-gate-v2.test.mjs
find tests/unit -name '*.test.mjs' -print0 | while IFS= read -r -d '' f; do node "$f"; done
```

结果：新增定向 Node 测试和现有全部 Node 单元测试通过。宿主机未安装 Go，`gofmt`/`go test ./...` 仍需在 Docker builder 或安装 Go 的环境执行；未启动 Docker。

本次闭环补充并统一了服务端实现：

- 评分写入始终从考核快照和 item-user 任务校验归属、版本、状态、评分区间及有限数值；0 分有效，必填指标不能 N/A，选填指标标记 N/A 时必须填写理由。必填完成度只统计 `submitted`，迁移遗留的必填 N/A 仍会阻断流程。
- 统一计算使用 Decimal HALF_UP、多个有效评分算术平均、逐项贡献值、总分和 S/A/B/C/D 等级；综合权重合计异常阻断计算，维度内权重异常保留源值并仅作为来源告警，不接受客户端总分。
- 流程按自评→评分→员工确认顺序推进；缺少必填自评/评分返回 `409 REQUIRED_SCORES_INCOMPLETE`、未完成项和责任人明细，状态保持不变；完整评分进入 `awaiting_employee_confirmation` 并落库计算结果。
- 评分页面展示评分职责、完成进度、阻塞指标、0 分、N/A 说明、贡献值和计算版本；失败信息包含请求 ID，并提供“重新读取”入口。
- `score_submitted`、`confirmation_gate_rejected` 和 `evaluation_calculated` 均写入审计；计算成功审计在事务提交后写入，审计详情只保留来源 ID、职责和数量，不写评分正文、完整评论、密码或上传内容。

新增/补充定向测试：`scoring_v2_test.go`、`calculation_v2_test.go`、`evaluation_workflow_v2_test.go`、`scoring_audit_v2_test.go`。宿主机没有 Go 工具链，Go 格式化和编译仍需在用户执行 Docker 构建后运行；本轮未启动 Docker。

## T057 统计汇总来源、诊断与重试（2026-10-14）

`lib/api.ts` 的 `statisticsApi.getPerformanceSummary` 已接入 `/statistics/performance-summary`，并补充 `diagnostic_level` 类型。统计页面从当前筛选条件下最近一条已完成考核读取汇总，展示总分/等级、来源快照 SHA-256、投入占比阈值与加权贡献、诊断级别、诊断信息和计算版本；汇总失败时保留错误请求标识并提供重试入口。最近考核数据补充 `employee_id`，调用前仍由服务端按员工数据范围授权。

新增 Node 契约测试：

```bash
node tests/unit/statistics-performance-summary.test.mjs
npx tsc --noEmit
git diff --check
```

结果：以上三项通过；未启动 Docker，真实 API 和浏览器页面验收仍需用户按 quickstart 执行。

## US1/US2 来源导入、模板维护和发布回归（T020–T033，2026-10-14）

补充了 HR 权限边界、来源元数据过滤、诊断/导入失败原子性和页面 loading/重试合同测试。模板发布侧验证了综合权重校验、必填指标评分职责门禁、同部门/岗位/周期重复发布键阻断、已发布模板不可直接改写以及复制新版本后再维护的历史隔离。来源诊断响应不返回服务器路径、密码或原始敏感元数据。

新增定向测试：`source_workbook_test.go`、`template_import_v2_test.go`、`tests/unit/template-publish-v2.test.mjs`；已通过 TypeScript、Node 单元测试和空白检查。真实 XLSX 上传、HR 页面点击和 Go 编译仍按 quickstart 在用户启动服务后执行。

## US5 HR 汇总与历史导出回归（T052–T058，2026-10-14）

汇总实现按月度/季度/年度筛选已完成考核，使用权威计算结果和计算版本；投入占比阈值按 `>= 0.20` 计入、低于阈值贡献为 0，缺少周期或占比会返回可诊断结果。测试覆盖 0.20 边界、舍入、非法数值、周期缺失、诊断等级和 `aggregation_diagnosed`/`aggregation_calculated` 审计。

单条结果、Excel 和批量签字 PDF 复用同一 `EvaluationResultSnapshot` 与导出版式：模板来源哈希、评分等级、加权贡献、评分职责、实际评分人和签字字段保持一致；PDF 固定 A4 竖向，历史已完成考核不随模板后续改动变化。`hr-diagnostics-v2.test.mjs` 验证诊断展示、阻断发布和重试反馈。

## 自动化回归（T059、T061，2026-10-14）

使用工作区 bundled Python 直接执行 `scripts/test_source_workbook.py`，结果稳定通过：21 张工作表、18 套岗位模板、137 个指标、9 个部门，以及 shared formula 和季度断档诊断均符合清单。当前环境未安装 pytest，因此没有使用 `python -m pytest` 包装器；解析器回归本身已执行并通过。

前端质量门禁全部通过：

```text
node --test tests/unit/*.test.mjs    23 passed
npx tsc --noEmit                      passed
npm run build                         passed
npm run lint                          passed
docker compose config                 passed
git diff --check                      passed
```

新增 `result_export_v2_test.go` 对单条结果和批量签字 PDF 的快照校验码、纵向版式、加权贡献、评分等级和字段一致性建立了专用回归合同。Go/gofmt 和真实页面验收仍需在用户可访问 Docker 的环境执行；本轮没有启动容器或修改数据卷。

## Spec-Kit Analyze（T064，2026-10-14）

已按 `spec.md`、`plan.md`、`tasks.md` 和项目 Constitution 完成跨文档检查。FR-001–FR-020、SC-001–SC-008 均有对应任务；未发现 CRITICAL/HIGH 规格冲突、宪章 MUST 违反或未覆盖的核心用户故事。发现的实现路径差异（统计汇总使用独立 `statistics_v2.go`、导出回归补充专用 v2 测试文件）已与现有实现和任务记录对齐。Go 编译、Docker 健康检查和真实页面验收属于环境/人工门槛，继续由 T060、T062、T063 跟踪。
## 源表空白评分回归修复（2026-10-14）

复核发现解析器曾对工作簿中空白的 H 列评分单元格生成 synthetic 分数，可能把未评分指标误报为已有总分。现已修复为保留 `score=null`、`score_origin=blank`，只有源文件真实提供的数值才参与离线样例计算；存在空白时 `expected_total` 和 `expected_grade` 保持未定，不写入 catalog，也不影响模板评分流程。

回归验证：

```text
PASS Hongxing parser: 21 sheets, 18 templates, 137 indicators, 9 departments
PASS shared formulas expanded; quarterly I17/J17 gap and annual scope diagnostics detected
```

并断言所有指标的 `score_origin` 仅为 `source` 或 `blank`，不存在 synthetic 分数；未评分、0 分和不适用继续保持可区分。

全流程验收脚本已同步调整：测试分数只在 `verify_workbook_runtime.py` 的隔离夹具中生成，用于提交临时考核并计算预期结果；`source.json` 和 catalog 继续保留 Excel 原始空白，不把测试分数写回业务目录。

## Docker 构建失败修复与后端实际回归（2026-10-16）

用户构建日志有两处真正失败：`TestCanonicalCalculationCoversEveryHongxingPositionProfile/04-软件应用开发-部门负责人` 的旧断言，以及 `TestTemplateImportV2PublishRejectsDuplicateMatchKey` 中草稿意外被启用。`buildx` 警告与负面测试的 GORM 日志不构成本次阻断原因；构建失败后 `/health` 的 OK 来自仍在运行的旧服务，不能作为新版本部署成功的证据。

修复与回归范围：

- 软件应用负责人内部维度权重 73%/140% 保留源值；综合权重经逐项 HALF_UP 后合计 100% 的快照允许计算，异常来源模板仍被发布门禁阻止。
- `KPITemplate.IsActive` 的 GORM 默认值改为 `false`，避免插入时把显式 `false` 替换为旧数据库默认值 `true`；不修改员工启用默认值。
- 创建与编辑草稿不能通过旧 `is_active` 参数绕过发布；复制的新版本及含来源异常的初始化模板保持 draft/inactive，只有发布操作启用模板。
- 启动迁移事务只修复 `status IN ('draft','retired') AND is_active=true` 的记录，每个模板写入 `template_activation_repaired` 审计；保留已发布模板、未知历史状态及考核快照。重复启动不重复修复，审计写入失败则回滚。
- 新增模型与 API 回归覆盖创建、编辑、复制、重复匹配键发布失败、种子导入、旧 default:true 表结构升级、修复幂等、审计失败回滚和历史数据保留。

本轮使用临时安装的官方 Go 1.23.12 工具链和临时模块缓存实际执行，未启动 Docker，也未写入现有业务数据库：

```text
gofmt（本 Feature 修改/新增的 Go 文件）       passed
go test ./...                              passed (handlers/models/routes)
node --test tests/unit/*.test.mjs            23 passed
docker compose config --quiet               passed
git diff --check                            passed
```

T060 的后端格式化与测试已完成。T062 的新容器启动、T063 的真实页面验收及 T065 的最终收敛仍未完成，不能据此把整个 Feature 标为已验收。

用户重新构建后页面验证：进入模板管理，新建草稿并复制一个已发布模板，二者均应保持草稿且未启用；已发布原模板保持不变。选择存在来源异常的草稿确认发布仍被阻止，正常模板通过全部校验后才能发布。无需重新生成管理员密码、清库或执行 `git pull` 更新当前正在开发的工作区。

## Phase 10 Convergence（2026-10-16）

本轮按 Spec-Kit Converge 发现并完成 T066–T071：

- 原始工作簿实际复核为 **0 个阻断、22 个告警**；10 项维度内合计异常保留来源值，仅作 warning。18 套模板逐项舍入综合权重均为 100%，不再因 73%、120%、130%、140%、210% 等维度内合计误阻断。
- HR 汇总季度 I17/J17 仅 A17=13 且 B:H 空白，诊断为 warning；B:H 任一业务输入（包括 0）或无序号业务输入时，缺公式升级为 blocking。年度 K40:K49 范围问题仍保留 warning。
- Go 在线解析器直接读取上传 XLSX 的 ZIP/XML 与单元格，按实际 SHA 保存不可变 catalog、shared formula 审计、贡献公式校验和部门来源映射；修改文件得到新版本，同 SHA 重验只刷新诊断并保留导入状态/历史模板。
- 草稿导入固定为 draft/inactive；发布需显式确认 warning，同 SHA 已存在草稿/停用模板时返回冲突并要求在模板列表显式发布，不假报全部模板已发布；显式部门映射实际作用于新模板。
- 页面契约新增 warning 确认、真实新增/复用数量、成功后关闭过期操作、失败清空与 request id 回归；`tests/unit/source-workbook-import-flow.test.mjs` 与既有 23 项 Node 单元测试通过。

本轮自动化证据：

```text
python3 scripts/test_source_workbook.py <原始工作簿>       PASS (21/18/137/9)
go test ./...                                              PASS (server)
go test ./handlers -run 'TestSourceWorkbook|TestParseSourceWorkbook|TestHongxingCatalog|TestTemplateImportV2|TestTemplateActivation' PASS
node --test tests/unit/*.test.mjs                         PASS (24)
npx tsc --noEmit                                          PASS
npx eslint app/templates/page.tsx lib/api.ts              PASS
npm run build                                              PASS
docker compose config --quiet                              PASS
git diff --check                                           PASS
```

T062/T063/T065 仍是环境与人工门槛：用户需要重新构建容器、访问页面完成一次真实导入/模板发布/岗位匹配/评分/导出验收，并把结果反馈后才能将 Feature 标记为最终验收完成。该步骤不需要清库、删除卷或 `git pull` 当前工作区。

## Phase 11 Template lifecycle convergence（2026-10-21）

本轮实现并回归：

- 新来源工作簿发布会创建新版本；同一部门/岗位/周期的旧 active 版本在同一事务中变为 `retired` 且 `is_active=false`。历史模板和历史考核快照保留，不物理删除。
- 草稿手动发布也使用同样的版本切换事务，不再因已有当前版本返回重复匹配冲突。
- 模板列表默认隐藏 retired 版本，提供“显示历史版本”开关；当前发布版本禁止直接删除，未引用历史版本可在显式查看历史后清理。
- 启动迁移只回填缺少直属上级的已知子部门负责人，优先使用上级部门负责人或 CTO；HR 已有直属上级不会被覆盖，每次实际修复写入审计。
- 新版考核的总结页显示“统一加权得分满分 100 分”，不再把“各指标满分合计 700”当作最终满分，也不显示不适用于新版流程的旧自评/主管/HR 零值卡片。
- 迁移和验收文档继续保留旧口径说明，本次已修正维度内权重“仅告警”的表述。

自动化证据：

```text
go test ./... (isolated Go 1.23 container)                  PASS
node --test tests/unit/*.test.mjs                           PASS (25)
npx tsc --noEmit                                            PASS
npx eslint app/templates/page.tsx app/evaluations/page.tsx  PASS
npm run build                                               PASS
docker compose config --quiet                               PASS
KPI_BASE_URL=http://localhost:3100 npx playwright test \
  tests/e2e/template-history.spec.ts \
  tests/e2e/template-export-layout.spec.ts                 PASS (5 scenarios)
git diff --check                                            PASS
```

## Phase 12 Excel workflow completion（2026-10-21）

本轮补齐 Excel 流程表对应的执行能力：

- 新流程考核增加工作总结、问题风险、所需支持、后续计划和提交时间；未提交时自评 API 和流程推进均被后端阻断。
- 评分任务从流程表得到 PM=1、负责人/CTO=2、产品经理=3 的顺位；越序提交返回 `SCORING_SEQUENCE_BLOCKED`，相同顺位可并行。
- 流程表八步流程、适用范围、责任角色、要求和时限随来源版本保存；模板会签规则优先标记为来源流程规则，缺少流程配置时才使用系统默认。
- 考核增加项目名称和投入占比；统计分析对同一员工同一周期聚合多条已完成考核，按项目展示原始分、投入占比和加权贡献，年度按有效项目投入计算。
- 当前发布模板删除被后端拒绝；模板指标读取要求 `kpi:view`；孤立评分任务支持按审计返回的样本 ID 显式清理并写审计。

本轮验证：

```text
go test ./... (Go 1.23 isolated container)                  PASS
go vet ./...                                               PASS
node --test tests/unit/*.test.mjs                          PASS (25)
npx tsc --noEmit                                           PASS
npx eslint ...                                             PASS
npm run build                                              PASS
docker compose config --quiet                              PASS
Playwright template lifecycle + export suite               PASS (5)
Playwright work-summary gate page                          PASS (1)
```

## Final product-manager mapping and convergence（2026-10-21）

页面验收发现产品经理郭庆无法匹配模板。根因是源工作簿产品部门只有“部门负责人”和“工程师”两套被考核岗位模板，而员工岗位名称是“产品经理”。已增加受控映射：

- 仅在部门名称为“产品部门”时，`产品经理/产品负责人` 映射到 `部门负责人` 模板。
- 其他部门的“产品经理”不会继承该别名，仍按原有评分职责逻辑处理。
- 回归覆盖产品部门正向匹配和其他部门拒绝泄漏。
- 产品经理提交工作总结时曾因角色只有评分权限、没有 `assessment:submit` 被中间件拒绝。工作总结和在线确认均为本人操作，现改为在处理器内强制校验 `evaluation.EmployeeID == 当前用户`；同时给 PM、产品经理和 CTO 系统角色补充 `assessment:submit` 默认权限。

最终质量门禁：Go 全量测试、`go vet`、25 项 Node 单元测试、TypeScript、ESLint、Next.js production build、Compose 配置和测试产物敏感信息扫描均通过。T062/T063 已由用户页面验收确认完成；T065 收敛检查覆盖 24 FR、8 SC、5 个用户故事，未发现新的高风险可构建缺口。产品经理映射需在下一次重建后在“创建考核”页面做一次最终页面复验。
