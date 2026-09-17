# Tasks: 绩效结果打印签字与归档

**Input**: Design documents from `specs/002-result-signoff/`

## Phase 1: Setup

- [x] T001 Verify existing ignore files and add protected sign-off upload paths to `.gitignore` and `.dockerignore`
- [x] T002 Add persistent sign-off upload volume and environment path in `docker-compose.yml`

## Phase 2: Foundational

- [x] T003 Add result snapshot and confirmation entities with constraints in `server/models/models.go`: one confirmation per evaluation, snapshot version/checksum unique per evaluation, remark maximum 500 characters
- [x] T004 Register backward-compatible AutoMigrate entries in `server/models/database.go`
- [x] T005 Implement canonical result snapshot, checksum, version reuse and transactional finalization helpers in `server/handlers/confirmation.go`

## Phase 3: User Story 1 - 导出最终评分表 (P1)

**Independent Test**: Export a pending-confirm evaluation and verify the XLSX contains final values, version/checksum, print settings and signature areas; an out-of-scope user is denied.

- [x] T006 [US1] Add export permission/data-scope/status regression tests in `server/handlers/confirmation_test.go`
- [x] T007 [US1] Enforce evaluation data scope and final-state eligibility, create/reuse result snapshots, and return version metadata in `server/handlers/export.go`
- [x] T008 [US1] Convert the individual workbook to an A4 print-friendly final result form with final comments, version/checksum and employee/manager/HR signature areas in `server/handlers/export.go`
- [x] T009 [US1] Add export response fields and detail-page download action with loading/success/error feedback in `lib/api.ts` and `app/evaluations/page.tsx`

## Phase 4: User Story 2 - 回录纸质签字 (P1)

**Independent Test**: Upload a valid signed PDF for the current exported version and verify the evaluation completes, confirmation metadata is visible and the protected attachment downloads; invalid/stale/unauthorized requests do not mutate data.

- [x] T010 [US2] Add paper confirmation, stale version, invalid file, duplicate and protected download tests in `server/handlers/confirmation_test.go`
- [x] T011 [US2] Implement paper confirmation multipart validation, protected storage, transaction, audit and attachment download in `server/handlers/confirmation.go`
- [x] T012 [US2] Register confirmation query/paper confirmation/attachment routes with permissions in `server/routes/routes.go`
- [x] T013 [US2] Add confirmation types and multipart API methods in `lib/api.ts`
- [x] T014 [US2] Add paper sign-off dialog, file/date validation, confirmation summary and attachment download UI in `app/evaluations/page.tsx`

## Phase 5: User Story 3 - 保留在线确认凭证 (P2)

**Independent Test**: The employee confirms their own pending result and receives an online confirmation record; another user, a duplicate request, an unresolved objection, and completed-result edits are rejected.

- [x] T015 [US3] Add online confirmation identity/state/duplicate/objection and completed-lock regression tests in `server/handlers/confirmation_test.go`
- [x] T016 [US3] Implement dedicated online confirmation endpoint and completed evaluation lock guards in `server/handlers/confirmation.go`, `server/handlers/kpi.go`, and score update handlers
- [x] T017 [US3] Register online confirmation route in `server/routes/routes.go` and replace generic completion update with the dedicated API in `lib/api.ts` and `app/evaluations/page.tsx`

## Phase 6: Polish & Validation

- [x] T018 Run `gofmt` and `cd server && go test ./...`
- [x] T019 Run `npx tsc --noEmit` and `npm run build`
- [x] T020 Run `docker compose config`, rebuild/start containers, check `/health`, and execute `specs/002-result-signoff/quickstart.md`

## Dependencies & Execution Order

- Phase 2 depends on Phase 1 and blocks all user stories.
- US1 establishes result versions required by paper confirmation.
- US2 depends on US1 snapshot/export behavior.
- US3 reuses the same snapshot/finalization helpers and can follow US1 independently of the paper UI.
- Validation follows all selected stories.

## Implementation Strategy

Deliver US1 and US2 as the paper-signature MVP, then add US3 so online confirmation receives the same audit quality and locking behavior.
