# Research: Evaluation Workflow Unification

**Date**: 2026-09-30

## Current ownership map

| Concern | Current source | Observed conflict |
|---|---|---|
| Assessment creation/status/legacy score writes | `server/handlers/kpi.go` | Fixed self/manager/HR fields and client-updatable status/total coexist with template scorer rules. |
| Template scorer resolution/snapshot | `server/handlers/evaluation_template.go` | One user is resolved per scorer type and GET/invitation compatibility can create broad all-item invitations. |
| Invitation scoring | `server/handlers/invitation.go` | Invitation ownership is scorer-level rather than item-level and can diverge from `KPIScore`. |
| Weight helpers | `server/handlers/calculation.go` | Pure weighted calculation exists but is not the sole business calculation path. |
| Result snapshot/PDF/Excel | `server/handlers/result_export.go`, `server/handlers/export.go`, `server/handlers/batch_results.go` | Output paths read different combinations of current template, legacy score fields, and snapshot data. |
| Statistics | `server/handlers/statistics.go` | Aggregation trusts `KPIEvaluation.TotalScore`, whose origin is not consistently canonical. |
| Protected routes | `server/routes/routes.go` | General permissions are present, but item-level task ownership is not represented. |
| Evaluation UI | `app/evaluations/page.tsx` | Fixed self/manager/HR presentation cannot render template-driven scorer responsibilities. |

## Decision 1: Keep the legacy score model for compatibility, but make explicit scoring tasks authoritative for new workflow records

- **Decision**: Add an additive task model rather than deleting `KPIScore`, `EvaluationInvitation`, or legacy `TotalScore` fields in the first release.
- **Rationale**: Existing assessments and exports already depend on those tables. A destructive rewrite would make production recovery and historical comparison impossible. New task records provide the missing assignment granularity without invalidating legacy reads.
- **Alternatives considered**:
  - Replace all legacy tables immediately: rejected because it risks data loss and requires an all-at-once migration.
  - Keep using `EvaluationInvitation` as the task model: rejected because it has no item-level scorer identity, responsibility type, or concurrency state.

## Decision 2: One task per assessment-item-user pair

- **Decision**: The stable identity of a scoring task is `(evaluation_id, item_id, scorer_user_id)` with a unique database constraint.
- **Rationale**: The task is the smallest unit needed for authorization, required completion checks, conflict detection, and dynamic UI rendering. Responsibility type and source rule are metadata rather than identity.
- **Alternatives considered**:
  - One invitation per scorer with embedded item list: rejected because it cannot safely handle partial completion or two responsibilities resolving to the same user.
  - One score row per item with a JSON map of scorers: rejected because it is difficult to authorize, query, constrain, and migrate.

## Decision 3: Separate business responsibilities from RBAC roles

- **Decision**: Add scoped responsibility assignments for `PM`, `DEPARTMENT_MANAGER`, `PRODUCT_MANAGER`, and `CTO`, while retaining `Role`, `Permission`, and `DataScope` for application authorization.
- **Rationale**: A person can be an ordinary employee in the application and still be a project-manager scorer for a specific department or project. Existing role changes must not invalidate assessment-time assignments.
- **Alternatives considered**:
  - Treat each scorer type as a system role: rejected because it couples UI/API access to business responsibility and caused the current migration failure.
  - Resolve scorer users on every page load: rejected because it makes historical results change when organizational data changes.

## Decision 4: Snapshot all calculation inputs at assessment creation

- **Decision**: Store template metadata, dimension/item weights, required flags, scorer rules, resolved scorer users, export layout, and calculation version in the assessment snapshot.
- **Rationale**: Historical assessments must be reproducible even after template edits, responsibility changes, or employee transfers.
- **Alternatives considered**:
  - Read the current template for old assessments: rejected because old totals and exports change over time.
  - Snapshot only item IDs: rejected because labels, weights, scoring rules, and export layout would still drift.

## Decision 5: Canonical calculation result is server-owned

- **Decision**: Use one calculation service to aggregate valid task scores, apply dimension/item weights, calculate total and grade, and expose a versioned result object used by pages, exports, and statistics.
- **Rationale**: The current code has a tested pure weighting function but business handlers still sum legacy fields. A single service closes that gap and prevents client-supplied totals.
- **Default rule**: multiple valid scorer scores for an item are averaged; `0` is valid; missing and not-applicable are distinct states.
- **Alternatives considered**:
  - Let each endpoint calculate independently: rejected because it caused the current detail/export/statistics divergence.
  - Trust a front-end total: rejected on integrity and authorization grounds.

## Decision 6: Backend-controlled workflow transitions

- **Decision**: Use states `draft`, `self_evaluating`, `scoring`, `awaiting_review`, `awaiting_employee_confirmation`, `completed`, `rejected`, and `cancelled`; transitions are derived from required task completion.
- **Rationale**: The old fixed `manager_evaluated` stage cannot represent PM, product manager, CTO, or multi-scorer flows.
- **Alternatives considered**:
  - Add more hard-coded states for each scorer type: rejected because the state count grows with template configuration.
  - Allow clients to set arbitrary status: rejected because it bypasses required-task checks.

## Decision 7: Read-only audit, then dry-run migration, then explicit apply

- **Decision**: Provide an audit report and dry-run migration before any apply action. Ambiguous records remain legacy and receive manual-review findings.
- **Rationale**: Existing data is not uniformly safe to infer. The migration must be explainable and reversible at the record level.
- **Alternatives considered**:
  - Automatically recalculate and overwrite all historical totals: rejected because it destroys the evidence needed to resolve disagreements.
  - Require a complete manual rebuild: rejected because clean in-progress records can be migrated deterministically.

## Decision 8: No business writes from GET requests

- **Decision**: Move invitation/task backfill out of ordinary GET handlers into explicit repair or migration operations with audit logging.
- **Rationale**: Read requests must be repeatable and side-effect free; this is required by the project constitution and avoids duplicate writes under concurrency.
- **Alternatives considered**:
  - Keep lazy backfill on page load: rejected because it produces hidden mutations and makes audit history unreliable.

## Decision 9: Preserve source weights (accepted A, 2026-10-01)

- **Decision**: Validate the sum of individually rounded composite weights, not local item totals or nominal dimension totals. Keep all original values and decimal HALF_UP precision.
- **Rationale**: The structure/electronics/software department-head profiles have local totals of 73/80/120/140 but composite totals of 100; normalization would change the actual workbook results.
- **Alternatives considered**: normalize per dimension or repair workbook inputs; rejected by the user's A selection. No additional ambiguity remains for this increment.

## Decision 10: Append completed sign-off evidence without reopening the workflow

- **Decision**: Keep one immutable `EvaluationConfirmation` per assessment and add an append-only `EvaluationSignoffSupplement` record for paper-signature files received after an online or paper confirmation. Accept supplements only when the assessment is `completed` and a final confirmation exists; preserve the assessment state, canonical result, result snapshot, checksum, and original confirmation metadata.
- **Rationale**: Operationally, employees may confirm online before HR receives the signed paper copy. Treating the later scan as a second confirmation would reopen a finalized workflow or overwrite the evidence that was actually confirmed. A separate record preserves both events and supports multiple copies or replacement scans without mutating history.
- **Authorization**: Upload requires approval permission plus assessment data-scope access. List/download requires assessment-view permission plus the same scope. The read endpoints are side-effect free.
- **Audit and cleanup**: Every successful upload records `upload_signoff_supplement` with the evaluation and supplement IDs and non-sensitive file metadata. Transactional assessment deletion removes supplement rows and physical files; a failed insert removes the newly written file.
- **Alternatives considered**:
  - Update `EvaluationConfirmation` in place: rejected because it destroys the original confirmation method, timestamp, and attachment provenance.
  - Reopen the assessment to `awaiting_employee_confirmation`: rejected because scoring and canonical results must remain locked after final confirmation.
  - Store the scan only in object storage without a row: rejected because uploader, signed date, audit linkage, scope checks, and deletion cleanup require a durable application record.
