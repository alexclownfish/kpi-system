#!/usr/bin/env bash
# Disposable end-to-end smoke test. It never attaches to an existing Compose project.
set -Eeuo pipefail
umask 077
root=$(cd -- "$(dirname -- "$0")/.." && pwd)
command -v docker >/dev/null || { echo 'docker is required' >&2; exit 1; }
command -v python3 >/dev/null || { echo 'python3 is required' >&2; exit 1; }
docker info >/dev/null || { echo 'Docker daemon is not accessible' >&2; exit 1; }

id="$(date -u +%Y%m%d%H%M%S)-$$-$(python3 -c 'import secrets; print(secrets.token_hex(3))')"
image="kpi-pg-migration-smoke:$id"
network="kpi-pg-migration-smoke-$id"
volume="kpi-pg-migration-smoke-$id"
pg="kpi-pg-migration-postgres-$id"
backend="kpi-pg-migration-backend-$id"
# Some VM-backed Docker daemons (for example Colima) do not share host /tmp.
# Point KPI_TEST_SCRATCH_PARENT at an already shared, writable host directory.
scratch_parent="${KPI_TEST_SCRATCH_PARENT:-/tmp}"
test -d "$scratch_parent" || { echo "Scratch parent does not exist: $scratch_parent" >&2; exit 1; }
scratch=$(mktemp -d "$scratch_parent/kpi-pg-migration-smoke.XXXXXX")
image_created=0
network_created=0
volume_created=0
pg_created=0
backend_created=0

cleanup() {
    status=$?
    trap - EXIT
    if [ "$status" -ne 0 ]; then
        echo 'Disposable smoke test failed; container logs follow.' >&2
        if [ -f "$scratch/retry.log" ]; then tail -n 30 "$scratch/retry.log" >&2; fi
        if [ "$pg_created" -eq 1 ]; then docker logs --tail 80 "$pg" >&2 || true; fi
        if [ "$backend_created" -eq 1 ]; then docker logs --tail 80 "$backend" >&2 || true; fi
    fi
    if [ "$backend_created" -eq 1 ]; then docker rm -f "$backend" >/dev/null 2>&1 || true; fi
    if [ "$pg_created" -eq 1 ]; then docker rm -f "$pg" >/dev/null 2>&1 || true; fi
    if [ "$volume_created" -eq 1 ]; then docker volume rm "$volume" >/dev/null 2>&1 || true; fi
    if [ "$network_created" -eq 1 ]; then docker network rm "$network" >/dev/null 2>&1 || true; fi
    if [ "$image_created" -eq 1 ]; then docker image rm "$image" >/dev/null 2>&1 || true; fi
    if [ -d "$scratch" ]; then rm -r -- "$scratch"; fi
    exit "$status"
}
trap cleanup EXIT

# Docker create commands may reuse a name. Abort on collision so cleanup only
# touches resources this invocation actually created.
if docker image inspect "$image" >/dev/null 2>&1 ||
   docker network inspect "$network" >/dev/null 2>&1 ||
   docker volume inspect "$volume" >/dev/null 2>&1 ||
   docker container inspect "$pg" >/dev/null 2>&1 ||
   docker container inspect "$backend" >/dev/null 2>&1; then
    echo 'Unexpected disposable-resource name collision' >&2
    exit 1
fi

mkdir -p "$scratch/secrets" "$scratch/admin-secrets" "$scratch/bootstrap-secrets" "$scratch/sqlite" "$scratch/snapshot"
python3 - "$scratch/secrets" "$scratch/admin-secrets" "$scratch/bootstrap-secrets" <<'PY'
import pathlib, secrets, sys
app_secrets = pathlib.Path(sys.argv[1])
admin_secrets = pathlib.Path(sys.argv[2])
bootstrap_secrets = pathlib.Path(sys.argv[3])
(app_secrets / 'postgres_password').write_text(secrets.token_hex(32) + '\n')
(admin_secrets / 'postgres_admin_password').write_text(secrets.token_hex(32) + '\n')
(bootstrap_secrets / 'bootstrap_admin_password').write_text(secrets.token_urlsafe(32) + '\n')
PY
jwt=$(python3 -c 'import secrets; print(secrets.token_urlsafe(48))')

echo '[1/10] Build isolated backend image; Dockerfile runs Go tests'
docker build -f "$root/Dockerfile.backend" -t "$image" "$root"
image_created=1

echo '[2/10] Generate synthetic SQLite data via kpi-server init-business'
docker run --rm -v "$scratch/sqlite:/app/db" -v "$scratch/bootstrap-secrets:/run/kpi-bootstrap:ro" \
    -e APP_ENV=production -e DB_DRIVER=sqlite -e JWT_SECRET="$jwt" \
    -e CORS_ALLOWED_ORIGINS=http://localhost -e SEED_HONGXING_CATALOG=true \
    -e BOOTSTRAP_ADMIN_EMAIL=smoke-admin@example.invalid \
    -e BOOTSTRAP_ADMIN_NAME=smoke-admin \
    -e BOOTSTRAP_ADMIN_PASSWORD_FILE=/run/kpi-bootstrap/bootstrap_admin_password \
    "$image" /app/kpi-server init-business
test -s "$scratch/sqlite/kpi.db"

# Exercise historical evaluation, score, confirmation, and supplemental signoff
# rows in addition to the initialization catalog (which has no evaluations).
mkdir -p "$scratch/signoffs-source" "$scratch/signoffs-target"
python3 - "$scratch/sqlite/kpi.db" "$scratch/signoffs-source" <<'PY'
import base64, hashlib, pathlib, sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
conn.execute('PRAGMA foreign_keys=ON')
employee_id = conn.execute('SELECT id FROM employees LIMIT 1').fetchone()[0]
template_id = conn.execute('SELECT id FROM kpi_templates LIMIT 1').fetchone()[0]
item_id = conn.execute('SELECT id FROM kpi_items WHERE template_id=? LIMIT 1', (template_id,)).fetchone()[0]
evaluation_id = conn.execute('INSERT INTO kpi_evaluations (employee_id,template_id,period,year,month,status,total_score) VALUES (?, ?, ?, ?, ?, ?, ?)', (employee_id, template_id, 'monthly', 2026, 10, 'completed', 88.5)).lastrowid
conn.execute('INSERT INTO kpi_scores (evaluation_id,item_id,self_score,manager_score,final_score) VALUES (?, ?, ?, ?, ?)', (evaluation_id, item_id, 80.0, 90.0, 88.5))
snapshot = '{"source":"migration-smoke","total_score":88.5}'
checksum = hashlib.sha256(snapshot.encode()).hexdigest()
snapshot_id = conn.execute('INSERT INTO evaluation_result_snapshots (evaluation_id,version,snapshot_json,checksum,created_by) VALUES (?, ?, ?, ?, ?)', (evaluation_id, 1, snapshot, checksum, employee_id)).lastrowid
conn.execute('INSERT INTO evaluation_confirmations (evaluation_id,snapshot_id,method,confirmed_by,confirmed_at,attachment_stored_name,attachment_original_name,attachment_content_type) VALUES (?, ?, ?, ?, ?, ?, ?, ?)', (evaluation_id, snapshot_id, 'paper', employee_id, '2026-10-23 00:00:00', 'smoke-signoff.png', 'signed.png', 'image/png'))
conn.execute('INSERT INTO evaluation_signoff_supplements (evaluation_id,uploaded_by,uploaded_at,attachment_stored_name,attachment_original_name,attachment_content_type) VALUES (?, ?, ?, ?, ?, ?)', (evaluation_id, employee_id, '2026-10-23 00:00:00', 'smoke-supplement.png', 'supplement.png', 'image/png'))
conn.commit()
conn.close()
png = base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+X1ioAAAAASUVORK5CYII=')
for name in ('smoke-signoff.png', 'smoke-supplement.png'):
    pathlib.Path(sys.argv[2], name).write_bytes(png)
PY

# Python's backup API creates a consistent independent single-file snapshot.
python3 - "$scratch/sqlite/kpi.db" "$scratch/snapshot/kpi.sqlite.snapshot" <<'PY'
import pathlib, sqlite3, sys
source = sqlite3.connect(pathlib.Path(sys.argv[1]).resolve().as_uri() + '?mode=ro', uri=True)
destination = sqlite3.connect(sys.argv[2])
source.backup(destination)
if destination.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
    raise SystemExit('SQLite integrity_check failed')
if destination.execute('PRAGMA foreign_key_check').fetchone() is not None:
    raise SystemExit('SQLite foreign_key_check failed')
destination.close()
source.close()
PY
sha=$(python3 - "$scratch/snapshot/kpi.sqlite.snapshot" <<'PY'
import hashlib, pathlib, sys
print(hashlib.sha256(pathlib.Path(sys.argv[1]).read_bytes()).hexdigest())
PY
)

echo '[3/10] Start unique network and PostgreSQL volume'
docker network create "$network" >/dev/null
network_created=1
docker volume create "$volume" >/dev/null
volume_created=1
docker run -d --name "$pg" --network "$network" --network-alias postgres \
    -v "$volume:/var/lib/postgresql/data" \
    -v "$scratch/admin-secrets:/run/kpi-admin-secrets:ro" \
    -v "$scratch/secrets:/run/kpi-app-secrets:ro" \
    -e POSTGRES_DB=kpi_smoke -e POSTGRES_USER=kpi_admin \
    -e POSTGRES_PASSWORD_FILE=/run/kpi-admin-secrets/postgres_admin_password \
    -e KPI_APP_USER=kpi_app \
    postgres:16-alpine >/dev/null
pg_created=1
ready=0
for attempt in $(seq 1 30); do
    if docker exec "$pg" pg_isready -U kpi_admin -d kpi_smoke >/dev/null 2>&1; then ready=1; break; fi
    sleep 2
done
if [ "$ready" -ne 1 ]; then echo 'PostgreSQL did not become ready' >&2; exit 1; fi

echo '[4/10] Provision a restricted application role with the production script'
docker exec -i "$pg" sh -s < "$root/scripts/provision-postgres-role.sh"
app_is_privileged=$(docker exec "$pg" psql -U kpi_admin -d kpi_smoke -Atc "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls FROM pg_roles WHERE rolname='kpi_app'")
if [ "$app_is_privileged" != f ]; then echo 'kpi_app is missing or has administrative privileges' >&2; exit 1; fi

run_app() {
    docker run --rm --network "$network" -v "$scratch/secrets:/run/kpi-secrets:ro" \
        -e DB_DRIVER=postgres -e PGHOST=postgres -e PGPORT=5432 \
        -e PGUSER=kpi_app -e PGDATABASE=kpi_smoke \
        -e PGPASSWORD_FILE=/run/kpi-secrets/postgres_password "$@"
}

run_admin() {
    docker run --rm --network "$network" -v "$scratch/admin-secrets:/run/kpi-admin-secrets:ro" \
        -e DB_DRIVER=postgres -e PGHOST=postgres -e PGPORT=5432 \
        -e PGUSER=kpi_admin -e PGDATABASE=kpi_smoke \
        -e PGPASSWORD_FILE=/run/kpi-admin-secrets/postgres_admin_password "$@"
}

echo '[5/10] Initialize schema as kpi_app without seeding business data'
run_app "$image" /app/kpi-server init-schema

echo '[6/10] Migrate once as kpi_admin and compare every copied table count'
run_admin -v "$scratch/snapshot:/migration-source:ro" "$image" \
    /app/kpi-migrate-sqlite-to-postgres \
    --source /migration-source/kpi.sqlite.snapshot --source-sha256 "$sha" \
    --expect-database kpi_smoke --confirm-empty-target | tee "$scratch/migration.log"
grep -q '^MIGRATION_RESULT=committed$' "$scratch/migration.log"
python3 - "$scratch/migration.log" > "$scratch/counts.tsv" <<'PY'
import json, pathlib, sys
lines = pathlib.Path(sys.argv[1]).read_text().splitlines()
reports = [json.loads(x.split('=', 1)[1]) for x in lines if x.startswith('MIGRATION_REPORT_JSON=')]
if len(reports) != 1 or reports[0]['status'] != 'committed':
    raise SystemExit('Missing committed migration report')
report = reports[0]
if any(x != 'passed' for x in report['checks'].values()):
    raise SystemExit('Migration check failed')
counts = report['table_row_counts']
for required in ('employees', 'roles', 'permissions', 'kpi_templates', 'kpi_evaluations', 'kpi_scores', 'evaluation_result_snapshots', 'evaluation_confirmations', 'evaluation_signoff_supplements'):
    if counts.get(required, 0) < 1:
        raise SystemExit(f'Synthetic data missing {required}')
for table, count in sorted(counts.items()):
    print(f'{table}\t{count}')
PY
while IFS=$'\t' read -r table expected; do
    if [[ ! "$table" =~ ^[a-z][a-z0-9_]*$ ]]; then echo "Unsafe table name: $table" >&2; exit 1; fi
    actual=$(docker exec "$pg" psql -U kpi_admin -d kpi_smoke -Atc "SELECT count(*) FROM public.\"$table\"")
    if [ "$actual" != "$expected" ]; then
        echo "Row count mismatch: $table expected $expected got $actual" >&2
        exit 1
    fi
done < "$scratch/counts.tsv"

echo '[7/10] Verify nonempty-target protection does not change data'
before=$(docker exec "$pg" psql -U kpi_admin -d kpi_smoke -Atc 'SELECT count(*) FROM employees')
if run_admin -v "$scratch/snapshot:/migration-source:ro" "$image" \
    /app/kpi-migrate-sqlite-to-postgres \
    --source /migration-source/kpi.sqlite.snapshot --source-sha256 "$sha" \
    --expect-database kpi_smoke --confirm-empty-target > "$scratch/retry.log" 2>&1; then
    echo 'Second migration unexpectedly succeeded' >&2
    exit 1
fi
grep -q '^MIGRATION_RESULT=failed$' "$scratch/retry.log"
after=$(docker exec "$pg" psql -U kpi_admin -d kpi_smoke -Atc 'SELECT count(*) FROM employees')
test "$before" = "$after"

echo '[8/10] Copy and verify historical paper-signature attachments'
docker run --rm -v "$scratch/signoffs-source:/migration-source:ro" \
    -v "$scratch/signoffs-target:/app/uploads/evaluation-confirmations" \
    "$image" sh -ec 'test -z "$(find /app/uploads/evaluation-confirmations -mindepth 1 -print -quit)"; cp -a /migration-source/. /app/uploads/evaluation-confirmations/'
python3 - "$scratch/signoffs-source" "$scratch/signoffs-target" <<'PY'
import hashlib, pathlib, sys
source, target = (pathlib.Path(x) for x in sys.argv[1:])
source_files = sorted(p.relative_to(source) for p in source.rglob('*') if p.is_file())
target_files = sorted(p.relative_to(target) for p in target.rglob('*') if p.is_file())
if source_files != target_files or len(source_files) != 2:
    raise SystemExit('Signoff attachment file list differs')
for relative in source_files:
    if hashlib.sha256((source / relative).read_bytes()).digest() != hashlib.sha256((target / relative).read_bytes()).digest():
        raise SystemExit(f'Signoff attachment checksum differs: {relative}')
PY

echo '[9/10] Exercise PostgreSQL dump and isolated database restore'
docker exec "$pg" sh -ec 'export PGPASSWORD="$(cat /run/kpi-admin-secrets/postgres_admin_password)"; exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$scratch/kpi.dump"
test -s "$scratch/kpi.dump"
docker exec "$pg" psql -U kpi_admin -d postgres -v ON_ERROR_STOP=1 -c 'CREATE DATABASE kpi_restore_smoke' >/dev/null
docker exec -i -e POSTGRES_DB=kpi_restore_smoke "$pg" sh -s < "$root/scripts/provision-postgres-role.sh"
docker exec -i "$pg" sh -ec 'export PGPASSWORD="$(cat /run/kpi-admin-secrets/postgres_admin_password)"; exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d kpi_restore_smoke --no-owner --role="$KPI_APP_USER" --exit-on-error' < "$scratch/kpi.dump"
while IFS=$'\t' read -r table expected; do
    actual=$(docker exec "$pg" psql -U kpi_app -d kpi_restore_smoke -Atc "SELECT count(*) FROM public.\"$table\"")
    if [ "$actual" != "$expected" ]; then
        echo "Restored row count mismatch: $table expected $expected got $actual" >&2
        exit 1
    fi
done < "$scratch/counts.tsv"
owner=$(docker exec "$pg" psql -U kpi_admin -d kpi_restore_smoke -Atc "SELECT tableowner FROM pg_tables WHERE schemaname='public' AND tablename='employees'")
test "$owner" = kpi_app

echo '[10/10] Start migrated backend with app-only credentials and run healthcheck'
docker run -d --name "$backend" --network "$network" \
    -v "$scratch/secrets:/run/kpi-secrets:ro" \
    -v "$scratch/signoffs-target:/app/uploads/evaluation-confirmations" \
    -e APP_ENV=production -e DB_DRIVER=postgres \
    -e PGHOST=postgres -e PGPORT=5432 -e PGUSER=kpi_app -e PGDATABASE=kpi_smoke \
    -e PGPASSWORD_FILE=/run/kpi-secrets/postgres_password \
    -e JWT_SECRET="$jwt" -e CORS_ALLOWED_ORIGINS=http://localhost \
    -e SEED_HONGXING_CATALOG=false -e EVALUATION_WORKFLOW_MODE=audit \
    "$image" >/dev/null
backend_created=1
if ! docker exec "$backend" sh -ec 'test "$PGUSER" = kpi_app; test "$PGPASSWORD_FILE" = /run/kpi-secrets/postgres_password; test ! -e /run/kpi-admin-secrets/postgres_admin_password; test ! -e /run/kpi-bootstrap/bootstrap_admin_password'; then
    echo 'Backend received administrator credentials or the wrong application role' >&2
    exit 1
fi
docker exec "$backend" sh -ec 'test -s /app/uploads/evaluation-confirmations/smoke-signoff.png; test -s /app/uploads/evaluation-confirmations/smoke-supplement.png'
if docker inspect "$backend" --format '{{range .Mounts}}{{println .Destination}}{{end}}' | grep -q 'kpi-admin-secrets'; then
    echo 'Backend mounts the PostgreSQL administrator secret' >&2
    exit 1
fi
ready=0
for attempt in $(seq 1 30); do
    if docker exec "$backend" /app/kpi-server healthcheck >/dev/null 2>&1; then ready=1; break; fi
    sleep 2
done
if [ "$ready" -ne 1 ]; then echo 'Migrated backend healthcheck failed' >&2; exit 1; fi
while IFS=$'\t' read -r table expected; do
    actual=$(docker exec "$pg" psql -U kpi_app -d kpi_smoke -Atc "SELECT count(*) FROM public.\"$table\"")
    if [ "$actual" != "$expected" ]; then
        echo "Backend startup changed historical table count: $table expected $expected got $actual" >&2
        exit 1
    fi
done < "$scratch/counts.tsv"

echo 'PASS: synthetic SQLite migrated by one-time administrator; restricted backend, counts, retry guard and health verified.'
