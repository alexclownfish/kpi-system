# Quickstart: 统一绩效结果导出验收

## Automated validation

```bash
docker compose --env-file .env.example build backend frontend nginx-gateway
docker compose --env-file .env.example config
```

后端构建须通过 `go test ./...`；前端构建须通过类型检查和 production build。

## Scenario 1: configure a template

1. 进入“KPI 模板管理”，选择测试模板和“结果导出设置”。
2. 选择“评分过程完整版”，取消非必需列、调整顺序、修改标题并增加“复核人签字”。
3. 保存并刷新。

Expected: 配置完整保留并显示成功；取消必需列时收到明确校验提示。

## Scenario 2: compare formats and entry points

1. 对同一已定稿考核从详情导出 Excel 与 PDF。
2. 用相同筛选执行批量导出签字 PDF。
3. 核对标题、列及顺序、数值、总结和签字栏。

Expected: 三者业务字段和顺序一致；批量 ZIP 仍含签字回收汇总表。

## Scenario 3: frozen history

1. 导出一份考核并记录布局。
2. 修改其模板布局。
3. 再次导出原考核，并定稿一份新考核后导出。

Expected: 尚未确认的原考核生成新版本并使用新布局；完成确认后再修改模板，原考核保持确认时布局；新考核使用新布局。

## Scenario 4: compatibility and denial

1. 对升级前模板和快照导出。
2. 用无 `report:export` 账号请求导出。
3. 用有权限但超范围账号请求单份导出。

Expected: 历史数据按默认简版导出；无权限和超范围请求均被后端拒绝。

## Safety

- 不执行 `docker compose down -v`。
- 测试数据使用唯一后缀，验收后只删除明确测试记录。

## 2026-09-18 execution record

- Backend Docker build and `go test ./...`: passed.
- Frontend TypeScript validation and Next.js production build: passed.
- Compose configuration and `/health`: passed; no data volume was removed.
- Authenticated browser scenarios remain for the project owner to execute after signing in again. The local validation restart used a new random JWT secret because the workspace has no `.env`, so pre-existing browser tokens were intentionally invalidated.
