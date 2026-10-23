# Local Runtime Acceptance - 2026-09-30

## Release Decision

**NOT ACCEPTED.** Local Docker, HTTP APIs and browser operations were exercised with real disposable records. Build success does not establish business correctness. The repeatable runtime check completed 47 assertions: 42 passed, 5 failed. Additional data-integrity and UI findings are recorded separately; these counts are not a whole-product completion percentage.

Environment: local Colima Compose only, gateway `http://localhost:8088`, demo data enabled, workflow mode enabled. No production data or Git changes were reset. Previously authorized local reset removed the old database and local KPI data volumes. Demo accounts were then recreated for testing.

## Verified Business Flows

| Scenario | Expected / Observed | Result |
| --- | --- | --- |
| Authentication | Five seeded users can log in; wrong password 401; unauthenticated data 401 | Pass |
| Data scope | Ordinary employee employee-list contains only self; company report 403; unrelated assessment detail 404; unrelated score list 403 | Pass |
| Position matching | Published monthly technical template resolves for matching employee; DIRECT_MANAGER resolves to Zhang San | Pass |
| Required work | Missing required tasks block progression 422; valid zero self-score advances to scoring | Pass |
| Single scorer | Manager submits 80; result 80, grade A; employee online confirmation completes assessment | Pass |
| Multiple scorers | Two PMs produce four assignments. A scores 0/100 -> 50; B scores 60/60 -> 60; 60%/40% total = 54, grade D | Pass |
| Scoring denial | Unassigned scorer 403; score above 100 400; empty or required not-applicable submission 422 | Pass |
| Responsibility snapshot | Deactivation affects new resolution, but existing assignments remain | Pass |
| Objection | Employee objection blocks confirmation; HR handling returns to awaiting confirmation; employee can then confirm | Pass |
| Paper signoff | Exported result can be registered with synthetic PDF evidence; assessment completes; employee downloads identical attachment; unrelated employee 403 | Pass |
| Published template | Direct edit returns 409 PUBLISHED_TEMPLATE_IMMUTABLE | Pass |
| Single export | PDF and Excel generated and downloaded; URL retains :8088; correct content types; both include total 80 and signature fields | Pass |
| PDF paper size | First page 594.95996 x 841.91998 points; portrait; rendered page visually inspected for text/table/signature placement | Pass |
| Batch export | ZIP includes PDF and signoff-summary Excel; complete extracted PDF text matches single export | Pass |
| Import boundary | Completed assessment excluded from final-score import template | Pass |
| Audit repeatability | Repeated audit finding payloads match | Pass, but finding completeness fails below |
| Migration permission | HR without system:edit gets 403 | Correct behavior; existing E2E incorrectly expects 200/422 |

PDF text checking normalizes Unicode compatibility radicals. A first raw-text assertion failed because the PDF extracted `员⼯` instead of `员工`; this was a test normalization issue, not a missing signature field. Similarly, 404 is accepted as safe out-of-scope detail denial, rather than insisting on 403.

## Reproduced Failures

| ID | Severity | Evidence | Required Fix |
| --- | --- | --- | --- |
| R01 | Critical | Qian Qi can PUT another employee's self score; 200 and persisted value. Restored by owner before subsequent workflow test | Owner authorization plus score/state validation on legacy self endpoint |
| R02 | Critical | Qian Qi GET another employee's comments returns 200; public comments are not constrained to assessment visibility | Scope all comment operations to accessible assessment |
| R03 | High | HR POST employee with built-in employee or department_manager role returns 403: 无权授予权限：提交考核 | Explicit safe built-in-role delegation policy, not indiscriminately adding elevated rights |
| R04 | High | Wang Wu sees two PM scoring tasks for assessment 2, but assessment list only contains his own unrelated assessment 3 | Unify assignment-based visibility in list/detail/pending APIs |
| R05 | High | Creation accepted period=not-valid, year=0, month=99; accepted a position-mismatched template; accepted status=completed and total_score=999 | Validate every creation path, use server-owned initial status/total |
| R06 | High | Employee KPI breakdown reports 0 for weighted-v2 items whose final values are 80, 50 and 60. Trends repeat 2026-03, omit February and show 0 for completed monthly score 80 | Canonical item statistics and correct date/grouping logic |
| R07 | High | After deleting paper-signoff assessment 6: orphan task=1, calculation rows=2, result snapshot=1, confirmation=1, and stored PDF still exists | Transactional relational cleanup and post-commit attachment cleanup |
| R08 | High | Audit marks all five assessments eligible, with zero errors, including invalid period and forged completed score without final canonical result | Expand audit checks for illegal state/data and orphan dependencies |
| R09 | Medium | Browser list shows self_evaluating as 未知状态; task card displays submitted in English | Shared canonical state labels and filters |
| R10 | High | T079 marked complete, but workflow E2E file contains only two audit/migration tests. MVP E2E fails first HR onboarding test and skips downstream scenarios | Real end-to-end coverage and correct authorization fixtures |

Dashboard average 377.6667 was polluted by the forged 999 record: (80 + 54 + 999) / 3. This is evidence of R05, not evidence that averaging itself is incorrect. Department report defaults to monthly/current-period filters, so a default report average of 80 versus an unfiltered dashboard average is not, by itself, a parity failure. Compare identical populations and periods when fixing R06.

Partial task submissions also persisted an interim canonical total in the earlier multi-scorer test. Workflow still correctly blocked confirmation. Since T048 intentionally recalculates after submissions, this is not automatically a calculation defect; distinguish preliminary and finalized availability clearly in the UI/export contract.

## Build and Browser Evidence

- Docker backend build executed `go test ./...`: handlers, models and routes passed.
- Frontend production build and type checking passed. Existing templates useEffect dependency warning remains.
- Backend healthy, frontend/gateway running; gateway health 200.
- Browser login, dashboard, all-assessments list, completed detail and PDF export controls exercised. Detail shows total 80, one required submitted task, online confirmation and disabled score editing.
- Detail displayed calculation V3 and signoff snapshot V1. These are different version counters, not necessarily different scores; labels must make the distinction clear (existing T087).
- Browser user-controlled tab was left alone after navigation changed; additional checks used an independent temporary tab.

## Limits and Remaining Gates

This is not an all-features-pass claim. Role CRUD/reassignment under a super-admin, successful import preview/commit, migration apply/rollback, full original Hongxing workbook golden-fixture parity, all-template selected-column combinations, backup restoration, SSE delivery, concurrency under load, and every page's full CRUD remain unverified in this live environment. The fresh demo reset currently has no bootstrap administrator configuration or super-admin employee. No existing employee was promoted solely to make tests pass.

Role/employee onboarding failures block the normal management acceptance suite. Backend unit coverage does not replace these remaining live checks. T090-T099 capture runtime convergence work without rewriting earlier completion markers.

## Retest Procedure

1. Rebuild local services after fixes. Do not pull Git merely to test local edits, and do not remove volumes during normal updates.
2. Create a disposable department, manager and two employees through the UI. HR onboarding must succeed without allowing elevated-role assignment.
3. Publish a matching template with two required indicators weighted 60%/40%, and two explicitly assigned PMs. Use scores 0/100 for indicator A and 60/60 for B.
4. Both assigned PMs must discover the assessment from their normal list. An unrelated employee must get 403/404 for detail/comments and cannot update any self/scorer values.
5. Leave one required task blank and attempt progression: expect an actionable rejection. Submit every task: total must be 54; confirm online on one fixture and via synthetic paper evidence on another.
6. Compare same-period detail/statistics/Excel/single PDF/batch PDF for total 54 and expected selected fields. PDF must be portrait. Editing/retiring/cloning current templates must not rewrite existing snapshots.
7. Attempt invalid periods and forged completion/total payloads; expect 4xx and zero new records. Run audit twice; expect deterministic findings and zero business writes.
8. Delete disposable records, verify zero orphan tasks/results/confirmations/comments/files, and rerun protected API/E2E regression checks.

## Verified Cleanup

Only the explicitly identified local verification records were removed: assessments 1-5, the already-deleted paper fixture 6's dependent rows, templates 4/5, inactive verification responsibilities 1/2, and the paper fixture's single stored PDF. Backend was stopped before copying/cleaning the SQLite file, then restarted. No production resources were used. This one-time cleanup follows the user's explicit permission to clear local dirty/old data; it is not a deployment procedure or recommendation to reset volumes during updates.

Post-cleanup checks:

- Live dashboard: employees 7, departments 4, evaluations 0, average 0.
- Live template list: only seeded templates 1/2/3 remain.
- Live responsibility list: empty.
- SQLite tasks/results/result snapshots/confirmations/dimensions: all zero; integrity check `ok`; foreign-key check returned no violations.
- Stored signoff attachment directory: empty. Export directory was observed empty during the cleanup inspection.
- Backend healthy; frontend/gateway running; `/health` 200; demo HR authentication still succeeds.

The pre-cleanup local test database is retained at `/private/tmp/kpi-local-before-cleanup.db` for recovery/evidence. Original export verification files remain outside the application in `/private/tmp/`. Temporary browser verification tab is closed; user's tabs are preserved.

Raw assertion evidence: `runtime-check-results-20260930.json` (47 assertions, no passwords or tokens). Browser detail screenshot: `/private/tmp/kpi-ui-evidence.png` (taken before fixture cleanup).
