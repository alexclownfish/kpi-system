# MVP acceptance tests

Run from the repository root against the Compose gateway:

```bash
export KPI_BASE_URL=http://localhost
export KPI_TEST_HR_EMAIL=sunba@company.com
export KPI_TEST_HR_PASSWORD='local test password'
npm run acceptance:mvp
```

The password must come from the environment. Never add it to source, reports, screenshots, or command output.
Temporary employees use an `acceptance-<timestamp>-<random>` run ID in their names and email addresses. Every
created record is registered immediately and deleted by exact numeric ID in reverse dependency order.

The suite is intentionally non-destructive: do not reset SQLite, bulk-delete by email prefix, or run
`docker compose down -v`. Sanitized HTML, machine-readable output, and cleanup status files are written below
`.artifacts/` and are ignored by Git.

## Full weighted workflow (opt-in)

`tests/e2e/evaluation-workflow.spec.ts` also contains a real-service scoring/sign-off scenario. It is skipped by
default because it creates two temporary assessments, submits zero-valued scores, exercises objection handling,
uploads a paper-signature fixture, and then deletes the exact numeric IDs it created. Enable it only against a
disposable database:

```bash
export KPI_E2E_WORKFLOW=1
export KPI_E2E_DEPARTMENT_ID=123
export KPI_E2E_MANAGER_ID=456
export KPI_TEST_HR_PASSWORD='local test password'
export KPI_BASE_URL=http://localhost
npm run test:e2e -- tests/e2e/evaluation-workflow.spec.ts --grep '真实评分闭环'
```

`KPI_E2E_TEMPLATE_ID` may pin a published template. Otherwise the suite selects the first published template for the
configured department and period, derives its position, creates a temporary ordinary-RBAC scorer, and adds exact
position-scoped scoring responsibilities for the scorer's item types. `KPI_E2E_YEAR` defaults to `2099` to keep
acceptance data separate from normal reporting. If the required variables are absent, the full scenario reports
skipped with the missing preconditions; it never falls back to an existing employee or assessment.

The online-confirmation scenario also verifies the post-completion paper-signature supplement flow: HR uploads two
separate PNG fixtures, the completed status remains unchanged, the list and protected download routes return both
records, and the employee's upload is rejected without `assessment:approve`. The exact temporary assessments are
deleted in `afterAll`; the suite then verifies their supplement listing is no longer accessible.

Migration dry-run is a separate opt-in because it creates an immutable audit batch. Set `KPI_E2E_MIGRATION=1`,
`KPI_TEST_ADMIN_EMAIL`, `KPI_TEST_ADMIN_PASSWORD`, and an exact `KPI_E2E_MIGRATION_EVALUATION_ID` to run that
read-only probe. The probe compares the evaluation and scoring-task response before and after the dry-run and never
applies the batch.
