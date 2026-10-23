# Quickstart Validation: Evaluation Workflow Unification

This guide is the acceptance path for the completed feature. It does not authorize deleting the SQLite volume or clearing existing assessment data.

## 1. Prerequisites

- A local `.env` exists with a valid `JWT_SECRET`, `CORS_ALLOWED_ORIGINS=http://localhost:8088`, `KPI_HTTP_BIND=127.0.0.1`, and `KPI_HTTP_PORT=8088`.
- A valid administrator account exists.
- Keep the existing Compose volumes. Do not run `docker compose down -v`.

## 2. Automated quality gates

Backend:

```bash
cd server
go test ./...
```

Frontend:

```bash
npx tsc --noEmit
npm run build
```

Compose configuration:

```bash
docker compose config
```

## 3. Build and start

The user may build the project with:

```bash
docker compose up -d --build
docker compose ps
curl -fsS http://localhost:8088/health
```

Expected result: all three services are running and `/health` returns HTTP 200.

## 4. Read-only audit acceptance

1. Log in as an administrator.
2. Open the assessment audit page or call the audit preview API with a limited period.
3. Record database row counts for assessments, legacy scores, invitations, and scoring tasks.
4. Run the same audit twice.
5. Confirm:
   - both reports contain the same finding codes and evaluation IDs;
   - valid records are marked eligible;
   - missing snapshots or ambiguous legacy scores are marked manual review;
   - business row counts and timestamps remain unchanged.

## 5. Template-driven task acceptance

Create a unique test template such as `E2E-研发考核-20260930`:

| Item | Required | Scorer responsibility |
|---|---:|---|
| 指标 A | yes | PM |
| 指标 B | yes | DEPARTMENT_MANAGER |
| 指标 C | yes | PM + PRODUCT_MANAGER |

Create or select active users with the corresponding business responsibilities, then create an assessment for an applicable employee.

Confirm:

- PM receives only A and C;
- department manager receives only B;
- product manager receives only C;
- no duplicate task exists for the same assessment, item, and user;
- changing a user's RBAC role after creation does not rewrite these assignments.

## 6. Authorization acceptance

1. Log in as a user assigned to item A and submit item A.
2. Attempt to submit item B through the UI or API.
3. Confirm item A succeeds and item B returns HTTP 403 with `SCORING_TASK_FORBIDDEN`.
4. Log in as an unassigned reviewer with general review permission and repeat the item B request.
5. Confirm general permission alone does not grant task write access.

## 7. Required-task and zero-score acceptance

1. Submit all but one required task.
2. Attempt to advance to employee confirmation.
3. Confirm the operation is blocked and identifies the missing task.
4. Submit the remaining task with a numeric score of `0`.
5. Confirm the task is complete and the assessment can advance when every other requirement is satisfied.

## 8. Weighted calculation acceptance

Use known values:

- Dimension 1 weight: 40%; item A weight inside dimension: 50%; item A score: 80.
- Dimension 1 weight: 40%; item B weight inside dimension: 50%; item B score: 100.
- Dimension 2 weight: 60%; item C weight inside dimension: 100%; two scorer values: 70 and 90.

Expected result:

```text
item A contribution = 80 × 20% = 16
item B contribution = 100 × 20% = 20
item C average = 80
item C contribution = 80 × 60% = 48
total = 84
```

Confirm detail view, canonical result API, Excel, PDF, batch sign-off PDF, and statistics all show total `84` and the same calculation version.

## 9. Snapshot and export acceptance

Template-save/clone regression evidence and exact page steps:
[template-save-acceptance-20261008.md](./template-save-acceptance-20261008.md).
Published/retired layouts are read-only; use “复制新版本后编辑”, save the draft
layout and publish from Calculation structure. Existing assessments retain their
creation-time layout. A rejected save must display text without a white page.

1. Complete an assessment and download its Excel and PDF.
2. Clone the source template, change an item label, weight, selected export columns, and print orientation, then publish the new version.
3. Reopen and export the historical assessment.
4. Confirm its original labels, weights, selected columns, portrait orientation, result, and checksum remain unchanged.

## 10. Migration dry-run acceptance

1. Select one clean in-progress legacy assessment and one ambiguous legacy assessment.
2. Run migration dry-run.
3. Confirm the clean assessment is eligible and the ambiguous assessment has a manual-review code.
4. Confirm no scores, statuses, invitations, tasks, totals, or snapshots changed.
5. Apply only the reviewed clean batch as an authorized administrator.
6. Confirm additive task/result records and an audit log are created while legacy records remain available.

## 11. Export failure diagnostics acceptance

1. From an authorized evaluation detail page, export one final result as PDF and Excel.
2. Confirm the success response includes a request ID, result version, and checksum; the page only reports success after the download URL is opened or the fallback tab is opened.
3. Trigger an export with an invalid evaluation ID or a filter that has no finalized records.
4. Confirm the page shows the stable error code, request ID, and actionable retry/filter guidance instead of a silent failure.
5. Trigger a render/storage failure in an isolated test environment and confirm the response marks it retryable and includes the result version when one exists.
6. For batch sign-off export, confirm the response includes one result/checksum entry per PDF in the archive.

## 12. End-to-end regression

```bash
npm run test:e2e -- tests/e2e/evaluation-workflow.spec.ts
```

Expected result: audit, allowed/denied score writes, zero-score handling, state transitions, snapshot immutability, calculation parity, export parity, and migration dry-run scenarios all pass.

## 13. Completed-assessment sign-off supplement acceptance

This path verifies the post-completion paper-signature requirement. It must be
run with a completed fixture that already has an online confirmation and a
separate in-scope operator with `assessment:approve`.

1. Open the completed assessment and record its status, total, calculation/result
   version, snapshot checksum, confirmation ID/method/time, and any original
   confirmation attachment metadata.
2. Upload a valid PDF, JPG, JPEG, or PNG no larger than 10 MB using “补充纸质签字”.
   The UI must state that the material is archived and the completion status is
   unchanged.
3. Re-read the assessment and confirmation. Confirm status remains `completed`,
   the total/result version/checksum and confirmation row are unchanged, and the
   original confirmation attachment is still available.
4. List supplements and download the new file. Confirm the list includes its
   ID, uploader, upload time, optional signed date, original basename, size and
   remark; download bytes and detected content type match the uploaded file.
5. Upload a second valid file. Confirm a second supplement ID is created and the
   first row/file remains available.
6. Inspect administrator audit logs. Confirm exactly one
   `upload_signoff_supplement` success entry per successful upload includes the
   evaluation/supplement IDs and non-sensitive file metadata, while excluding
   the free-text remark and raw contents.
7. Repeat the list/download as an out-of-scope user and the upload as a user
   lacking approval permission. Confirm both are denied without changing rows,
   files, status, or audit success entries.
8. Attempt uploads while the assessment is `awaiting_employee_confirmation`,
   `rejected`, `cancelled`, or has no confirmation. Confirm a conflict response
   and no database/file residue. Repeat with an empty, unsupported, malformed,
   and over-10-MB file, an invalid date, and a remark over 500 characters.
9. Delete a disposable assessment that has supplements through the normal API.
   Confirm all supplement rows and physical files are gone. Force an isolated
   persistence/deletion failure and confirm staged files are restored and no
   partial rows remain.

API smoke examples (replace IDs and bearer token with fixture values):

```bash
curl -fsS -X POST "$KPI_API/evaluations/42/confirmation/supplements" \
  -H "Authorization: Bearer $TOKEN" \
  -F "signed_at=2026-10-12" \
  -F "remark=纸质签字回收" \
  -F "file=@signed-copy.png"

curl -fsS "$KPI_API/evaluations/42/confirmation/supplements" \
  -H "Authorization: Bearer $TOKEN"

curl -fLo signed-copy.png \
  "$KPI_API/evaluations/42/confirmation/supplements/12/attachment" \
  -H "Authorization: Bearer $TOKEN"
```

For the first two JSON calls, record `request_id`. Force a state, scope, or file
validation failure and verify the response uses `error.code`,
`error.message`, `error.details` and the same `request_id`; the successful upload
continues to expose the legacy `data` and `evaluation.status` fields.

The automated baseline currently covers successful upload, immutable original
confirmation, unchanged completed status, audit redaction, list/download, and
rejection before completion in `server/handlers/confirmation_test.go`.
Additional malformed-file, scope, concurrency, and deletion-failure coverage is
implemented by T121–T122; the real-service browser execution remains tracked by
T123 in `tasks.md`.

## 14. Safe cleanup

Delete only the uniquely named test assessments/templates through normal application APIs when allowed. Do not remove Docker volumes or the database file.
## 本轮验证记录

权重方案 A 最新验收见 [weight-policy-acceptance-20261001.md](./weight-policy-acceptance-20261001.md)：
18/18 原表岗位流程、综合权重校验、计算与导出复核通过。临时测试页面为
`http://localhost:18092/templates`；普通 Compose 服务保持不变。
隔离环境的原表模板、组织、评分职责及合成账号已准备，无需逐项猜测配置。

页面复测：选择结构部门负责人维护草稿 → 计算结构 → 内部权重 120%/80%
可以保存；首项 70 改成 60 后综合权重 98.5%，发布禁用且保存提示错误，
恢复 70。电子/软件负责人同样保留原值，不归一化。考核页三个已完成
合成样例总分分别为 24.82、51.07、33.71。

本地更新当前开发代码只需 `docker compose up -d --build`，不需要 `git pull`
或清空数据库；线上拉取远端代码属于另一部署流程。

最新增量验收记录见 [business-repair-acceptance-20261001.md](./business-repair-acceptance-20261001.md)。
确认、纸质签字和异议状态守卫的最新增量记录见
[workflow-guard-acceptance-20261008.md](./workflow-guard-acceptance-20261008.md)。
本轮增量验证结果如下：

- `git diff --check`: 通过。
- `npx tsc --noEmit`: 通过（使用工作区 Node 运行时）。
- `npm run build`: 通过；仅保留既有 `app/templates/page.tsx` Hook 依赖警告。
- `go test ./...`: 当前工作机未安装 Go，且 Docker socket 不可用，未执行；请在 Go/CI 或用户启动的构建环境执行。
- Playwright: 已补充 `tests/e2e/evaluation-workflow.spec.ts`；真实服务场景需服务启动并提供验收账号后执行。

## 生产验收顺序

1. 保持 `EVALUATION_WORKFLOW_MODE=audit`，执行 `docker compose config`，确认环境变量已展开。
2. 由运维执行 `docker compose up -d --build`，确认 backend healthcheck 通过。
3. 使用管理员打开审计页，执行审计预览和迁移 dry-run；确认人工复核项为空或已单独处理。
4. 复核评分职责、模板发布版本、历史考核快照和导出布局。
5. 将模式切换为 `enabled`，再次构建 backend，并创建一条测试考核验证评分任务和必填阻塞。
6. 运行 `npx playwright test tests/e2e/evaluation-workflow.spec.ts`。测试只使用精确 ID 清理，不删除 Compose 卷。
