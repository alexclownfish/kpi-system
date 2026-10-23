# API Contract: Evaluation Workflow Unification

## Common response format

Successful response:

```json
{
  "data": {},
  "message": "操作成功",
  "request_id": "req_..."
}
```

Error response:

```json
{
  "error": {
    "code": "SCORING_TASK_FORBIDDEN",
    "message": "您不是该指标的评分人",
    "details": {}
  },
  "request_id": "req_..."
}
```

All protected endpoints require JWT authentication. Permission middleware is necessary but not sufficient for task writes; handlers must also verify task ownership and assessment state.

## Template weight validation (policy A)

`PUT /api/templates/:id/calculation-structure` and `POST /api/templates/:id/publish`
retain existing authorization, immutable-version rules and response envelopes.
Save/publish require finite 0..100 dimension/item weights, at least one item per
dimension and sum of individually HALF_UP-rounded composite weights equal to 100.
Internal item sums (including 73/80/120/140) and dimension totals do not independently
block the operation. No weights are normalized. Invalid input returns HTTP 400
with an actionable message and no partial structure writes. Canonical calculation
uses the same rule and rejects a stored composite value inconsistent with its
snapshot inputs, without changing the snapshot or persisted historical result.

## Template export-layout version contract

- `PUT /api/templates/:id` with `export_layout` updates a draft and returns its
  normalized layout together with dimensions/items, so saving the layout does
  not clear the calculation editor's structure. Published and retired templates
  return HTTP 409 `PUBLISHED_TEMPLATE_IMMUTABLE`; the client renders
  `error.message` as text, never the error object itself.
- `POST /api/templates/:id/clone` creates a separate draft with template version
  incremented by one and copies dimensions, items and the current export layout.
  The source remains unchanged. The settings page selects the returned draft,
  adds it to the list and permits editing/saving before publication.
- `export_layout.version` identifies the layout schema (currently 1), not the
  template's publication version. Old weighted-v2 assessments retain their
  creation-time layout; saving a clone cannot refresh their result snapshots.
- Published/retired export settings are read-only, with an explicit clone action.
  Draft save failures keep entered values and show a readable error; closing the
  dialog permits retry without reporting a failed save as successful.

## Read-only audit

### `POST /api/assessment-audits/preview`

Runs a read-only audit. Despite using POST for a structured filter body, it must not persist or modify business data.

Required permission: `system:edit` or a dedicated `assessment:audit` permission.

Request:

```json
{
  "evaluation_ids": [1, 2],
  "department_id": 3,
  "period": "2026-Q3",
  "statuses": ["pending", "self_evaluated"],
  "include_completed": false
}
```

Response data:

```json
{
  "run_id": "uuid",
  "summary": {
    "evaluations_scanned": 2,
    "eligible": 1,
    "manual_review": 1,
    "errors": 0
  },
  "findings": [
    {
      "evaluation_id": 2,
      "category": "snapshot",
      "severity": "manual_review",
      "code": "SNAPSHOT_MISSING",
      "message": "考核缺少模板快照，不能自动迁移",
      "eligible_for_migration": false,
      "details": {}
    }
  ]
}
```

Errors: `400 INVALID_AUDIT_FILTER`, `403 FORBIDDEN`, `500 AUDIT_FAILED`.

## Scoring tasks

### `GET /api/evaluations?assigned_to_me=true`

Returns only assessments in `scoring` or `awaiting_review` with a caller-owned
task in `pending`, `in_progress`, or `rejected`. The list, pagination count,
and summary cards apply the same assignment and data-scope filter. Historical
submitted tasks do not appear in this pending-work list. Unfiltered authorized
assessment reads continue to include assigned historical assessments.

`GET /api/evaluations/pending/:employeeId` requires the caller's own employee ID.
Pending-task counts include explicit assignments independently of RBAC role
names. Final-score averages include completed assessments with valid zero scores
and exclude unfinished assessments.

### `PUT /api/scores/:id/self`

Only the assessment employee can write self scores. Weighted-v2 assessments must
be in `self_evaluating`; legacy assessments must be in `pending`. Validate against
the immutable snapshot's maximum, with zero accepted. Weighted-v2 does not require
a legacy manager assignment. Errors: `403 SELF_SCORE_FORBIDDEN`,
`409 SELF_SCORE_LOCKED`, and `400 SCORE_OUT_OF_RANGE` (see handler codes).

### `GET /api/evaluations/:id/scoring-tasks`

Returns tasks visible to the caller, task completion summary, and assessment workflow state. Data-scope and task assignment are applied before serialization.

Required permission: one of `assessment:view`, `review:view`, plus scope access.

Response data:

```json
{
  "evaluation_id": 1,
  "workflow_state": "scoring",
  "completion": { "required": 3, "submitted": 2, "blocking": 1 },
  "tasks": [
    {
      "id": 10,
      "item_id": 100,
      "item_name": "缺陷密度",
      "scorer_user_id": 23,
      "scorer_type": "PM",
      "scorer_label": "项目经理",
      "required": true,
      "status": "pending",
      "score": null,
      "max_score": 100,
      "comment": "",
      "version": 1,
      "can_edit": true
    }
  ]
}
```

Errors: `403 ASSESSMENT_SCOPE_FORBIDDEN`, `404 EVALUATION_NOT_FOUND`.

### `PUT /api/scoring-tasks/:id`

Saves an in-progress score without marking the task submitted.

Required checks: authenticated user has general scoring permission, owns the task or has explicit override permission, assessment is editable, version matches.

Request:

```json
{
  "score": 0,
  "comment": "已按数据核验",
  "not_applicable": false,
  "version": 1
}
```

Response data: updated task including incremented `version`.

Errors: `400 SCORE_OUT_OF_RANGE`, `409 TASK_VERSION_CONFLICT`, `422 COMMENT_REQUIRED`, `403 SCORING_TASK_FORBIDDEN`, `409 TASK_LOCKED`.

### `POST /api/scoring-tasks/:id/submit`

Validates and submits a task. A numeric score of `0` is valid. `not_applicable=true` requires template permission and no numeric score.

Request:

```json
{
  "score": 0,
  "comment": "未达到目标",
  "not_applicable": false,
  "version": 2
}
```

Response data:

```json
{
  "task": { "id": 10, "status": "submitted", "score": 0, "version": 3 },
  "workflow": { "state": "scoring", "blocking_tasks": 1 }
}
```

## Workflow and result

### `POST /api/evaluations/:id/workflow/advance`

Requests the backend to evaluate requirements and advance to the next valid state. The request does not contain a target state or total score.

Request:

```json
{ "expected_state": "scoring" }
```

Response data:

```json
{
  "previous_state": "scoring",
  "current_state": "awaiting_employee_confirmation",
  "blocking_tasks": [],
  "result_version": 2
}
```

Errors: `409 WORKFLOW_STATE_CONFLICT`, `422 REQUIRED_TASKS_INCOMPLETE`, `422 RESULT_CALCULATION_FAILED`, `403 WORKFLOW_FORBIDDEN`.

### `GET /api/evaluations/:id/result`

Returns the canonical result used by detail, confirmation, export, and statistics.

Response data includes:

```json
{
  "evaluation_id": 1,
  "template_version": 3,
  "calculation_version": "weighted-v2",
  "result_version": 2,
  "dimensions": [],
  "items": [],
  "total_score": 86.5,
  "grade": "A",
  "export_layout": { "orientation": "portrait", "columns": [] }
}
```

Errors: `404 RESULT_NOT_AVAILABLE`, `409 SNAPSHOT_INVALID`, `403 ASSESSMENT_SCOPE_FORBIDDEN`.

## Evaluation responsibilities

### `GET /api/evaluation-responsibilities`

Lists assignments visible within the caller's administration scope.

Required permission: `role:view` or dedicated `responsibility:view`.

### `POST /api/evaluation-responsibilities`

Creates an assignment independent from RBAC role assignment.

Request:

```json
{
  "code": "PM",
  "scope_type": "department",
  "department_id": 3,
  "user_id": 23,
  "active": true
}
```

Errors: `400 INVALID_RESPONSIBILITY_SCOPE`, `409 RESPONSIBILITY_DUPLICATE`, `422 USER_INACTIVE`, `403 FORBIDDEN`.

### `PUT /api/evaluation-responsibilities/:id`

Updates active/effective scope for future assessment resolution. It does not rewrite existing task snapshots.

### `DELETE /api/evaluation-responsibilities/:id`

Performs an audited deactivation rather than deleting historical evidence.

## Completed sign-off supplements

These endpoints handle a scanned paper-signature copy that arrives after the
assessment has already been finalized. They never replace the single effective
`EvaluationConfirmation` row or reopen the workflow.

### `POST /api/evaluations/:id/confirmation/supplements`

Uploads one append-only supplement. Required permission: `assessment:approve`
plus access to the assessment's data scope. The assessment MUST be
`completed` and MUST have an existing final confirmation (online or paper).

Request: `multipart/form-data`

| Field | Required | Validation |
|---|---:|---|
| `file` | yes | non-empty PDF/JPG/JPEG/PNG, at most 10 MB; server detects the content type |
| `signed_at` | no | `YYYY-MM-DD` |
| `remark` | no | at most 500 characters |

Successful response (`200`):

```json
{
  "message": "补充签字材料上传成功",
  "data": {
    "id": 12,
    "evaluation_id": 42,
    "uploaded_by": 7,
    "uploaded_at": "2026-10-13T12:00:00Z",
    "signed_at": "2026-10-12T00:00:00Z",
    "attachment_name": "signed-copy.png",
    "attachment_content_type": "image/png",
    "attachment_size": 1024,
    "remark": "纸质签字回收"
  },
  "evaluation": { "id": 42, "status": "completed" },
  "request_id": "req_..."
}
```

The existing confirmation handlers keep the legacy `message`/`data` shape and
the `evaluation` sibling for compatibility. The supplement response now also
carries the common request identifier; clients must continue to read `data` and
the returned `evaluation.status`.

The response status MUST remain `completed`. The canonical result, total,
result snapshot version/checksum, and original confirmation are unchanged. The
stored file key is generated by the server; only a safe basename is exposed as
the display filename. Each successful upload records an
`upload_signoff_supplement` audit operation with evaluation/supplement IDs and
non-sensitive file metadata. The audit details MUST exclude the remark and raw
file contents.

Errors use the common envelope and always include `request_id`:

- `400 INVALID_EVALUATION_ID`, `SIGNOFF_FILE_INVALID`,
  `SIGNOFF_FILE_READ_FAILED`, `SIGNOFF_FILE_TYPE_UNSUPPORTED`,
  `SIGNOFF_SIGNED_AT_INVALID`, or `SIGNOFF_REMARK_TOO_LONG` for invalid input;
- `403 ASSESSMENT_SCOPE_FORBIDDEN` when the assessment is outside the caller's
  data scope;
- `409 SIGNOFF_SUPPLEMENT_STATE_CONFLICT` when the assessment is not
  `completed`, or `SIGNOFF_CONFIRMATION_REQUIRED` when it has no final
  confirmation;
- `500 SIGNOFF_CONFIRMATION_READ_FAILED`, `SIGNOFF_STORAGE_FAILED`, or
  `SIGNOFF_SUPPLEMENT_WRITE_FAILED` for server failures. A failed persistence
  operation removes the just-written file and does not create a success audit
  entry.

### `GET /api/evaluations/:id/confirmation/supplements`

Requires `assessment:view` plus assessment data-scope access. Returns the
assessment's supplements ordered newest first:

```json
{ "data": [ { "id": 12, "evaluation_id": 42, "attachment_name": "signed-copy.png" } ], "message": "补充签字材料读取成功", "request_id": "req_..." }
```

This endpoint is read-only and returns no supplements from another assessment.
Successful responses include `request_id`; an invalid ID returns
`400 INVALID_EVALUATION_ID`, an out-of-scope request returns
`403 ASSESSMENT_SCOPE_FORBIDDEN`, and a database read failure returns
`500 SIGNOFF_SUPPLEMENT_READ_FAILED`.

### `GET /api/evaluations/:id/confirmation/supplements/:supplement_id/attachment`

Requires `assessment:view` plus assessment data-scope access. The
`supplement_id` MUST belong to `:id`; otherwise return `404`. On success the
server streams the stored file with its detected content type and original
basename and echoes the generated/requested `X-Request-ID` response header. The
endpoint has no business-write or audit side effects.

Errors use `request_id` and stable codes: `400 INVALID_EVALUATION_ID` or
`INVALID_SIGNOFF_SUPPLEMENT_ID`, `403 ASSESSMENT_SCOPE_FORBIDDEN`, and `404
SIGNOFF_SUPPLEMENT_NOT_FOUND` or `SIGNOFF_ATTACHMENT_NOT_FOUND`.

### Supplement cleanup invariant

Normal assessment deletion removes all supplement rows and their stored files
in the same recoverable transaction path as confirmation attachments. A failed
delete restores staged files and leaves the assessment and its supplements
consistent.

## Operational audit diagnostics

### `GET /api/audit-logs`

Returns recent audited operations for administrator diagnostics. Required
permission: `audit:view` or `system:edit`. The endpoint is read-only and never
creates or updates audit records.

Supported query parameters:

- `action_prefix`: optional prefix filter, such as `assessment_migration_`,
  `evaluation_responsibility_`, or `workflow_`;
- `resource`: optional exact resource filter;
- `result`: optional exact result filter;
- `limit`: optional integer from 1 to 200, default 50;
- `before_id`: optional descending cursor.

Response data:

```json
{
  "logs": [
    {
      "id": 42,
      "action": "assessment_migration_dry_run",
      "resource": "migration_batch",
      "resource_id": "batch-1",
      "result": "SUCCESS",
      "details": "eligible=1 manual_review=0",
      "ip_address": "127.0.0.1",
      "created_at": "2026-10-08T10:00:00Z",
      "actor": { "id": 1, "name": "admin", "email": "admin@example.com" }
    }
  ],
  "next_before_id": 1
}
```

Errors: `400 INVALID_AUDIT_LOG_LIMIT`, `400 INVALID_AUDIT_LOG_CURSOR`,
`403 AUDIT_LOG_FORBIDDEN`, `403 AUDIT_LOG_SCOPE_FORBIDDEN`,
`500 AUDIT_LOG_READ_FAILED`.

## Migration

### `POST /api/assessment-migrations/dry-run`

Creates a deterministic preview from current data without modifying assessments, scores, invitations, tasks, statuses, or totals.

Required permission: `system:edit` plus assessment administration scope.

Request:

```json
{
  "evaluation_ids": [1, 2],
  "target_version": "weighted-v2"
}
```

Response data:

```json
{
  "batch_id": "uuid",
  "mode": "dry_run",
  "eligible_evaluation_ids": [1],
  "manual_review": [{ "evaluation_id": 2, "code": "AMBIGUOUS_LEGACY_SCORE" }],
  "planned_task_count": 5
}
```

### `POST /api/assessment-migrations/:batchId/apply`

Applies only the exact reviewed eligible set from an unexpired dry-run batch. The server revalidates source checksums before writing.

Request:

```json
{ "confirm_batch_id": "uuid" }
```

Errors: `409 MIGRATION_SOURCE_CHANGED`, `409 MIGRATION_ALREADY_APPLIED`, `422 MIGRATION_HAS_AMBIGUITIES`, `403 FORBIDDEN`.

### `GET /api/assessment-migrations/:batchId`

Returns status, summary, affected IDs, skipped IDs, and errors. This GET has no write side effects.

## Legacy endpoint transition

- `PUT /api/scores/:id/manager`, `/hr`, and `/final` remain available only for legacy assessments during the compatibility window.
- New workflow assessments reject writes through legacy score endpoints with `409 LEGACY_SCORE_ENDPOINT_DISABLED`.
- `GET /api/evaluations/:id/invitations` must stop creating invitations/tasks and becomes read-only.
- `PUT /api/evaluations/:id` must ignore/reject client-supplied total or arbitrary workflow status for new workflow assessments.

## Export diagnostics

Successful export responses include the request identifier and the immutable
result version used to build the file:

```json
{
  "file_url": "/api/download/exports/export-1",
  "file_name": "绩效结果-员工-2026年10月.pdf",
  "file_size": 12345,
  "message": "导出成功",
  "request_id": "req_...",
  "result_version": 3,
  "checksum": "..."
}
```

Batch sign-off exports use `results` entries containing `evaluation_id`,
`result_version`, and `checksum` for every PDF included in the archive. Export
failures use the common error envelope and stable codes such as
`EXPORT_RENDER_FAILED`, `EXPORT_RESULT_SNAPSHOT_FAILED`,
`NO_FINALIZED_EVALUATIONS`, and `EXPORT_ARCHIVE_FAILED`. `error.details` always
contains `retryable` and `retry_guidance`; when a result version exists it also
contains `result_version`. The UI must show the error code, request ID, result
version (when present), and the retry guidance so operators can provide an
actionable support report.
