# Initialization and Position Contract

`kpi-server init-business`：自动迁移、系统默认配置、业务目录、显式管理员初始化后退出0，失败非0。普通启动共用逻辑。不创建演示账号、评分、职责或考核，不输出密码。

SEED_HONGXING_CATALOG 默认 true；false 跳过目录；其它非空值拒绝。SEED_DEMO_DATA=true 仅警告。管理员沿用 BOOTSTRAP_ADMIN_* 文件配置，已有有效管理员不重置密码。

## GET /api/departments/:id/positions

要求认证及 department:view / employee:create / employee:edit 任一权限。200: `{ "data": [{ "id": 1, "department_id": 1, "name": "工程师" }] }`，按id排序。空目录 data:[]。
400 id无效；404部门不存在；401未认证；403无权限；500查询失败，均含error。绝不返回员工、密码或哈希。

UI：部门变化清空旧建议，忽略过期异步响应；保留已有自由文本。读取失败显示明确提示。新岗位建议与自定义输入共存。
