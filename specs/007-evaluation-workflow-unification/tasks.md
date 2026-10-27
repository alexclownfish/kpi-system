# Tasks: Evaluation Workflow Unification

**Input**: Design documents from `/specs/007-evaluation-workflow-unification/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/evaluation-workflow-api.md`

**Tests**: Automated regression tests are required by FR-022 and the project constitution. Test tasks precede their corresponding implementation tasks.

**Organization**: Tasks are grouped by user story so each stage has an independent validation checkpoint.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it targets a separate file and does not depend on an incomplete task.
- **[Story]**: Maps a task to a user story in `spec.md`.

## Phase 1: Setup and Safety Baseline

**Purpose**: Establish stable response, configuration, and test fixtures before changing workflow data.

- [X] T001 Record existing legacy/new workflow behavior and route ownership in `specs/007-evaluation-workflow-unification/research.md` using current `server/handlers/kpi.go`, `server/handlers/invitation.go`, `server/handlers/result_export.go`, and `server/routes/routes.go`
- [X] T002 Add evaluation workflow feature-mode configuration with disabled/audit/new-write values in `server/models/database.go` and document environment behavior in `.env.example`
- [X] T003 [P] Add reusable stable success/error envelope helpers and request IDs in `server/handlers/response.go`
- [X] T004 [P] Add shared frontend API error parsing for `{error.code,error.message,error.details,request_id}` in `lib/api.ts`
- [X] T005 Add test database fixture builders for templates, snapshots, scorer responsibilities, legacy scores, and invitations in `server/handlers/evaluation_workflow_test_helpers.go`

**Checkpoint**: Feature can be disabled or placed in audit-only mode; existing production data remains untouched.

---

## Phase 2: Foundational Data and Authorization

**Purpose**: Add the minimum additive schema and shared authorization/calculation structures that block all user-story work.

**⚠️ CRITICAL**: Do not enable new workflow writes until this phase and its tests pass.

- [X] T006 Add `EvaluationResponsibility`, `EvaluationScoringTask`, `EvaluationCalculationResult`, `AssessmentAuditFinding`, and `MigrationBatch` models with the exact field/state constraints from `specs/007-evaluation-workflow-unification/data-model.md` in `server/models/models.go`
- [X] T007 Add additive AutoMigrate entries, unique indexes, and safe compatibility checks for the new entities in `server/models/database.go`
- [X] T008 [P] Add model tests proving `(evaluation_id,item_id,scorer_user_id)` uniqueness, valid zero scores, nullable missing scores, and no legacy-row deletion in `server/models/evaluation_workflow_models_test.go`
- [X] T009 Add snapshot parser/version adapter for legacy and `weighted-v2` snapshots in `server/handlers/evaluation_snapshot.go`
- [X] T010 [P] Add snapshot parser tests for missing, legacy, valid, duplicate-scorer, and malformed snapshots in `server/handlers/evaluation_snapshot_test.go`
- [X] T011 Add evaluation visibility and task-write authorization helpers that combine RBAC, data scope, task ownership, override permission, active state, and optimistic version checks in `server/handlers/evaluation_access.go`
- [X] T012 [P] Add allowed/denied/data-scope/task-version authorization tests in `server/handlers/evaluation_access_test.go`
- [X] T013 Register protected workflow, responsibility, audit, and migration route groups with explicit permissions in `server/routes/routes.go`

**Checkpoint**: Schema is additive, protected routes exist, and task ownership can be enforced without relying on frontend visibility.

---

## Phase 3: User Story 1 - Audit Inconsistent Assessments (Priority: P1) 🎯 Safety MVP

**Goal**: Produce a deterministic read-only report that separates clean, automatically migratable, and manual-review records.

**Independent Test**: Run the audit twice on mixed fixtures and prove identical findings and unchanged business rows/timestamps.

### Tests for User Story 1

- [X] T014 [P] [US1] Add contract tests for audit filter validation, permission denial, finding schema, and stable errors in `server/handlers/assessment_audit_test.go`
- [X] T015 [P] [US1] Add regression test proving audit execution changes zero assessment, legacy score, invitation, task, snapshot, and timestamp rows in `server/handlers/assessment_audit_readonly_test.go`

### Implementation for User Story 1

- [X] T016 [US1] Implement deterministic checks for snapshot presence/validity, item counts, legacy score counts, task counts, duplicate assignments, legal statuses, scorer resolution, and recalculable totals in `server/handlers/assessment_audit.go`
- [X] T017 [US1] Implement `POST /api/assessment-audits/preview` with data-scope filtering and stable finding codes in `server/handlers/assessment_audit.go`
- [X] T018 [P] [US1] Add audit request/response types and client method in `lib/api.ts`
- [X] T019 [US1] Add administrator audit summary/filter/finding UI with loading, empty, success, and actionable error states in `app/evaluations/audit/page.tsx`
- [X] T020 [US1] Add navigation visibility for the assessment audit page using backend-granted permission data in `components/sidebar.tsx`

**Checkpoint**: HR can inspect data inconsistency safely before any migration operation exists.

---

## Phase 4: User Story 2 - Assign Explicit Scoring Tasks (Priority: P1)

**Goal**: Create exact item-level scorer assignments from a published template snapshot and expose only authorized tasks.

**Independent Test**: Create A→PM, B→department manager, C→PM+product manager and verify exact task counts, visibility, ownership, and duplicate prevention.

### Tests for User Story 2

- [X] T021 [P] [US2] Add task generation tests for item-specific scorer types, duplicate user resolution, inactive users, missing responsibilities, and unchanged assignments after RBAC changes in `server/handlers/scoring_task_factory_test.go`
- [X] T022 [P] [US2] Add scoring-task list/write contract tests including allowed owner, denied unassigned reviewer, zero score, range validation, not-applicable validation, lock state, and version conflict in `server/handlers/scoring_test.go`

### Implementation for User Story 2

- [X] T023 [US2] Implement scoped scorer resolution using `EvaluationResponsibility` with explicit fallback errors instead of role-name guessing in `server/handlers/evaluation_responsibility_resolver.go`
- [X] T024 [US2] Extend assessment snapshots with item-level resolved scorer users, source rules, aggregation rules, export orientation, and checksums in `server/handlers/evaluation_snapshot.go`
- [X] T025 [US2] Implement transactional task generation with the unique assessment-item-user rule in `server/handlers/scoring_task_factory.go`
- [X] T026 [US2] Integrate strict department, position, period, published-version, snapshot, and task validation into assessment creation in `server/handlers/kpi.go`
- [X] T027 [US2] Implement visible task listing and completion summary in `server/handlers/scoring.go`
- [X] T028 [US2] Implement draft-save and submit endpoints with ownership, range, not-applicable, task lock, and optimistic-concurrency checks in `server/handlers/scoring.go`
- [X] T029 [US2] Remove `ensureTemplateInvitations` writes from GET paths and retain only read-compatible invitation responses in `server/handlers/invitation.go` and `server/handlers/evaluation_template.go`
- [X] T030 [P] [US2] Add typed task/list/save/submit methods and scorer labels in `lib/api.ts`
- [X] T031 [US2] Add dynamic item-level scoring task component showing only assigned editable items in `components/evaluation-scoring-task-list.tsx`
- [X] T032 [US2] Replace fixed scorer edit actions with the task component while retaining legacy read-only display for legacy assessments in `app/evaluations/page.tsx`

**Checkpoint**: New assessments have exact scorer tasks; an unassigned user cannot write any item even with general review permission.

---

## Phase 5: User Story 3 - Complete a Template-Driven Workflow (Priority: P1)

**Goal**: Advance assessment states only from backend completion rules rather than fixed manager/HR stages.

**Independent Test**: Submit required work in different orders; confirmation remains blocked until all required tasks are valid, and a valid zero score counts as complete.

### Tests for User Story 3

- [X] T033 [P] [US3] Add table-driven workflow transition tests covering every allowed transition and every forbidden jump in `server/handlers/evaluation_workflow_test.go`
- [X] T034 [P] [US3] Add regression tests for required task missing, valid zero, allowed/disallowed not-applicable, rejected task, optional review, online confirmation, and paper confirmation in `server/handlers/evaluation_confirmation_workflow_test.go`

### Implementation for User Story 3

- [X] T035 [US3] Implement canonical workflow states and transition guards without scorer-specific state names in `server/handlers/evaluation_workflow.go`
- [X] T036 [US3] Implement required self/task completion evaluation and structured blocking details in `server/handlers/evaluation_workflow.go`
- [X] T037 [US3] Implement `POST /api/evaluations/:id/workflow/advance` without accepting client target status or total score in `server/handlers/evaluation_workflow.go`
- [X] T038 [US3] Remove arbitrary status/total writes for new workflow assessments while preserving legacy compatibility in `server/handlers/kpi.go`
- [X] T039 [US3] Route online confirmation, paper confirmation, objections, and rejection through workflow guards in `server/handlers/confirmation.go` and `server/handlers/kpi.go`
- [X] T040 [P] [US3] Add typed workflow state, blocking-task, and advance responses in `lib/api.ts`
- [X] T041 [US3] Replace the fixed self/manager/HR progress indicator with task-completion and backend workflow state in `app/evaluations/page.tsx`
- [X] T042 [US3] Show human-readable blocking reasons and retry-safe submission feedback in `app/evaluations/page.tsx`

**Checkpoint**: The backend, not the UI, decides when an assessment can enter review, confirmation, or completion.

---

## Phase 6: User Story 4 - Calculate One Authoritative Result (Priority: P1)

**Goal**: Calculate weighted item and total results from the immutable snapshot and expose one canonical result to all consumers.

**Independent Test**: Known input scores produce the expected weighted total, client totals are ignored, and every result consumer sees the same version and value.

### Tests for User Story 4

- [X] T043 [P] [US4] Expand calculation unit tests for dimension weight × item weight, multi-scorer average, zero score, rounding, not-applicable, and invalid weight totals in `server/handlers/evaluation_calculation_test.go`
- [X] T044 [P] [US4] Add integration tests proving forged client totals are ignored and canonical result versions update only after accepted score changes in `server/handlers/evaluation_result_test.go`

### Implementation for User Story 4

- [X] T045 [US4] Move/reuse pure weight helpers from `server/handlers/calculation.go` behind a snapshot-based canonical calculation service in `server/handlers/evaluation_calculation.go`
- [X] T046 [US4] Implement item aggregation, composite contribution, total, grade, calculation version, and deterministic rounding in `server/handlers/evaluation_calculation.go`
- [X] T047 [US4] Persist versioned `EvaluationCalculationResult` records and update the compatibility `KPIEvaluation.TotalScore` only from the canonical result in `server/handlers/evaluation_calculation.go`
- [X] T048 [US4] Trigger recalculation after accepted task submission and before confirmation using one database transaction in `server/handlers/scoring.go` and `server/handlers/evaluation_workflow.go`
- [X] T049 [US4] Implement `GET /api/evaluations/:id/result` with snapshot/data-scope validation in `server/handlers/evaluation_calculation.go`
- [X] T050 [P] [US4] Add canonical result types/client methods in `lib/api.ts`
- [X] T051 [US4] Render dimension weights, item weights, scorer results, weighted contributions, total, grade, and calculation version from the canonical response in `app/evaluations/page.tsx`

**Checkpoint**: A single backend result is authoritative; no client or legacy handler can set a new-workflow total directly.

---

## Phase 7: User Story 5 - Preserve History and Unify Exports (Priority: P2)

**Goal**: Make historical detail, Excel, PDF, sign-off PDF, and statistics consume the same immutable canonical result and export layout.

**Independent Test**: Modify the live template after completion and confirm all historical consumers retain original labels, portrait orientation, selected columns, total, and checksum.

### Tests for User Story 5

- [X] T052 [P] [US5] Add snapshot immutability and template-edit regression tests in `server/handlers/evaluation_snapshot_history_test.go`
- [X] T053 [P] [US5] Add parity tests comparing detail payload, Excel extraction, detail PDF text/orientation, batch sign-off PDF text/orientation, and statistics total in `server/handlers/evaluation_output_parity_test.go`

### Implementation for User Story 5

- [X] T054 [US5] Refactor result snapshot creation to consume `EvaluationCalculationResult` and assessment snapshot labels/layout in `server/handlers/result_export.go`
- [X] T055 [US5] Route single Excel, single PDF, and batch sign-off PDF generation through the same canonical result adapter and portrait-orientation setting in `server/handlers/export.go`, `server/handlers/result_export.go`, and `server/handlers/batch_results.go`
- [X] T056 [US5] Update confirmation evidence to reference the canonical result version/checksum in `server/handlers/confirmation.go`
- [X] T057 [US5] Update department, employee, trend, dashboard, and sign-off statistics to use finalized canonical results with legacy fallback labeling in `server/handlers/statistics.go`
- [X] T058 [US5] Reject mutations to published template items through legacy item/template endpoints and direct users to clone/version flow in `server/handlers/kpi.go`, `server/handlers/template.go`, and `server/handlers/template_calculation.go`
- [X] T059 [P] [US5] Add frontend export/result version and actionable download-error handling in `lib/api.ts`
- [X] T060 [US5] Update evaluation export actions and status messages to use canonical result availability and shared selected fields in `app/evaluations/page.tsx`
- [X] T061 [US5] Update template export settings to persist and preview orientation/columns for the next published version only in `components/template-calculation-editor.tsx` and `app/templates/page.tsx`

**Checkpoint**: Every historical view and output agrees, and later template edits cannot change old assessments.

---

## Phase 8: User Story 6 - Separate RBAC From Evaluation Responsibilities (Priority: P2)

**Goal**: Let administrators maintain scoped business scoring responsibilities without changing system roles or rewriting existing tasks.

**Independent Test**: Assign an ordinary employee as PM for a scope, create an assessment, deactivate the responsibility, and verify only future resolution changes.

### Tests for User Story 6

- [X] T062 [P] [US6] Add responsibility CRUD, scope validation, duplicate assignment, inactive user, audit log, and existing-task immutability tests in `server/handlers/evaluation_responsibility_test.go`
- [X] T063 [P] [US6] Add data-scope allowed/denied contract tests for responsibility administration in `server/handlers/evaluation_responsibility_scope_test.go`

### Implementation for User Story 6

- [X] T064 [US6] Implement responsibility list/create/update/deactivate handlers with stable errors and audit logs in `server/handlers/evaluation_responsibility.go`
- [X] T065 [US6] Add responsibility permissions and seed them idempotently without granting them through role-name assumptions in `server/models/database.go`
- [X] T066 [US6] Remove PM/product-manager/CTO role-name fallback from new workflow resolution while retaining explicit legacy audit warnings in `server/handlers/evaluation_responsibility_resolver.go`
- [X] T067 [P] [US6] Add typed responsibility CRUD client methods in `lib/api.ts`
- [X] T068 [US6] Add the organization-performance “评分职责” administration page with scope, user, active state, validation, and success/failure feedback in `app/evaluation-responsibilities/page.tsx`
- [X] T069 [US6] Add permission-aware navigation for the responsibility page in `components/sidebar.tsx`

**Checkpoint**: Business scoring responsibilities no longer depend on the user's single system role.

---

## Phase 9: User Story 7 - Migrate Safely and Converge (Priority: P3)

**Goal**: Migrate deterministic in-progress legacy assessments in reviewed batches without overwriting ambiguous history.

**Independent Test**: Dry-run mixed fixtures, apply only the unchanged eligible batch, and verify additive records/audit logs with all legacy rows preserved.

### Tests for User Story 7

- [X] T070 [P] [US7] Add dry-run determinism/no-write/manual-review tests in `server/handlers/assessment_repair_test.go`
- [X] T071 [P] [US7] Add apply-batch checksum conflict, already-applied, authorization, rollback, and legacy-preservation tests in `server/handlers/assessment_migration_test.go`

### Implementation for User Story 7

- [X] T072 [US7] Implement migration eligibility planning from audit findings with source checksums and explicit manual-review reasons in `server/handlers/assessment_repair.go`
- [X] T073 [US7] Implement `POST /api/assessment-migrations/dry-run` and immutable reviewed batch payloads in `server/handlers/assessment_repair.go`
- [X] T074 [US7] Implement transactional apply with source revalidation, task/result creation, per-record rollback, skipped-record reporting, and audit logging in `server/handlers/assessment_repair.go`
- [X] T075 [US7] Implement migration batch status retrieval without GET side effects in `server/handlers/assessment_repair.go`
- [X] T076 [P] [US7] Add typed dry-run/apply/status client methods in `lib/api.ts`
- [X] T077 [US7] Add migration review UI that separates eligible and manual-review records and requires explicit apply confirmation in `app/evaluations/migration/page.tsx`
- [X] T078 [US7] Add audit/migration operation entries to existing audit-log views or administrator diagnostics in `app/settings/page.tsx`

**Checkpoint**: Eligible records can move to the new workflow without deleting or silently recalculating ambiguous production history.

---

## Phase 10: Polish and Cross-Cutting Validation

**Purpose**: Complete regression, documentation, performance, and deployment gates across all stories.

- [X] T079 Add end-to-end scenarios for audit, exact task assignment, allowed/denied writes, zero score, workflow advancement, snapshot immutability, output parity, responsibility changes, and migration dry-run in `tests/e2e/evaluation-workflow.spec.ts`
- [X] T080 [P] Add fixture cleanup using unique feature prefixes without deleting volumes in `tests/e2e/helpers/cleanup.ts`
- [X] T081 [P] Update operator and deployment documentation for audit-only, new-write enablement, migration dry-run/apply, and rollback boundaries in `README.md` and `docs/production-deployment.md`
- [X] T082 Run and record backend `go test ./...`, frontend `npx tsc --noEmit`, `npm run build`, and relevant Playwright results in `specs/007-evaluation-workflow-unification/quickstart.md`
- [ ] T083 Run `docker compose config`, user-managed `docker compose up -d --build`, service health verification, and page acceptance steps from `specs/007-evaluation-workflow-unification/quickstart.md`
- [X] T084 Run Spec Kit convergence against the implemented code and append any remaining gaps to `specs/007-evaluation-workflow-unification/tasks.md`

## Phase 11: Convergence

- [X] T085 [US3] Add contract regression tests covering online confirmation, paper confirmation, objection submission, objection handling, rejection, and terminal-state guards for both legacy and weighted-v2 assessments
- [X] T086 [US7] Add an administrator audit-log/diagnostics view for `assessment_migration_*`, `evaluation_responsibility_*`, and workflow transition operations
- [X] T087 [US5] Expose export request ID, stable error code, result version, and retry guidance in typed clients and evaluation export failure UI
- [X] T088 [US5] Enforce export-layout version/published-template next-version semantics in the UI and add a contract test proving historical snapshots retain their layout (partial)
- [ ] T089 [P] Run Docker Compose config/build/health and Playwright acceptance in a real service environment, then record results and resolve any failures (environment-dependent)

---

## Dependencies and Execution Order

### Phase dependencies

- Phase 1 has no dependency and can start immediately.
- Phase 2 depends on Phase 1 and blocks every workflow write.
- US1 depends on Phase 2 and is the first deployable safety increment.
- US2 depends on Phase 2 and the foundational responsibility schema.
- US3 depends on US2 task generation and authorization.
- US4 depends on US2 task data and US3 transition integration.
- US5 depends on US4 canonical results.
- US6 backend schema/resolver foundation is available after Phase 2; its administration UI may be completed in parallel with US5 after US2 resolution tests pass.
- US7 depends on US1 audit, US2 task creation, and US4 canonical results.
- Phase 10 depends on all stories included in the release.

### Parallel opportunities

- Frontend API typing tasks can run in parallel with backend handlers after contracts stabilize.
- Unit/contract tests marked `[P]` can be written in parallel because they target separate test files.
- US5 export parity and US6 administration UI can proceed in parallel after their stated backend dependencies are complete.
- Documentation and E2E fixture cleanup can run in parallel with final integration.

## Independent Test Summary

| Story | Independent acceptance |
|---|---|
| US1 | Two audit runs return identical findings and mutate zero rows. |
| US2 | Template item/scorer rules create exact unique tasks; unassigned users receive 403. |
| US3 | Required tasks, including valid zero scores, exclusively control workflow advancement. |
| US4 | Known weighted inputs produce one backend total and forged client totals are ignored. |
| US5 | Detail, Excel, PDFs, and statistics match after the live template changes. |
| US6 | Responsibility changes affect future resolution only and do not alter RBAC or existing tasks. |
| US7 | Dry-run is read-only; apply migrates only unchanged eligible records with audit evidence. |

## Implementation Strategy

### Safety MVP

1. Complete Phase 1 and Phase 2.
2. Complete US1 read-only audit.
3. Deploy in audit-only mode and inspect real data before enabling any new workflow writes.

### Core workflow MVP

1. Complete US2 explicit scoring tasks.
2. Complete US3 backend workflow.
3. Complete US4 canonical calculation.
4. Validate one new test template and assessment end to end before enabling the mode for more users.

### Full convergence

1. Complete US5 output unification and US6 responsibility administration.
2. Complete US7 controlled migration.
3. Run Phase 10 gates and Spec Kit convergence.

## Notes

- Do not use `git reset --hard`, `git checkout --`, `docker compose down -v`, or direct SQLite deletion during implementation or testing.
- Existing uncommitted work belongs to the user and must be preserved.
- The user will run `docker compose up -d --build`; implementation tasks must still run proportional local tests before handoff.
- Every completed implementation turn must include page-level manual test instructions.

## Phase 12: Convergence

Runtime acceptance evidence: `runtime-acceptance-20260930.md`. Findings below were reproduced against local Docker services, not inferred from build success.

- [X] T090 CRITICAL Enforce evaluation-owner authorization, valid score range, and workflow-state guards in `server/handlers/kpi.go` `UpdateSelfScore`, with cross-department denial regressions, per FR-007 and Constitution I/II (contradicts)
- [X] T091 CRITICAL Apply evaluation visibility and write authorization to comment read/create/update/delete endpoints in `server/handlers/comment.go`; verify an unrelated employee cannot read or write public comments on another assessment, per Constitution I/II and FR-022 (contradicts)
- [X] T092 Restore intended HR employee/manager onboarding with an explicit, non-escalating role-grant policy in `server/handlers/permissions.go` and `server/handlers/employee.go`; retain denial for super-admin/custom elevated roles and rerun MVP onboarding acceptance, per plan: existing permission infrastructure and Constitution III/IV (partial)
- [X] T093 Include explicit scoring-task assignments in evaluation list/detail/pending scopes without widening unrelated employee access in `server/handlers/permissions.go`, `server/handlers/evaluation_access.go`, and `server/handlers/kpi.go`; verify ordinary-employee PM can discover its assigned assessment, per FR-006 and US6/AC1 (contradicts)
- [X] T094 Validate all assessment creation paths for employee/template department and position matching, period/year/month/quarter validity, server-owned initial status and total; reject forged completed/999-score payloads and mismatched templates in `server/handlers/kpi.go`, per FR-009/FR-012 and T026 (contradicts)
- [X] T095 Consume canonical item results in employee KPI breakdown and correct monthly trend grouping/date generation in `server/handlers/statistics.go`; compare like-for-like filters and known 80/54 fixtures across reports, per FR-013 and SC-003 (partial)
- [X] T096 Make assessment deletion transactional and clean dependent tasks, calculations, snapshots, confirmations, comments, invitation scores and signoff attachments; include template-dimension cleanup and prove zero orphan rows/files after fixture cleanup, per Constitution V and T080
- [X] T097 Detect invalid period, forged completed status/total, missing final canonical results, and orphan workflow records in read-only audit findings in `server/handlers/assessment_audit.go`; invalid runtime fixtures currently reported eligible must instead be actionable findings, per FR-001/FR-002
- [X] T098 Render every canonical workflow state consistently in list badges, filters and task status labels using `app/evaluations/page.tsx` and the shared status helper; remove unknown-state output for self_evaluating/scoring and add UI regression coverage, per FR-008/FR-016 (partial)
- [X] T099 Expand `tests/e2e/evaluation-workflow.spec.ts` beyond its two audit/migration requests to exercise scoring, owner/unassigned access, zero scores, confirmation/objection/paper attachment, snapshot/layout freezing, statistics and export parity; use separate HR and authorized administrator migration fixtures, per FR-022 and T079/T089 (partial). The opt-in suite is complete in code; real service execution remains covered by T083/T089.

## Phase 13: Convergence

Source-workbook acceptance on 2026-10-01 confirms T090–T093/T095/T098 remain open. Requirements checklist: 16/16 checked. This phase adds only distinct gaps; source-workbook weight conflicts and HR project-effort aggregation remain tracked in feature 006.

- [X] T100 CRITICAL Restore editable owner self-evaluation fields for weighted-v2 assessments in `app/evaluations/page.tsx`, preserving frozen item names/maxima and valid zero scores; remove the unrelated legacy manager prerequisite from weighted self-score writes and add regressions, per FR-008/FR-009/FR-016 and US3 (partial)
- [X] T101 Hydrate export layout identically for automatic and explicit template selection in `server/handlers/evaluation_template.go`; test matching, assessment snapshot and historical layout preservation, per FR-014/FR-015 and US5/AC2 (partial)
- [X] T102 Calculate averages, item contributions and totals using decimal HALF_UP semantics instead of binary floating-point intermediate rounding in `server/handlers/evaluation_calculation.go`; regress 63 × 7.5% = 4.73 and zero/multi-scorer boundaries, per FR-011/FR-012 and SC-003 (contradicts)

## Phase 14: Convergence

Browser acceptance found that refreshing after one task submission unmounts every task editor and discards other unsaved inputs.

- [X] T103 Preserve other task editors and unsaved values when saving/submitting an individual scoring task; distinguish initial loading from refresh and verify sequential multi-item entry in `components/evaluation-scoring-task-list.tsx`, per FR-016 and US3 (partial)

## Phase 15: Convergence

Evidence: `business-repair-acceptance-20261001.md`. Existing T096/T097/T099 and
earlier open tasks are retained rather than duplicated. HR aggregation and the
source-weight policy remain in feature 006. Convergence is not a release sign-off.

- [X] T104 Add and verify template-responsibility-derived signing-label defaults for weighted-v2 exports, while preserving explicit custom signing labels and immutable historical layouts in `server/handlers/evaluation_snapshot.go`, `server/handlers/result_export.go` and template settings; regress source department-head/engineer/product-team signing roles per US5, FR-014/FR-015 and plan: snapshot-first exports
- [X] T105 Add canonical weighted export columns for dimension/item/composite weights, actual scorers and weighted contribution in `server/handlers/result_export.go`, shared payload/client types and `app/templates/page.tsx`; retain legacy-column compatibility, freeze selected columns and test Excel/single-PDF/batch-PDF parity per FR-011/FR-015 and US5/AC2

## Phase 16: Source Weight Policy A

**Goal**: Accept original source profiles without normalization; reject genuinely invalid composite totals.
**Independent Test**: Three previously blocked head profiles save/publish/complete with original weights and source expected results; invalid composite totals remain blocked. Existing history and normal Compose volumes are unchanged.

- [X] T106 [US4] Add three original department-head fixtures, rounding/range/nonfinite/group-shape and invalid-composite tests in `server/handlers/calculation_test.go`, `server/handlers/template_calculation_test.go` and `server/handlers/evaluation_calculation_test.go` before implementation, per FR-023 and US4/AC4
- [X] T107 [US4] Replace internal-total validation with rounded composite-total validation and validate snapshot composite consistency in `server/handlers/calculation.go` and `server/handlers/evaluation_calculation.go`, per FR-023
- [X] T108 [US4] Align frontend composite-total gating, decimal rounding and neutral internal totals in `components/template-calculation-editor.tsx` with regression checks in `tests/unit/template-weights.test.mjs`, per FR-016/FR-023
- [X] T109 [US4] Remove obsolete source-local-total blockers and run all 18 original profiles in `scripts/verify_workbook_runtime.py` against an isolated backend, recording score/export/confirmation results in `specs/007-evaluation-workflow-unification/weight-policy-acceptance-20261001.md`, per US4/AC4 and SC-003
- [X] T110 Run Go suite, TypeScript, targeted lint, production build, browser valid/invalid template verification and scoped convergence; update `specs/007-evaluation-workflow-unification/quickstart.md` with real results and page test instructions

**Dependencies**: T106 → T107 → T108 → T109 → T110. Frontend pure rounding tests may be prepared alongside T106. Earlier open tasks remain separately tracked.

## Phase 17: Convergence

Scoped review of policy A: save/publish/calculation/preview and 18-profile workflows meet FR-023. Existing open export, audit, deletion and aggregation work remains tracked. One additional visible inconsistency was reproduced on the template page.

- [X] T111 [US4] Display a 100-point maximum for weighted templates instead of summing per-item score maxima in `app/templates/page.tsx`; preserve legacy totals and add a pure regression in `lib/template-weights.ts` and `tests/unit/template-weights.test.mjs`, per FR-016 and plan: canonical weighted result (contradicts)

## Phase 18: Convergence

- [X] T112 [US3/US5] Lock every materialized scoring task atomically when online or paper confirmation finalizes an assessment; reject task save/submit in awaiting_employee_confirmation, completed, pending_confirm and other terminal states while preserving submitted status and canonical snapshot parity; add optional-task and legacy-terminal regressions per FR-008/FR-016 and Constitution I/III

## Phase 19: Manual acceptance regression fixes

Manual acceptance on 2026-10-13 found three gaps in the final output and audit surfaces. The implementation and automated regressions below close those gaps; real Compose/page execution remains tracked by T083/T089.

- [X] T113 [US5] Restore weighted contribution and grade in the unified Excel/PDF/sign-off export document, backfill missing evidence for completed weighted-v2 snapshots without changing snapshot identity or checksum, and expose the fields in template export settings per FR-013/FR-015 (partial)
- [X] T114 [US3/US5] Show the paper-signoff action for both `pending_confirm` and `awaiting_employee_confirmation`, hide it after completion or unresolved objection, and cover the state guard with a frontend regression per FR-008/FR-016 (partial)
- [X] T115 [US1] Make orphan workflow audit findings actionable with a bounded count and sample ID list while keeping the audit read-only; document the seven historical orphan scoring tasks and prevent new deletion paths from producing them per FR-001/FR-002 (partial)

## Phase 20: Completed sign-off supplement evidence

**Goal**: Allow an authorized operator to archive paper-signature evidence after
online or paper final confirmation without reopening the assessment, changing
the canonical result, or overwriting the original confirmation. Supplements are
append-only, scope protected, auditable, and cleaned up with the assessment.

**Independent Test**: Use a completed assessment with an online confirmation;
upload a valid image as an in-scope approver, verify that status/result/snapshot
and original confirmation are unchanged, list and download the supplement as an
in-scope viewer, then verify an out-of-scope viewer and a non-completed upload
are rejected. Repeat with an invalid file and confirm no row/file residue.

### Documentation and contract tasks

- [X] T116 [US8] Add `EvaluationSignoffSupplement` data-model invariants, append-only semantics, storage safety, state guard, and transactional deletion rules to `data-model.md`, per FR-024–FR-029.
- [X] T117 [US8] Document multipart upload, list, and download request/response/error contracts, permission and data-scope checks, audit redaction, and cleanup behavior in `contracts/evaluation-workflow-api.md`.
- [X] T118 [US8] Record the separate-confirmation design decision and add the completed-signoff supplement behavior to the implementation plan and research decisions.

### Existing implementation evidence

- [X] T119 [US8] Preserve the additive model/migration, protected routes, completed-state upload handler, list/download handlers, UI action, and typed client methods already present in `server/models`, `server/handlers/confirmation.go`, `server/routes/routes.go`, `app/evaluations/page.tsx`, and `lib/api.ts`.
- [X] T120 [US8] Retain the API regression covering successful online-confirmed upload, immutable original confirmation, unchanged `completed` status, upload audit with remark redaction, list/download, and rejection while awaiting confirmation in `server/handlers/confirmation_test.go`.

### Remaining acceptance coverage

- [X] T121 [US8] Add API regressions for unsupported/empty/oversized files, invalid dates/remarks, missing final confirmation, out-of-scope upload/list/download, concurrent append behavior, and failed persistence cleanup with zero orphan files in `server/handlers/confirmation_test.go`.
- [X] T122 [US8] Add deletion regression proving supplement rows and physical attachments are removed transactionally and restored on a failed deletion, alongside existing evaluation deletion coverage.
- [X] T123 [US8] Add opt-in browser acceptance for online confirmation followed by supplement upload, visible “完成状态不变” feedback, supplement list/download, repeat upload, and precise cleanup in `tests/e2e/evaluation-workflow.spec.ts` and `tests/e2e/README.md`; real-service execution remains environment-dependent and is recorded as pending in the dated acceptance note.
- [X] T124 [US8] Align supplement upload/list/download responses with the common request-ID/error envelope (while preserving the legacy `data`, `evaluation.status`, and file-stream behavior; successful downloads echo `X-Request-ID`) and add contract regressions for request IDs and stable error codes, per FR-016/FR-021 (Go suite pending environment verification).

**Dependencies**: T116 → T117 → T119/T120 → T121/T122/T124 → T123. T121/T122/T124 may run in parallel; T123 depends on a built service and available browser credentials.

## Phase 21: Convergence — HTTPS export download URLs

- [X] T125 [US5] Preserve the outer TLS protocol through the internal Docker Nginx `/api/` and SSE proxy so backend-generated PDF/Excel download URLs retain `https://` behind an HTTPS gateway; validate the Nginx image configuration with `nginx -t` and document the required rebuild/reload in the production migration guide.

**Evidence (2026-10-27)**: `nginx/conf.d/kpi.conf` forwards `$http_x_forwarded_proto` for API/SSE traffic and falls back to HTTP when accessed directly. The built test image passed `nginx -t` with service host aliases. Production must rebuild the `nginx-gateway` image before page verification.

- [X] T126 [US5] Extend T125 with the public `:18182` origin requirement (US5/AC4), protocol fallback/validation on API/SSE/health/frontend paths, a real Docker Nginx proxy regression in `tests/nginx-forwarded-origin.py`, and outer-gateway `$http_host` plus explicit Compose rebuild instructions in `quickstart.md` and `docs/production-postgres-migration.md`. Confirm the original config fails the regression and the fixed config passes; production PDF download remains pending user validation.

**T126 evidence (2026-10-27)**: Original `afb4c92` Nginx configuration reproduced `http://kpi.auracor.net:18182/...` for an HTTPS API request. The fixed configuration passed all 20 actual proxy checks (four paths × five origin cases) in an isolated Nginx container. T125's initial raw-header forwarding is superseded by the validated map and direct-protocol fallback. Analyze: US5/AC4 → T126 has full scoped coverage, no constitution conflict; Converge: no remaining local proxy gap, but external gateway edits and real production PDF/Excel downloads are explicitly unverified. No business database was accessed by this test.
