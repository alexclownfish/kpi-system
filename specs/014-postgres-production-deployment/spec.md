# Feature Specification: PostgreSQL 生产迁移与冷部署

**Feature Branch**: `main`
**Created**: 2026-10-23
**Status**: In progress — local migration and restore rehearsal passed; real production data and cutover pending
**Input**: 在全新线上目录重新拉取部署，并把旧线上 SQLite 的员工、考核和签字附件完整迁移至 PostgreSQL。

## User Scenarios & Testing

### User Story 1 - 保留历史数据切换数据库 (Priority: P1)

运维人员从旧服务取得一致性数据库副本与附件，在隔离环境迁移后核对员工、角色、模板、考核、评分、审计和签字记录。校验失败时，旧服务仍可恢复使用。

**Why this priority**: 用户明确要求保留旧业务数据，任何遗漏均不可接受。

**Independent Test**: 以旧库副本演练迁移，核对全部业务表行数、关键关联、主键序列和附件文件。

**Acceptance Scenarios**:

1. **Given** 旧服务已有真实数据，**When** 生成一致性副本并迁移，**Then** 新库中的所有业务记录、ID 和关系均可核对。
2. **Given** 迁移校验失败，**When** 运维人员停止切换，**Then** 旧库与签字附件未被删除或修改。
3. **Given** 迁移完成，**When** 使用原账号登录并打开历史考核及签字附件，**Then** 内容和状态与旧服务一致。

---

### User Story 2 - 从空目录重新部署 (Priority: P1)

运维人员在新服务器空目录拉取代码、配置私密参数、启动数据库和应用，并通过原有域名使用系统。重建应用后数据仍在。

**Why this priority**: 服务器目录无源码，部署必须从仓库获取可复现版本。

**Independent Test**: 在隔离目录克隆发布提交、部署、检查健康状态，重启并再次登录。

**Acceptance Scenarios**:

1. **Given** 目标目录为空，**When** 执行首次部署步骤，**Then** 完整代码被拉取且服务健康。
2. **Given** 数据库暂时不可用或凭据错误，**When** 后端启动，**Then** 明确失败，不静默连接本地 SQLite。
3. **Given** 应用已运行，**When** 重建容器，**Then** 历史数据与签字附件仍然存在。

---

### User Story 3 - 新库业务与备份验收 (Priority: P2)

管理员在迁移后的系统里检查岗位模板，创建考核并走完评分、确认、导出。运维人员执行数据库和附件备份演练。

**Why this priority**: 健康检查只能证明进程存活，不能证明业务正确。

**Independent Test**: 用测试账号走完整考核并生成完整备份，核查记录和导出。

**Acceptance Scenarios**:

1. **Given** 迁移已通过核对，**When** 打开模板并创建考核，**Then** 岗位匹配、指标排序和评分流程正常。
2. **Given** 考核完成，**When** 员工确认、上传签字并导出，**Then** 页面、文件和数据库状态一致。
3. **Given** 新库有业务数据，**When** 执行备份演练，**Then** 全部业务表与附件可恢复。

### Edge Cases

- 空目录并不代表旧 Docker 命名卷不存在；发现同名卷时停止并核对。
- 迁移包含不在旧页面 SQL 备份名单中的新表、历史快照与附件。
- 旧数据库可能正有写入，副本必须一致，切换时必须冻结写入。
- 数据库密码、JWT 密钥和临时密码不得入库或打印到日志。

## Requirements

### Functional Requirements

- **FR-001**: 系统 MUST 显式支持 PostgreSQL 连接，失败不得回退到 SQLite。
- **FR-002**: 系统 MUST 保留 SQLite 模式供现有环境和测试使用。
- **FR-003**: 迁移 MUST 完整保留业务表、主键、关系、权限、审计、考核快照与附件。
- **FR-004**: 迁移 MUST 提供校验报告，至少比对表行数、关联完整性、序列和附件数量。
- **FR-005**: 首次部署 MUST 从空目录获取源码并建立不公开数据库端口的持久化服务。
- **FR-006**: 部署 MUST 安全初始化密钥、等待数据库就绪、明确健康检查及错误。
- **FR-007**: 历史账号登录、岗位模板、考核流程、确认和导出 MUST 在新库正常工作。
- **FR-008**: PostgreSQL MUST 有全量数据库备份和经过演练的维护窗口恢复步骤。
- **FR-009**: 切换失败时 MUST 能回到保留的旧服务、旧数据库和附件。

### Key Entities

- **业务数据库**: 员工、部门、岗位、权限、模板、考核、评分、确认和审计等全部记录。
- **附件档案**: 与确认记录相对应的签字材料。
- **部署凭据**: 数据库密码、JWT 密钥及首次管理员密码，仅保存在运行环境。

## Success Criteria

### Measurable Outcomes

- **SC-001**: 迁移后全部业务表行数与源库一致，关键关联检查 0 错误。
- **SC-002**: 抽样的历史员工、模板、已完成考核和签字附件 100% 可访问。
- **SC-003**: 一次新考核从创建到签字导出完整通过，无数据库兼容错误。
- **SC-004**: 重新构建应用后，抽样记录和附件 100% 保留。
- **SC-005**: 数据库与附件备份均能在隔离环境恢复并通过数量核对。

## Assumptions

- 目标路径 `/mnt/datadisk0/project/kpiSystem` 当前无部署文件，但旧服务数据仍需保留。
- 旧生产数据库为 SQLite；实际 Docker 卷和附件路径在迁移前只读确认。
- 域名拟用 `https://kpi.auracor.net`，管理员邮箱为 `yangwenzhe@howxing.com`，现有账号密码哈希应迁移而非重置。
- 服务器已有 Docker Compose，且有该仓库的 SSH 读取权限。
