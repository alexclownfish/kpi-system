# Implementation Plan: Evaluation Workflow Unification

**Branch**: `007-evaluation-workflow-unification` | **Date**: 2026-09-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/007-evaluation-workflow-unification/spec.md`

## Summary

This feature replaces the current split assessment flow with one backend-owned model for item-level scoring tasks, workflow transitions, weighted calculation, historical snapshots, and exports. The implementation will preserve the legacy `KPIScore`, invitation, and total fields for compatibility while making new assessment records use explicit scoring-task assignments and one canonical calculation result. Production data is handled in stages: first a read-only audit, then a dry-run migration, then an explicitly authorized migration batch.

## Technical Context

**Language/Version**: Go 1.23.0/1.23.11 backend; TypeScript with Next.js 15 and React 19 frontend

**Primary Dependencies**: Gin, GORM, SQLite, JWT/bcrypt, Excelize, Playwright, existing Radix UI components

**Storage**: Existing SQLite database and Compose named volumes; add backward-compatible tables and indexes through GORM migration. Paper-signature files use the existing sign-off attachment volume; supplement rows are separate from final confirmation rows.

**Testing**: `go test ./...`; TypeScript `tsc --noEmit`; Next.js production build; Playwright/manual acceptance; contract tests for protected API responses

**Target Platform**: Linux Docker Compose deployment with local macOS development support

**Project Type**: Full-stack web application with Go API and Next.js frontend

**Performance Goals**: Audit and calculation for a single assessment should complete within 2 seconds for up to 500 items and 20 scorers; export generation should complete within 10 seconds for a 100-assessment batch under normal deployment resources

**Constraints**: Preserve existing business rows and Compose volumes; no GET-side writes; all protected writes require backend authorization and structured error responses; published templates are immutable

**Scale/Scope**: Existing MVP organization with thousands of employees, tens of thousands of assessments, up to 100 items per template, and up to 10 scorer assignments per item

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Backend mandatory authorization**: PASS. Score writes, migration, responsibility maintenance, and export access remain protected by Go middleware plus task-level authorization.
- **II. Data-scope isolation**: PASS. Audit, scoring tasks, assessments, and exports will apply existing data-scope helpers; out-of-scope records are rejected before response serialization.
- **III. Explicit contracts and user feedback**: PASS. New APIs are documented in `contracts/`; UI states and error payloads are part of the tasks.
- **IV. Testable incremental delivery**: PASS. User stories are independently testable; regression tests cover allowed/denied scoring and data consistency.
- **V. Data and deployment safety**: PASS. GORM migration is additive; audit/dry-run are non-destructive; no Compose volume reset is required.
- **VI. Single code source/minimal complexity**: PASS. Changes target only `app/`, `components/`, `lib/`, and `server/`; legacy tables are retained only for compatibility.

**Post-design re-check**: PASS. No constitution exception is required.

## Project Structure

### Documentation (this feature)

```text
specs/007-evaluation-workflow-unification/
├── plan.md              # This file
├── research.md          # Phase 0 decisions
├── data-model.md        # Phase 1 entities and invariants
├── quickstart.md        # Phase 1 validation guide
├── contracts/           # Phase 1 API contracts
└── tasks.md             # Phase 2 implementation tasks
```

### Source Code (repository root)

```text
server/
├── models/models.go                         # new task/responsibility/result entities
├── models/database.go                       # additive migration and constraints
├── handlers/assessment_audit.go             # read-only audit APIs
├── handlers/assessment_repair.go            # dry-run/apply migration APIs
├── handlers/scoring.go                      # task list, task score writes, submit
├── handlers/evaluation_workflow.go          # guarded transitions and completion checks
├── handlers/evaluation_calculation.go       # snapshot-based canonical calculation
├── handlers/evaluation_template.go          # snapshot and scorer resolution integration
├── handlers/result_export.go                # canonical result adapters for Excel/PDF
├── handlers/confirmation.go                 # online/paper confirmation and completed supplements
├── handlers/statistics.go                   # canonical result aggregation
├── handlers/permissions.go                  # task-level authorization helpers
├── handlers/*_test.go                       # unit, contract, and regression tests
└── routes/routes.go                         # protected API routes

app/
├── evaluations/page.tsx                     # dynamic task/role scoring and completed-signoff UI
├── templates/page.tsx                       # immutable published template UX
└── ...                                      # existing pages use canonical result responses

components/
└── evaluation-scoring-task-list.tsx         # reusable task and status presentation

lib/
└── api.ts                                   # typed client methods and error handling

tests/
└── e2e/evaluation-workflow.spec.ts          # end-to-end acceptance coverage
```

**Structure Decision**: Keep the existing full-stack layout. Add backend services and handlers under `server/`, typed client/UI changes under `lib/`, `app/`, and `components/`, and preserve the current export and authentication infrastructure. Do not modify `kpi-main/` or other historical copies.

## Architecture Decisions

1. **Explicit task ownership**: one `EvaluationScoringTask` represents one assessment-item-scorer-user assignment. A unique constraint prevents duplicate task creation.
2. **Snapshot-first history**: assessment creation persists template, scorer rule, resolved user, calculation, and export-layout snapshots. All historical reads and exports use the snapshot.
3. **Canonical result service**: `BuildEvaluationResult` is the only authority for item aggregation, weighted contribution, total, grade, and calculation version. Client totals are ignored.
4. **Responsibilities separate from RBAC**: add scoped business responsibility assignments without reusing `Role` as a scorer type. Existing RBAC continues to protect pages and APIs.
5. **Non-destructive migration**: audit and dry-run are read-only; apply mode writes additive task/result/audit records and preserves legacy fields and values.
6. **No GET side effects**: invitation/task backfill is an explicit repair operation, not part of ordinary assessment reads.
7. **Source weight policy A (2026-10-01)**: Reuse one backend validator for save/publish/canonical calculation: finite 0..100 values, matching nonempty groups, and sum of decimal HALF_UP `ROUND(dimension × item / 100, 1)` equal to 100. Do not enforce internal or dimension totals or normalize. Frontend uses decimal-safe preview rounding and the same composite gate; local sums are neutral informational values. Validate snapshot composite values against original inputs, without modifying stored snapshots. Regression fixtures come from the immutable source manifest; runtime verification uses a disposable backend and preserves the ordinary Compose volumes.
8. **Post-completion sign-off evidence**: Keep `EvaluationConfirmation` as the immutable single final confirmation. A separate append-only `EvaluationSignoffSupplement` accepts validated paper files only after `completed` plus an existing confirmation; it preserves status/result/snapshot identity, applies the same data-scope boundary, and writes a dedicated audit action. Deletion cleans rows and files transactionally.

## Implementation Phases

### Phase 0 - Audit and safety baseline

- Add read-only audit service and report format.
- Add tests proving no write side effects and deterministic output.
- Add feature flags/configuration so new writes can be enabled per environment.

### Phase 1 - Data model and contracts

- Add scoring-task, responsibility-assignment, canonical-result, and migration-audit entities.
- Add additive indexes and unique constraints.
- Document protected endpoints and stable error payloads.

### Phase 2 - New assessment creation and task authorization

- Resolve template responsibility rules to users during assessment creation.
- Persist the resolved snapshot and explicit tasks.
- Enforce task-level authorization for reads and writes.

### Phase 3 - Workflow and calculation

- Implement backend-controlled state transitions.
- Enforce required-task completion and optimistic concurrency.
- Calculate canonical results from snapshot weights and scorer aggregation rules.

### Phase 4 - UI, exports, and statistics

- Render dynamic scorer tasks instead of fixed self/manager/HR columns.
- Route Excel/PDF/sign-off/statistics through canonical result adapters.
- Add completed-assessment sign-off supplement upload/list/download controls with a clear “completion state unchanged” message.
- Show structured loading/success/error states, including invalid-file, scope, and terminal-state failures.

### Phase 5 - Migration and convergence

- Provide dry-run and authorized apply operations.
- Mark ambiguous legacy records for manual review.
- Run `speckit-converge` and close old write paths only after compatibility checks pass.
- Verify assessment deletion removes supplement rows and physical attachments without leaving orphan files.

## Complexity Tracking

No constitution violations or additional projects are required. The legacy model remains read-compatible during migration so that the feature can be delivered incrementally without a destructive rewrite.
