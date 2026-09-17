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
