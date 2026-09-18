---
name: kpi-production-update
description: Safely update this KPI System production deployment from Git and rebuild it with Docker Compose. Use for production pull, rebuild, restart, health verification, or deployment troubleshooting; do not use for first-time bootstrap or destructive data reset.
---

# KPI production update

Update an existing production checkout without replacing its `.env`, `secrets/`, SQLite volume, exports, signoff uploads, or backups.

## Preflight

1. Confirm the exact production host and repository directory. Do not assume the current machine is production.
2. Read `docs/production-deployment.md`, `docker-compose.yml`, and `.env.example` when they changed since the deployed revision.
3. Run read-only checks first:

   ```bash
   pwd
   git status --short --branch
   git remote -v
   git fetch --prune origin
   git rev-list --left-right --count HEAD...@{upstream}
   test -f .env
   docker compose config --quiet
   docker compose ps
   ```

4. Stop before deployment when:
   - the checkout has local modifications or untracked files;
   - the branch has diverged from its upstream;
   - `.env` is missing or Compose configuration fails;
   - the requested commit has not been pushed;
   - a schema-affecting release has no recent verified backup.

Never print `.env`, secret-file contents, JWTs, passwords, or authenticated URLs. Report only whether required values/files are present.

## Update

Capture the old revision, then use fast-forward-only Git update and Compose recreation:

```bash
git rev-parse HEAD
git pull --ff-only
docker compose config --quiet
docker compose up -d --build
```

`docker compose up -d --build` is the supported update command. It recreates services when required while retaining the named volumes declared in `docker-compose.yml`.

Do not run `docker compose down -v`, `docker volume rm`, delete `secrets/`, overwrite `.env`, rotate `JWT_SECRET`, or re-enable `BOOTSTRAP_ADMIN_*` during a routine update.

## Verification

Run:

```bash
docker compose ps
docker compose logs --tail=100 backend frontend nginx-gateway
curl -fsS "http://127.0.0.1:${KPI_HTTP_PORT:-8088}/health"
```

Then verify the public login page and one authenticated read request. For export-related releases, additionally export one Excel and one PDF from a `pending_confirm` evaluation.

Success requires all services to remain running, backend to be healthy, `/health` to return success, and no new startup or migration error in logs.

## Failure handling

- Preserve the old and new commit IDs and collect relevant service logs.
- Do not automatically reset Git, restore a database, or remove volumes.
- Explain whether the failure is build-time, startup, health-check, migration, proxy, or application-level.
- A rollback requires explicit approval and a compatible data/schema assessment. Rebuilding an older commit does not automatically reverse database migrations.

Report the deployed commit, service status, health result, and any manual verification still required.
