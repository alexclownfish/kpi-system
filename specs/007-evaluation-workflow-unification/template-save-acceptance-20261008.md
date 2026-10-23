# Template save regression acceptance — 2026-10-08

## Current state and scope

User reported a white client-exception page after Save. Feature 007 had 101/111
tasks checked; T061 and T088 covered export-layout authoring/version semantics.
Requirements checklist passed 16/16. This increment fixes template settings save
feedback and clone/version handling; it does not implement new PDF content fields.

## Scoped analysis

| Requirement | Existing task | Finding and resolution |
|---|---|---|
| FR-016, SC-008 | T004, T061, T088 | Layout save passed structured `error` objects to the React alert; calculation save/publish passed them to toast. Both now use the shared message parser. |
| FR-017, SC-006 | T058, T061, T088 | Backend already rejected published/retired writes, but layout settings remained editable. Fields, ordering and save are now read-only with an explicit clone action. |
| FR-014, FR-022 | T052, T088 | Extended the historical-layout contract through actual reject/clone/update handlers, comparing old snapshot layout/checksum and the newly created snapshot's layout. |

No new requirement ambiguity or spec/plan conflict blocks this repair. The
constitution's backend authorization, feedback, regression, data-safety and
single-source rules remain applicable. No extension hook configuration exists.

## Root cause and changes

Published writes return HTTP 409 `PUBLISHED_TEMPLATE_IMMUTABLE` with an object
containing `code` and `message`. The old expression `detail.message || detail.error`
can return that object; passing it as a React child throws. The old expression
was independently reproduced with React rendering. The regression executes the
actual save handler against Axios structured/legacy errors and verifies a string.

- Layout save uses `getApiErrorMessage`, preserves unsaved input on failure,
  resets submitting state after closing the alert and never reports false success.
- Published/retired settings are read-only. Clone shows a submitting state,
  selects the returned draft, inserts it into the list and loads its items/layout.
- Calculation-editor clone/publish callbacks now synchronize selected layout
  and insert new templates instead of losing the cloned row.
- Layout update responses preload dimensions/items and hydrate calculation data,
  preventing the successful save from clearing the calculation structure editor.
- Layout schema version remains 1; template publication version increments on
  clone. Existing assessments keep their snapshot fields, title and signatures.

## Validation evidence

| Check | Result |
|---|---|
| `node tests/unit/template-save.test.mjs` | PASS: real handler, structured and legacy errors, React rendering, success state, locked-write guard |
| Playwright `template-export-layout.spec.ts` | 4/4 PASS: published/retired clone, preset/column/orientation preview and save; failed save/close/retry; calculation save structured error; zero browser page exceptions |
| Current-source `go test -count=1 ./...` | PASS in a disposable builder container; source streamed with tar rather than VM-mounted files |
| TypeScript `tsc --noEmit` | PASS |
| Next.js production build | PASS |
| Targeted ESLint | 0 errors; existing `fetchTemplates` effect dependency warning remains |

Browser tests used the actual production-built page with mocked HTTP responses on
a temporary loopback server. They did not access the business database. The
backend contract uses disposable in-memory SQLite. Ordinary Compose services,
volumes, credentials and environment configuration were not changed.

## Page acceptance after the user rebuilds

1. Run `docker compose up -d --build` from the local working directory. Local
   development does not require `git pull`. Refresh the browser to load new assets.
2. Templates → view a published template → Result export settings: fields and Save
   are disabled and the new-version instruction is visible.
3. Click “复制新版本后编辑”. Change title/selected columns/signatures and Save.
   Confirm success, switch tabs and return; saved settings remain. The cloned
   draft appears in the template list and its calculation structure is retained.
4. Use Calculation structure to publish the clone when ready. Newly created
   applicable assessments use that published template's layout; existing
   assessments keep the original snapshot rather than adopting the change.
5. Re-export an old finalized assessment and check original title, fields,
   signatures and score remain unchanged.

## Scoped convergence

T061/T088 are complete with UI and backend regressions. No additional gap was
found in this repair scope. Feature 007 is now 103/111 checked; eight open tasks
remain: T059, T083, T089, T096, T097, T099, T104 and T105. They retain their existing
IDs; no duplicate convergence tasks were appended. Real Compose/page acceptance
T083/T089 remains open, since the user controls rebuilding the running services.
