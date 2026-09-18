# Tasks: 统一绩效结果导出

**Input**: Design documents from `/specs/005-unified-result-exports/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: 本 Feature 涉及 API 契约、历史快照兼容、权限/范围和两条导出链路统一，必须包含自动化回归测试。

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 Verify existing Git, Docker, TypeScript and Go ignore patterns remain sufficient in `.gitignore`, `.dockerignore`, and `eslint.config.mjs`
- [X] T002 Record the active Spec Kit feature directory in `.specify/feature.json` and retain artifacts under `specs/005-unified-result-exports/`

---

## Phase 2: Foundational (Blocking Prerequisites)

- [X] T003 Add nullable `ExportLayoutJSON` storage to `KPITemplate` in `server/models/models.go`, preserving existing rows through AutoMigrate
- [X] T004 Define layout presets, field catalog, normalization and validation with title 1-60 chars, 2-11 unique columns including `item_name` and `final_score`, and 1-6 unique signature labels of at most 20 chars in `server/handlers/result_export.go`
- [X] T005 [P] Add layout validation, default fallback and serialization tests in `server/handlers/result_export_test.go`
- [X] T006 Extend `resultSnapshotItem` with self/manager/HR scores and comments and embed the normalized layout in `resultSnapshotPayload` in `server/handlers/confirmation.go`
- [X] T007 [P] Add snapshot layout freeze and legacy snapshot fallback regression tests in `server/handlers/result_export_test.go`

**Checkpoint**: Layout configuration and immutable snapshot representation are available to all export stories.

---

## Phase 3: User Story 1 - 单个与批量导出保持一致 (Priority: P1) 🎯 MVP

**Goal**: Single Excel and batch PDF resolve the same columns, values, summary and signatures.

**Independent Test**: Export one finalized evaluation through both paths and compare the unified document model and generated field order.

- [X] T008 [US1] Add a unified export document builder that resolves metadata, configured columns, row values, summary and signature labels from a snapshot in `server/handlers/result_export.go`
- [X] T009 [P] [US1] Add document builder tests covering the final-signoff and full-process presets in `server/handlers/result_export_test.go`
- [X] T010 [US1] Implement the shared dynamic Excel renderer in `server/handlers/result_export.go`
- [X] T011 [US1] Implement the shared dynamic HTML/PDF renderer with wrapping and pagination-safe table headers in `server/handlers/result_export.go`
- [X] T012 [US1] Replace hard-coded single evaluation Excel construction with the unified renderer while preserving status, permission and `ApplyEvaluationScope` checks in `server/handlers/export.go`
- [X] T013 [US1] Replace hard-coded batch signoff PDF construction with the unified renderer while preserving filter scopes and `签字回收汇总.xlsx` in `server/handlers/batch_results.go`
- [X] T014 [US1] Add allowed and denied single-export regression tests plus mixed-template batch document tests in `server/handlers/result_export_test.go`

**Checkpoint**: Existing single Excel and batch PDF entry points share one business representation.

---

## Phase 4: User Story 2 - 按考核模板配置导出版式 (Priority: P1)

**Goal**: KPI editors configure controlled export fields and signatures per template.

**Independent Test**: Save different layouts on two templates and verify normalized API responses and distinct exports.

- [X] T015 [US2] Accept and validate `export_layout` on template create/update, return normalized layouts on template reads, and audit layout changes in `server/handlers/template.go`
- [X] T016 [P] [US2] Add template layout request/response and invalid-config handler tests in `server/handlers/result_export_test.go`
- [X] T017 [P] [US2] Add `ResultExportLayout`, presets, field metadata and template API typing in `lib/api.ts`
- [X] T018 [US2] Add a “结果导出设置” tab with preset selection, title, field enable/order controls, summary/opinion switches and editable signature labels in `app/templates/page.tsx`
- [X] T019 [US2] Add loading, success and actionable failure feedback for layout saves in `app/templates/page.tsx`

**Checkpoint**: Different templates can safely produce different controlled signoff layouts.

---

## Phase 5: User Story 3 - 冻结历史导出版式 (Priority: P2)

**Goal**: Existing snapshots reproduce their original layout after template edits.

**Independent Test**: Create a snapshot, change template layout, and verify the old snapshot remains unchanged while a new evaluation gets the new layout.

- [X] T020 [US3] Ensure all exports load persisted snapshot JSON instead of rebuilding payload from current template when a snapshot already exists in `server/handlers/confirmation.go` and `server/handlers/result_export.go`
- [X] T021 [US3] Add regression tests proving template edits do not alter an existing snapshot checksum/layout and old snapshot JSON still exports in `server/handlers/result_export_test.go`

**Checkpoint**: Historical signoff evidence is reproducible.

---

## Phase 6: User Story 4 - 选择单份导出格式 (Priority: P2)

**Goal**: Users choose Excel or PDF from the evaluation detail while both formats use identical content.

**Independent Test**: Export the same evaluation as xlsx and pdf and compare the configured document fields.

- [X] T022 [US4] Add validated `format=xlsx|pdf` handling and audit metadata to `ExportEvaluationToExcel` in `server/handlers/export.go`
- [X] T023 [P] [US4] Extend `exportApi.evaluation` with the format parameter in `lib/api.ts`
- [X] T024 [US4] Add an Excel/PDF choice and visible progress/success/error feedback to the evaluation detail export action in `app/evaluations/page.tsx`
- [X] T025 [US4] Add format default, invalid-format and PDF response regression tests in `server/handlers/result_export_test.go`

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T026 Run `gofmt` on changed Go files and verify all tasks reflect the contracts in `specs/005-unified-result-exports/contracts/result-export-api.md`
- [X] T027 Run backend `go test ./...` through the backend Docker build and fix regressions in changed `server/` files
- [X] T028 Run frontend TypeScript checking and Next.js production build through the frontend Docker build and fix regressions in `app/` and `lib/`
- [X] T029 Validate `docker compose --env-file .env.example config` and do not remove any existing Docker volume
- [ ] T030 Execute the repeatable manual scenarios in `specs/005-unified-result-exports/quickstart.md` and record any environment-only limitations

## Phase 8: Convergence

- [ ] T031 Complete authenticated browser acceptance for template layout save, single Excel/PDF downloads, mixed-template batch PDF ZIP, and historical-layout re-export per US1/AC1, US2/AC1, US3/AC1, and US4/AC1 (partial)
- [X] T032 Fix Chromium local-file navigation by converting generated HTML and PDF paths to absolute file URLs and disabling print headers in `server/handlers/result_export.go` per US4/AC1 (contradicts)
- [X] T033 Change snapshot lifecycle so `pending_confirm` exports adopt the latest saved template layout while `completed` evaluations retain their completed snapshot in `server/handlers/confirmation.go` per FR-009A (contradicts)
- [X] T034 Add regression tests for Unicode absolute PDF file URLs, pending-confirm layout version refresh, and completed-layout immutability in `server/handlers/result_export_test.go` per US3/AC1 and US4/AC1 (missing)

---

## Dependencies & Execution Order

- Phase 2 depends on Phase 1 and blocks all user stories.
- US1 depends on the normalized layout and snapshot shape from Phase 2.
- US2 may begin after Phase 2 but its UI is validated against the US1 renderer.
- US3 depends on snapshot creation from Phase 2 and the unified builder from US1.
- US4 depends on both shared renderers from US1.
- Polish follows all selected stories.

## Parallel Opportunities

- T005 and T006 can proceed after T004 because they touch separate implementation/test sections.
- T009 can be written while renderers T010-T011 are implemented.
- T016 and T017 are independent after the template contract is fixed.
- T023 can proceed while T022 is implemented.

## Implementation Strategy

1. Complete T001-T014 for the core MVP: one layout definition shared by current Excel and batch PDF.
2. Complete T015-T019 to expose template-level controlled customization.
3. Complete T020-T025 for historical immutability and single PDF selection.
4. Complete T026-T030 before handoff.
