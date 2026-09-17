# Quickstart Validation: 用户角色与权限管理

1. 确认测试账号已绑定 `super_admin` 主角色；若现有环境尚无超级管理员，必须先由系统所有者明确授权一次性初始化，不得自动提升 HR 权限。

   ```bash
   export BOOTSTRAP_SUPER_ADMIN_EMAIL='由系统所有者确认的账号邮箱'
   docker compose up -d --build backend
   unset BOOTSTRAP_SUPER_ADMIN_EMAIL
   ```

   后端只会在系统不存在有效超级管理员时执行初始化，并写入 `bootstrap_super_admin` 审计记录。初始化成功后应移除该环境变量；重复启动不会改派其他账号。
2. 使用超级管理员打开 `http://localhost/roles`（“组织与绩效 / 用户角色”），确认系统角色显示只读标识。
3. 复制“部门负责人”为“测试部门复核员-<时间戳>”，移除创建考核权限，保留查看和审核权限，数据范围选择本部门。
4. 将该角色分配给专用测试用户并调用 `/api/me` 或重新登录刷新身份信息。
5. 验证测试用户能查看本部门考核并执行被授予操作，不能创建考核或查看其他部门。
6. 尝试授予操作者没有的权限或更宽数据范围，确认返回 403。
7. 尝试删除仍绑定用户的角色，确认被拒绝；改派用户后删除成功。
8. 尝试改派最后一个超级管理员，确认被拒绝。
9. 检查审计日志包含角色创建、修改、复制、分配和删除事件。
10. 清理时先将测试用户改回原角色，再删除带时间戳的测试角色；不得删除数据库卷。

## Quality Gates

```bash
cd server && go test ./...
npx tsc --noEmit
npm run build
docker compose config
docker compose build backend frontend
docker compose up -d
curl http://localhost/api/health
```
