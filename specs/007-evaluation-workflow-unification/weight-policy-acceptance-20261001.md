# Weight Policy A Acceptance (2026-10-01)

## Scope

The user accepted A: preserve original inputs and validate the sum of
individually HALF_UP-rounded composite weights. No normalization or historical
snapshot rewrite. Internal item and dimension totals are informational only;
individual weights remain finite values in 0..100 with nonempty matching groups.

Source: `/Users/simon/Downloads/荭杏研发项目绩效考核表.xlsx`.
SHA256: `c3f8749e6ebc046cff88a3b5d052ee7a81fad4766c2678907e6d0be01b6997c0`.
Existing source manifest unchanged. Self-contained Go fixtures in
`server/handlers/testdata/source-head-weights.json` derive from that manifest,
so Docker build tests need no files outside the server build context.

## Spec Kit

- Clarify recorded one accepted answer in features 006 and 007.
- Plan/data model/contracts/tasks aligned with FR-023; both quality checklists remain 16/16.
- Analyze: no critical/high conflict in this increment; coverage through T106–T110.
- Implement: regressions reproduced three original HTTP 400 failures before repair.
- Converge identified the misleading weighted-template 1000-point header; T111 fixed it with legacy fallback and a regression test.
- This increment does not mark earlier open work or the whole platform complete.

## Functional acceptance

Isolated backend: `kpi-weight-policy-20261001`, `http://localhost:18089`.
Isolated frontend: `http://localhost:18092`, final detached process 88924.
No ordinary Compose database/volume was reset or rebuilt. Previous isolated
backend `kpi-source-repair-20261001` was stopped with its data retained.

All 18 source profiles (9 departments × 2 positions) completed template
save/publish, automatic matching, owner self evaluation including zero, exact
template-driven scoring assignments, required-task blocking, calculation and
grade, confirmation, exports, statistics, and frozen historical results after
template cloning.

| Previously blocked head profile | Internal totals retained | Expected / actual | Grade |
| --- | --- | --- | --- |
| Structure | 120%, 80% | 24.82 / 24.82 | D |
| Electronics | 120%, 70% | 51.07 / 51.07 | D |
| Application software | 73%, 140% | 33.71 / 33.71 | D |

Scores are source numeric values where provided, otherwise explicitly synthetic
deterministic inputs. These are not actual employee performance outcomes.

Evidence: `/private/tmp/kpi-business-repair.WWDirc/weight-policy-runtime/`.

- Original `results.json`: 412/413 passed, 18 workflows, zero source-weight blockers.
- One PDF extraction failure mapped “齐” to the embedded-font glyph “⻬”; visual rendering confirmed no missing content or incorrect orientation.
- `export-recheck.json`: all 18 existing PDFs rechecked with the explicit mapping, 18/18 passed. No export regenerated; original evidence retained.
- `final-results.json`: combined final evidence 413/413 passed. This is runtime plus export recheck, not a second full runtime run.
- All 18 single/batch PDFs have identical normalized full text, portrait pages, complete item labels, totals and employee signing fields.
- The three newly accepted head PDFs were visually rendered and checked for clipping/overlap.
- Template-driven signing defaults and richer export columns remain separate T104/T105 work; parity does not mean those features are finished.

## Browser acceptance

Synthetic administrator on localhost:18092:

1. Opened `02-结构部门/部门负责人维护新版本` → calculation structure.
2. Composite total 100.0%, internal totals 120.0%/80.0% neutral; save succeeded.
3. Changed first item 70 → 60: composite total 98.5%, publish disabled, save showed the specific error.
4. Restored 70 and saved. No invalid value persisted.
5. Final build verification: KPI item header displays 100 points rather than 1000; calculation structure still shows 100.0% and enables publishing.

Screenshot: `template-valid-ui.png` in the evidence directory. Browser test tab
remains on localhost:18092/templates.

## Quality gates

- Full `go test ./...`: PASS in isolated Go builder.
- `node tests/unit/template-weights.test.mjs`: PASS for 18 profiles, decimal rounding, range/invalid-total boundaries, weighted/legacy maximum display.
- TypeScript `tsc --noEmit`: PASS.
- Targeted ESLint: no errors; existing `fetchTemplates` dependency warning remains in `app/templates/page.tsx`.
- Final Next production build: PASS, same existing warning only.
- `git diff --check`: PASS.

## Manual testing

The isolated environment already contains source templates, organization,
responsibilities and synthetic accounts. No field-by-field configuration needed.
Browser is logged in; if needed, use `source-admin@example.test` with synthetic
password `Synthetic-Isolated-Test-20261001!` on localhost:18092 only. These accounts
do not exist in or change the ordinary deployment.

1. Open structure head maintenance draft → calculation structure. Original internal totals 120%/80% must save successfully.
2. Temporarily change first item 70 → 60. Composite 98.5% must disable publish and produce a save error; restore 70.
3. Repeat with electronics/software head drafts, preserving their original weights instead of normalizing them.
4. Assessments: inspect completed structure/electronics/software head fixtures, totals 24.82/D, 51.07/D, 33.71/D. Compare Excel, detail PDF and batch PDF.
5. Update ordinary local services with `docker compose up -d --build` from the current checkout. No `git pull` or database reset is needed for local uncommitted development. Completed history retains frozen inputs/results.

## Broader remaining work

Feature 007 still has T039, T059, T061, T078, T083, T085–T089, T096, T097,
T099, T104 and T105 open. Feature 006 project-effort and period aggregation
remains unimplemented. This report is not a whole-platform production sign-off.
