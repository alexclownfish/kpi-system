# Business Repair Acceptance - 2026-10-01

## Scope and safety

This is an incremental repair, not whole-platform sign-off. Existing uncommitted
work was preserved. The ordinary Compose services/database were neither rebuilt
nor reset. Verification used disposable source-workbook fixtures on backend
`http://localhost:18089` and a current-source frontend on
`http://localhost:18092`. The original workbook was not changed.

Spec Kit requirements checklist: 16/16 checked. Analyze/converge/implement were
used to map reproduced defects to tasks. T090-T095, T098, T100-T103 are verified;
T096/T097/T099 and earlier incomplete tasks remain open.

## Verified repairs

| Area | Actual verification |
| --- | --- |
| Self evaluation | Owner only; outsiders denied; frozen max score; zero valid; no legacy manager required for weighted-v2; submitted self scores locked. |
| Comments | Assessment visibility for all four operations; comment ownership and route assessment enforced. |
| HR onboarding | Basic employee/manager grants permitted without granting elevated roles or custom-role privileges. |
| Assigned work | An ordinary-employee PM can discover and score assigned items; unrelated assessments excluded; pending list/count/pagination agree. Completed tasks leave pending work. |
| Creation | Department/position/period mismatch, invalid date fields, forged initial state/total, and injected associations rejected. |
| Calculation | Decimal HALF_UP; 63 x 7.5% = 4.73; multi-scorer average not rounded before weighting; non-finite inputs rejected. |
| Statistics | Frozen canonical item scores used; month-end trends have no duplicate months; completed zero scores included in assessment-list averages. |
| Layout | Automatic and explicit template matching load the same configured layout into the assessment snapshot. Historical-layout regression tests pass. |
| UI | Chinese states; weighted owner self-score fields restored; self completion calls backend advance; other draft inputs survive one task submission. |
| Version labels | Confirmation's result-document version is named signing snapshot, distinct from canonical calculation version. |

## Automated evidence

- Full Go suite: `go test ./...` passed in the isolated Go build container after
  the final backend changes; handlers/models/routes all passed.
- Frontend `tsc --noEmit`: passed.
- Targeted ESLint on changed evaluation components/client/context: passed after
  repairing missing links to already installed local lint plugins.
- Final Next production build: compilation, lint/type checks and all 22 page
  generation passed (exit 0). One existing `fetchTemplates` hook-dependency
  warning remains in `app/templates/page.tsx`. Local plugin-resolution failures
  from the earlier build were repaired without suppressing lint rules.
- `docker compose config --quiet`: passed. Ordinary-service image rebuild was
  intentionally left to the user.
- Isolated backend `/health`: HTTP 200. Production-built frontend login: HTTP 200.
- `git diff --check`: passed.

The workbook runtime suite before the final pending-list/average refinements
passed 344/344 checks across 15 department/position workflows. Three source
profiles were blocked by weight-policy conflicts, not silently normalized.
Supplemental evidence: 367 passed / 384 total, with 17 unresolved comparisons:
15 default signing-label comparisons and 2 HR aggregation comparisons. Ten
additional grade-boundary workflows cover 0, 59, 60, 69, 70, 79, 80, 89, 90,
and 100, including locked submissions and employee confirmation.

These counts are test-case results, not a feature-completion percentage. The
latest pending-list/average/multi-scorer changes have separate Go contract tests;
the source suite was not replayed against its already-populated database.

Source SHA-256:
`c3f8749e6ebc046cff88a3b5d052ee7a81fad4766c2678907e6d0be01b6997c0`.
Runtime evidence and exported files:
`/private/tmp/kpi-business-repair.WWDirc/source-runtime/`.

## Real browser journey

All score entry in this journey used the actual page, not API score filling.
API setup created disposable fixtures only.

1. Engineer opened October 2026 assessment #26; submitting empty self evaluation
   displayed six missing required indicators.
2. Engineer saved six self scores, including a valid zero, then completed self
   evaluation; state became scoring and all self inputs became read-only.
3. PM, whose system role is ordinary employee, opened pending assigned work and
   submitted exactly four PM items. Other scores were not editable. The attempt
   to advance with two required tasks missing was blocked.
4. Department head submitted the one quality item; its pending list became empty.
5. Product manager submitted the final item and advanced. All scorer values were
   80; total was 80.00, grade A, required completion 6/6.
6. Engineer confirmed online. Assessment became completed, pending badge cleared,
   total stayed 80, and no score could be edited.

Screenshot: `/private/tmp/kpi-business-repair.WWDirc/ui-workflow-completed.png`.

The browser-completed assessment was re-read through the backend after building:
#26 is completed with total 80. A new disposable November 2026 assessment #27 is
prepared in self_evaluating for the same engineer, with all template and
responsibility setup already present. The production-built verification frontend
remains available at `http://localhost:18092/evaluations`, bound only to localhost.
Prepared test accounts (isolated environment only):

| Participant | Login email | Assigned work |
| --- | --- | --- |
| Employee | source-engineer-0@example.test | Six self-evaluation items and final confirmation |
| PM | source-pm-0@example.test | Four PM items |
| Department head | source-head-0@example.test | One quality item |
| Product manager | source-product-0@example.test | One product-consistency item |

Shared synthetic fixture password: `Synthetic-Isolated-Test-20261001!`. These are
disposable test credentials, not real administrator credentials. The ordinary
deployment and its accounts are unchanged. Enter 80 for all formal scorer items;
the expected final result is 80.00/A, regardless of a valid zero self score.

All 15 source-workflow PDFs had portrait page dimensions and corresponding
single/batch content. A rendered representative PDF was visually inspected:
no clipped item labels or overlapping table text. Default signature labels are
still generic and are not proof of source-scorer signature parity.

## Remaining work and policy decision

- T096: transactional removal and dependent-row/attachment cleanup.
- T097: audit invalid periods, forged terminal totals, missing canonical finals,
  and orphan workflow records before approving migration.
- T099: repeatable full browser/contract regression, including objections/paper
  confirmation/migration, not just the existing two E2E requests.
- Earlier tasks: diagnostics, export errors/version presentation, next-version
  template-export editing, and legacy/weighted objection/rejection coverage.
- T104/T105: default signing roles and selected export fields need source-workbook parity
  work; configured custom layouts must retain precedence and historical snapshots.
- Feature 006 T037-T042: project effort threshold, project contribution and
  monthly/quarterly/yearly aggregation remain unimplemented.

Three department-head profiles conflict with the current local-weight policy:
structure 120/80, electronics 120/70, software 73/140 within individual
dimensions, although each profile's combined item weights totals 100. Select
either source composite-weight validation or correct the source local weights;
do not silently normalize or rewrite signed historical calculations.

## User verification after local rebuild

1. Keep existing `.env` and volumes. Run `docker compose up -d --build`; no
   `git pull` is needed for changes already in this local checkout.
2. Use the configured gateway port, not a hard-coded test port. Existing
   weighted assessments use the new self-score UI without recreation. New
   weighted assessment creation requires `EVALUATION_WORKFLOW_MODE=enabled`;
   `audit` intentionally does not enable new workflow writes.
3. Employee opens own assessment, verifies missing-required feedback and saves
   zero, then completes self evaluation.
4. Each actual scorer opens pending assigned work; edit only own items, fill
   several scores, submit one, and verify the other entered values remain.
5. Leave one required item blank: advancing must fail. Finish it: confirmation
   becomes available, total equals the weighted source result.
6. Employee confirms; state and badge refresh, scores lock, and final detail,
   Excel and PDFs retain the same total. Repeat on an assessment with customized
   export layout to check its assessment-time fields.

No production migration or automatic repair of historical business rows has
been performed by these fixes.
