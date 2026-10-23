# Validation

日常入口 http://localhost。用户自行 `docker compose up -d --build` 更新，无需 git pull、不删除卷。

1. 本地 admin / yangwenzhe@howxing.com 登录。临时密码只保存在忽略的 secrets 文件，首次使用后修改。
2. 部门页查原表九部门；旧部门可保留。
3. 模板页每部门两套月度已发布模板，结构负责人内部120/80保留、综合100。
4. 员工创建页选原表部门，岗位建议含部门负责人和工程师，仍可输入自定义岗位。
5. 重启无重复。维护人工测试请复制模板；原模板改名、停用、删除不重建由自动测试验证。
6. 实际评分人和部门负责人需真实人员后配置，不生成虚构用户或职责。

门禁：Go全套、TS、lint、production build、Compose config和健康检查、源JSON重新生成比较。
