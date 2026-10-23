# Workflow guard acceptance — 2026-10-08

## Scope

T039 closes the confirmation and objection transition boundary for legacy and
weighted-v2 assessments. The handlers now share backend guards instead of
trusting a status value supplied by a caller or a legacy shortcut.

## Implemented checks

- Online and paper confirmation use the same state, objection, snapshot, self-
  evaluation, and required scoring-task guard.
- Weighted-v2 confirmation is accepted only from
  `awaiting_employee_confirmation`; legacy confirmation remains limited to
  `pending_confirm`.
- Required zero scores are accepted as completed work; missing and rejected
  tasks return structured blocking details.
- Objection submission is conditional on the current state and uses an
  optimistic database update so duplicate concurrent submissions cannot both
  succeed.
- Weighted-v2 objection handling is limited to `rejected` and returns to
  `awaiting_employee_confirmation`; completed/cancelled and already-handled
  records cannot be moved backward.
- Legacy objection handling remains compatible with `pending_confirm` and
  requires an explicit total score, including zero.

## Automated regression coverage

Added to `server/handlers/evaluation_confirmation_workflow_test.go`:

- required self/task completion and valid zero-score confirmation;
- terminal confirmation rejection;
- legacy/new state separation;
- online and paper weighted-v2 endpoints returning the same structured
  incomplete-task response.

Added to `server/handlers/evaluation_objection_workflow_test.go`:

- legacy objection submission and handling, including an explicit zero total;
- weighted-v2 objection transition and forged-total rejection;
- terminal completed-state rejection for stale objection records.

Validation completed with the existing one-shot backend builder image (the
Compose services were not restarted):

- `go test ./handlers -run "Test(Confirmation|Weighted|LegacyObjection|NextWorkflow|RequiredTask|NotApplicable)" -count=1`: passed.
- `go test ./...`: passed (`handlers`, `models`, and `routes`).
- `tsc --noEmit`: passed with the bundled workspace Node runtime.
- `docker compose config --quiet`: passed.
- `git diff --check`: passed.
- Targeted ESLint passed. The production Next build passed; it reported one
  pre-existing exhaustive-deps warning in `app/templates/page.tsx`.

Docker Compose rebuild and browser acceptance remain user-managed; they are
tracked separately by T089/T099.

## T078/T086 diagnostics increment

- Added protected `GET /api/audit-logs` with bounded pagination, resource/result
  filters, actor display, permission checks, and `ALL` data-scope enforcement.
- Added settings-page “审计诊断” entry for migration, responsibility, and
  evaluation operation records with loading, empty, permission/error, and
  retry states.
- Added workflow audit events for advance, objection submission, objection
  handling, online confirmation, and paper confirmation.
- Canonical result persistence and confirmation evidence now share the same
  transaction for weighted-v2 online and paper confirmation.
