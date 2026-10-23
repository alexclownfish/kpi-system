# Implementation Plan: 荭杏业务初始化

**Date**: 2026-10-01 | **Spec**: [spec.md](spec.md)

## Summary

原表只读提取为无评分及人员的版本化 JSON，嵌入后端。复用模板权重校验和事务，默认初始化九部门、十八部门岗位及十八已发布 weighted-v2 模板。唯一初始化记录保护 HR 修改和删除。移除演示数据创建函数。

## Technical Context

- Go1.23 / Gin / GORM / SQLite；Next15 / React19 / TypeScript，无新依赖。
- SEED_HONGXING_CATALOG 默认 true，false 跳过；旧 SEED_DEMO_DATA 不再生成任何账号，警告弃用。
- init-business 命令只初始化默认配置、目录及显式管理员后退出，无监听端口。
- 测试：逐项来源比较、原子回滚、重复启动、修改删除保护、权限边界；Go全套、TS、lint、build、Compose配置及本地登录。
- 范围：九部门十八岗位十八模板；不实现工时汇总或完整岗位管理。

## Constitution Check

I/II：目录查询授权且不包含人员。III：错误可见。IV：先回归测试。V：原子事务、唯一记录、保留历史及已有密码；操作前备份，不删除卷、不覆盖 .env。VI：仅正式根目录代码，复用校验。设计前后均通过。

## Project Structure

- server/handlers/hongxing_catalog.go / hongxing_catalog_test.go：初始化、岗位读取、回归。
- server/handlers/data/hongxing-catalog-v1.json：来源 SHA 和纯业务结构。
- scripts/build_hongxing_catalog.py：可重复只读原表提取。
- server/models/models.go / database.go：JobPosition、BusinessInitialization迁移；删除演示函数。
- server/main.go / server/routes/routes.go：共享初始化及岗位接口。
- lib/api.ts / app/employees/page.tsx：部门岗位建议，兼容自定义输入。
- docker-compose.yml / .env.example / docs/production-deployment.md：开关和安全更新说明。

## Decisions

1. 核对原表部门标题、负责人和工程师标记及 SHA，复用既有只读提取；不保存 H 列评分或 synthetic 字段。
2. 整个目录一次事务成功后写 BusinessInitialization 唯一记录；有记录即返回，HR删除也不重建。
3. 同名部门唯一复用，歧义拒绝；已有相同部门/岗位/月度模板不论状态均不覆盖。
4. JobPosition department_id/name 唯一；岗位不是权限角色，员工 Position 仍支持自由文本。
5. 模板指标全部必填，沿用当前评分类型；保留产品团队负责人原文及 CTO 回退说明，不创建虚构评分人员。
6. 本地管理员恢复使用用户已授权的文件凭据，先备份现有库；真实登录验证，不写代码密码。
7. 隔离验收显式关闭新目录初始化，不与日常 localhost 账号混用。

## Complexity Tracking

无宪章例外。独立内部初始化记录避免标记暴露给公共设置接口。
