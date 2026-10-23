# Data Model: Evaluation Workflow Unification

## Existing entities retained for compatibility

| Entity | Current use | Feature treatment |
|---|---|---|
| `Employee` | Login identity, department, position, manager | Reused as the user target and scorer identity; role fields are not used as responsibility identity. |
| `Role`, `Permission`, `UserRole`, `DataScope` | RBAC and data access | Reused for page/API authorization; no scorer type is inferred solely from a role name. |
| `KPITemplate`, `KPIDimension`, `KPIItem` | Template, dimensions, items, weights, required flags, scorer type rules | Remain the authoring source for draft/published templates; published versions are immutable. |
| `KPIEvaluation` | Assessment header, legacy status/total, template snapshot | Additive fields or related records may be added; old score fields remain readable and are marked legacy when necessary. |
| `KPIScore` | Fixed self/manager/HR/final fields | Read-compatible legacy representation; new workflow writes explicit task records as the authority. |
| `EvaluationInvitation`, `InvitedScore` | Legacy invitation scoring | Preserved for historical reads and migration comparison; no new GET-side backfill. |
| `EvaluationResultSnapshot`, `EvaluationConfirmation` | Finalized output and sign-off evidence | Reused, but snapshot payload is generated from the canonical result. |

## New entities

### EvaluationResponsibility

Represents a business scoring responsibility independent of application RBAC.

| Field | Type | Required | Constraints |
|---|---|---:|---|
| `id` | integer | yes | primary key |
| `code` | string | yes | canonical values: `PM`, `DEPARTMENT_MANAGER`, `PRODUCT_MANAGER`, `CTO`; unique |
| `name` | string | yes | display label |
| `scope_type` | string | yes | `company`, `department`, `project`, `position` |
| `department_id` | integer | no | required when scope is department/position as applicable |
| `position_type` | string | no | used for position-scoped matching |
| `project_id` | integer | no | reserved for project-scoped matching when project data exists |
| `user_id` | integer | yes | references `Employee.id` |
| `active` | boolean | yes | inactive assignments cannot resolve new tasks |
| `effective_from` | timestamp | no | optional scheduling boundary |
| `effective_to` | timestamp | no | optional scheduling boundary |
| `created_by` | integer | yes | audit actor |
| `created_at`, `updated_at` | timestamp | yes | standard timestamps |

Unique matching rule: one active assignment for the same `(code, scope_type, department_id, position_type, project_id, user_id)`.

### EvaluationScoringTask

The authoritative assignment and score state for one user, one item, and one assessment.

| Field | Type | Required | Constraints |
|---|---|---:|---|
| `id` | integer | yes | primary key |
| `evaluation_id` | integer | yes | references `KPIEvaluation.id` |
| `item_id` | integer | yes | references the assessment snapshot item identity |
| `scorer_user_id` | integer | yes | references `Employee.id` |
| `scorer_type` | string | yes | copied from the resolved responsibility rule |
| `required` | boolean | yes | copied from snapshot rule |
| `status` | string | yes | `pending`, `in_progress`, `submitted`, `rejected`, `not_applicable`, `locked` |
| `score` | decimal | no | nullable; `0` is valid; range is `0..max_score` |
| `comment` | text | no | required when template rule says so |
| `not_applicable` | boolean | yes | mutually exclusive with a submitted numeric score |
| `submitted_at` | timestamp | no | set on submit, not on draft save |
| `locked_at` | timestamp | no | set after workflow locks task |
| `version` | integer | yes | optimistic concurrency, starts at 1 |
| `source_rule` | string | no | snapshot reference explaining how user was resolved |
| `created_at`, `updated_at` | timestamp | yes | standard timestamps |

Unique constraint: `(evaluation_id, item_id, scorer_user_id)`.

Validation invariants:

- `score IS NULL` means not submitted unless `not_applicable=true`.
- `score = 0` is a submitted valid score when status is `submitted`.
- `not_applicable=true` requires an allowed template policy and cannot coexist with a numeric score.
- Only the assigned scorer, or a specifically authorized reviewer in the allowed review state, may write the task.

### EvaluationCalculationResult

Server-owned cache/version of the canonical result for one assessment.

| Field | Type | Required | Constraints |
|---|---|---:|---|
| `id` | integer | yes | primary key |
| `evaluation_id` | integer | yes | one current result per evaluation; historical versions may be retained |
| `version` | integer | yes | monotonic per evaluation |
| `calculation_version` | string | yes | e.g. `weighted-v2` |
| `snapshot_checksum` | string | yes | identifies source snapshot |
| `result_json` | text | yes | item results, scorer values, weights, total, grade |
| `total_score` | decimal | yes | server-calculated value |
| `grade` | string | no | configured grade mapping result |
| `calculated_at` | timestamp | yes | calculation time |
| `calculated_by` | integer | no | system or operator identity |

The result is regenerated after accepted score submissions and before confirmation; a client-provided total is never copied into this entity.

### AssessmentAuditFinding

Read-only audit output persisted only when an operator chooses to retain an audit run; the live audit endpoint can return transient findings.

| Field | Type | Required | Constraints |
|---|---|---:|---|
| `id` | string | yes | audit-run identifier plus sequence |
| `run_id` | string | yes | groups one audit run |
| `evaluation_id` | integer | no | null for template-level findings |
| `category` | string | yes | `snapshot`, `task`, `score`, `status`, `duplicate`, `calculation`, `export`, `scope` |
| `severity` | string | yes | `info`, `warning`, `error`, `manual_review` |
| `code` | string | yes | stable machine-readable finding code |
| `message` | string | yes | actionable human-readable explanation |
| `details_json` | text | no | supporting IDs/counts |
| `eligible_for_migration` | boolean | yes | deterministic migration eligibility |
| `created_at` | timestamp | yes | audit timestamp |

### MigrationBatch

Tracks dry-run and applied migration operations.

| Field | Type | Required | Constraints |
|---|---|---:|---|
| `id` | string | yes | unique batch ID |
| `mode` | string | yes | `dry_run` or `apply` |
| `status` | string | yes | `created`, `running`, `completed`, `failed`, `cancelled` |
| `source_version` | string | yes | legacy or source calculation version |
| `target_version` | string | yes | new workflow version |
| `filters_json` | text | yes | selected period/department/status filters |
| `summary_json` | text | yes | counts and manual-review totals |
| `operator_id` | integer | yes | authorized operator |
| `started_at`, `completed_at` | timestamp | no | lifecycle timestamps |
| `created_at`, `updated_at` | timestamp | yes | standard timestamps |

### EvaluationSignoffSupplement

Stores an additional paper-signature copy uploaded after an assessment has already
been finalized. This entity is intentionally separate from
`EvaluationConfirmation`: the original confirmation is the single effective
confirmation, while supplements are append-only evidence attached to it.

| Field | Type | Required | Constraints |
|---|---|---:|---|
| `id` | integer | yes | primary key |
| `evaluation_id` | integer | yes | indexed reference to `KPIEvaluation`; upload requires `status=completed` and an existing final confirmation |
| `uploaded_by` | integer | yes | reference to `Employee`; must have approval permission and assessment data-scope access |
| `uploaded_at` | timestamp | yes | server assigned upload time |
| `signed_at` | date | no | optional physical signature date |
| `attachment_stored_name` | string | yes | server generated safe key; never derived directly from user filename |
| `attachment_original_name` | string | no | sanitized basename shown to readers |
| `attachment_content_type` | string | no | detected MIME type; only PDF/JPG/JPEG/PNG accepted |
| `attachment_size` | integer | no | positive bytes, at most 10 MiB |
| `remark` | string | no | at most 500 characters; free text is not copied into audit details |
| `created_at`, `updated_at` | timestamp | yes | standard timestamps |

Supplement rows are append-only in the workflow API. Multiple rows may reference
one completed assessment, and a supplement upload never changes the assessment
state, canonical result, result snapshot, snapshot checksum/version, or original
confirmation. Listing and download are read-only and use the assessment's data
scope. Transactional assessment deletion removes both rows and physical files;
failed database insertion removes the just-written file.

## Assessment snapshot shape

`KPIEvaluation.TemplateSnapshotJSON` remains the immutable source of truth for historical reads and is extended to contain:

```json
{
  "template": { "id": 1, "name": "研发岗位考核", "version": 3, "period": "quarterly" },
  "calculation_version": "weighted-v2",
  "dimensions": [
    {
      "id": 10,
      "name": "交付质量",
      "weight": 40,
      "items": [
        {
          "id": 100,
          "name": "缺陷密度",
          "max_score": 100,
          "item_weight": 50,
          "composite_weight": 20,
          "required": true,
          "scorer_types": ["PM"],
          "aggregation": "average"
        }
      ]
    }
  ],
  "resolved_scorers": [
    { "item_id": 100, "scorer_type": "PM", "user_id": 23, "user_name": "项目经理" }
  ],
  "export_layout": { "version": 2, "columns": ["employee", "item_score", "total_score"], "orientation": "portrait" }
}
```

The snapshot is append-only after assessment creation. A repair operation may add a migration annotation but must not rewrite the original template rules or resolved users without an auditable version change.

## Weight invariants (policy A, 2026-10-01)

- Dimension/item weights are finite values in 0..100; groups are nonempty and match dimensions.
- Composite weight is decimal HALF_UP `ROUND(dimension_weight * item_weight / 100, 1)`; rounded composite weights sum to 100.
- Dimension and internal item totals are informational only; no normalization occurs.
- Canonical calculation rejects inconsistent stored composite values rather than rewriting historical snapshots.

## State transitions

```text
draft
  -> self_evaluating       (assessment created and self tasks available)
  -> scoring               (self requirements complete)
  -> awaiting_review       (all required scoring tasks complete and review is configured)
  -> awaiting_employee_confirmation (canonical result available)
  -> completed              (employee confirms online or paper confirmation is archived)

completed
  -> completed              (authorized operator appends sign-off supplement; no workflow transition)

Any active state -> rejected    (authorized reviewer returns work)
Any non-completed state -> cancelled (authorized administrator)
```

Transitions are backend operations. A request to jump to a state is rejected when required tasks or review conditions are not satisfied.

## Compatibility and migration rules

1. Legacy `KPIScore` and invitation rows remain readable and are never deleted by this feature.
2. Completed legacy assessments retain their original total and use a `legacy` calculation version unless an operator explicitly approves a recalculated result.
3. In-progress assessments with complete snapshots and uniquely resolvable scorers may receive tasks during an apply migration.
4. Ambiguous assessments receive manual-review findings and remain unchanged.
5. Every apply batch records the operator, source/target versions, affected IDs, skipped IDs, and errors.
6. Completed assessments may receive one or more sign-off supplements after an online or paper final confirmation. Supplements do not replace the confirmation, recalculate the result, reopen scoring, or alter historical snapshots; their upload, list, download, and deletion-cleanup behavior is independently auditable.
