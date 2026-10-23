# Acceptance: 可维护部门层级与负责人

日期：2026-10-01。范围010，仅复核本次新增组织负责人及上级关系；不是整个平台所有功能的再次验收。

## 根因与修复
- 原上级合法性要求祖先部门DEPARTMENT_MANAGER评分职责，RBAC的部门负责人角色不等于实际组织负责人，导致截图候选为空。
- 新增Department.leader_id由管理员明确维护，独立于权限角色和岗位名称；上级候选、员工保存、模板部门负责人/直属上级评分复用组织负责人。
- 部门列表独立展示上级部门及负责人；父候选排除自身及后代，保留后端环校验。失效负责人可见，解析不静默兜底。
- 未指定组织负责人时兼容原职责；其它模板评分类型不改。CTO评分项不会因为选择父负责人就变为父负责人评分。

## 自动化结果
| 检查 | 结果 |
| --- | --- |
| 先写失败回归 | 三个后端失败：负责人未保存、父负责人被拒绝、候选为空；前端父候选缺少后代排除失败 |
| Go全套 `go test ./...` | PASS，handlers/models/routes |
| `department_leader_test.go` 五项组合回归 | PASS：维护、跨部门/停用/范围拒绝、省略/null、明确父负责人无重复职责、自定义身份、优先级、失效/调岗/上级循环、候选、预览/创建/任务/快照、历史、写权限允许/拒绝、初始化保留 |
| `department-hierarchy-form.test.mjs` | PASS，父候选自身/后代/既有环，以及负责人字段/独立列 |
| 原错误解析及团队范围unit | PASS |
| `tsc --noEmit` | PASS，最终production build亦检查类型 |
| 变更页面和lib/api.ts ESLint | PASS |
| 最终Next production build | PASS，既有templates/page.tsx fetchTemplates依赖警告保留 |
| `git diff --check` | PASS |

## 实际页面与重启验收
复用普通库的隔离副本，测试服务仅18096/18097，不替换普通库、不切换用户账号。

1. 仅在副本准备“隔离验收父部门负责人”（属于机器人事业研发部，无评分职责记录）。
2. 浏览器部门管理设置父部门负责人并保存，成功提示及列表可见。
3. 浏览器将软件应用开发部上级清空保存，再重新指定机器人事业研发部，同时指定王成斌为部门负责人，均保存成功。
4. 浏览器编辑王成斌，上级候选包含刚指定父负责人和既有CTO；不包含自己。选择父负责人并保存，员工列表显示新上级，姓名、邮箱、岗位和在职状态保留。
5. 重启临时后端后只读HTTP核对：父负责人ID17、软件应用开发部parent_id16/leader_id14、王成斌manager_id17全部保持；父负责人评分职责记录为0，候选仍包含父负责人且无本人。
6. 最终页面390×844窄屏截图复核，长上级名称被表单约束，按钮完整可操作；浏览器尺寸已恢复。

截图目录 `/private/tmp/kpi-hierarchy-runtime.aCSIPx/`：
`parent-leader-saved.jpg`、`editable-hierarchy.jpg`、`parent-leader-candidate.jpg`、`parent-supervisor-saved.jpg`、`parent-supervisor-mobile.jpg`。
这些是隔离验证证明，不表示普通环境已配置真实负责人。副本中的测试账号不进入初始化脚本或Git。

## 部署边界
- 未执行普通Compose重建、清库、git pull、提交或推送；未改变账号密码、普通环境角色/职责和流程开关。
- 重建时AutoMigrate新增nullable indexed leader_id；不会自动猜测/填充负责人。009层级初始化marker仍保持，HR后续修改不会重复覆盖。
- 历史快照/已派任务保持；仅新考核使用当前组织配置。
- 普通服务访问地址不变。临时前后端及编译服务结束后停止；保留测试证据，不留第二套运行环境。

## Spec Kit
Specify→Clarify（无阻塞歧义）→Plan→Tasks→Analyze（7FR覆盖、无冲突）→Implement→Converge。
Converge核对7FR、3SC、7个用户验收场景、6计划决策、6宪章原则；限定范围无未实现项。
