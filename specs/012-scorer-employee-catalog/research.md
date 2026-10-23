# Research

- Decision: 既有 GET /employees 每页 100，按 totalPages 读取所有页，保留过滤条件。
- Rationale: 原 pageSize=500 超上限回退 10，未加载后续页；departmentApi.getCatalog 是本地既有模式。
- Alternatives: 仅改 100 会漏 101 人；扩大固定上限仍截断；新增无范围接口违反隔离。

- Decision: 固定 employees.id 升序；任意页失败整体失败；页面清空旧数据并以 loading/loadError 禁用误操作。
- Rationale: 稳定名单跨页不漏不重，失败不伪装成空名单或完整名单。
- Alternatives: 吞掉后续页错误复现静默缺失；角色筛选错误地将业务职责等同于系统角色。

- Decision: 保留 ApplyEmployeeScope、is_active=true 和保存时人员校验，不修改职责/权限/工作流模式。
- Rationale: 后端已支持 CTO，系统角色不限制候选；自动分配真实职责超出本次范围。
