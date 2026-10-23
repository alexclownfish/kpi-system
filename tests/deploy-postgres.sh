#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/kpi-pg-deploy-test.XXXXXX")"
trap '[[ -n "$fixture_root" && -d "$fixture_root" ]] && rm -rf -- "$fixture_root"' EXIT
real_docker="$(command -v docker)"

fixture_repo="$fixture_root/fixture-repo"
deploy_root="$fixture_root/runtime"
mkdir -p "$fixture_repo/scripts" "$deploy_root/ci-build" "$fixture_root/bin"
cp "$repo_root/scripts/deploy-production.sh" "$fixture_repo/scripts/"
cp "$repo_root/scripts/build-docker-images.sh" "$fixture_repo/scripts/"
cp "$repo_root/scripts/provision-postgres-role.sh" "$fixture_repo/scripts/"
cp "$repo_root/docker-compose.yml" "$repo_root/docker-compose.postgres.yml" "$fixture_repo/"
cp "$repo_root/Dockerfile.backend" "$repo_root/Dockerfile.frontend" "$repo_root/Dockerfile.nginx" "$fixture_repo/"
chmod +x "$fixture_repo/scripts/build-docker-images.sh"
git -C "$fixture_repo" init -q
git -C "$fixture_repo" add .
git -C "$fixture_repo" -c user.name=Test -c user.email=test@example.com commit -qm 'deploy fixture'
cp "$repo_root/scripts/deploy-production.sh" "$deploy_root/ci-build/deploy-production.sh"

cat > "$fixture_root/bin/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TEST_DOCKER_LOG"
if [[ "$1" == compose && "$2" == version ]]; then
  echo 'Docker Compose version test'
fi
if [[ "$*" == *' config --environment' ]]; then
  args=("$@")
  for ((i=0; i<${#args[@]}; i++)); do
    if [[ "${args[$i]}" == --env-file ]]; then
      cat "${args[$((i+1))]}"
      exit 0
    fi
  done
fi
EOF
chmod +x "$fixture_root/bin/docker"
cat > "$fixture_root/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${@: -1}" >> "$TEST_CURL_LOG"
exit 0
EOF
chmod +x "$fixture_root/bin/curl"
cat > "$fixture_root/bin/psql" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$TEST_PSQL_ARGS"
cat > "$TEST_PSQL_SQL"
EOF
chmod +x "$fixture_root/bin/psql"
export TEST_DOCKER_LOG="$fixture_root/docker-calls.log"
export TEST_CURL_LOG="$fixture_root/curl-calls.log"
export TEST_PSQL_ARGS="$fixture_root/psql-args.log"
export TEST_PSQL_SQL="$fixture_root/psql-stdin.sql"
export PATH="$fixture_root/bin:$PATH"

bash "$deploy_root/ci-build/deploy-production.sh" \
  --postgres --db-only --domain https://kpi.example.com \
  --repo-url "$fixture_repo" > "$fixture_root/deploy-output.log"

test -d "$deploy_root/ci-build/source/.git"
test -f "$deploy_root/docker-compose.yml"
test -f "$deploy_root/docker-compose.postgres.yml"
test -s "$deploy_root/secrets/postgres_password"
test -s "$deploy_root/pg-admin-secrets/postgres_admin_password"
! cmp -s "$deploy_root/secrets/postgres_password" "$deploy_root/pg-admin-secrets/postgres_admin_password"
grep -q '^COMPOSE_PROJECT_NAME=kpi-system-pg$' "$deploy_root/.env"
grep -q '^PG_ADMIN_USER=kpi_admin$' "$deploy_root/.env"
grep -q '^PG_USER=kpi_app$' "$deploy_root/.env"
grep -q '^SEED_HONGXING_CATALOG=false$' "$deploy_root/.env"
grep -q '^BOOTSTRAP_ADMIN_EMAIL=$' "$deploy_root/.env"
grep -q '^BOOTSTRAP_ADMIN_NAME=$' "$deploy_root/.env"
grep -q 'up -d --wait postgres$' "$TEST_DOCKER_LOG"
grep -q 'exec -T --user 0 postgres sh -s$' "$TEST_DOCKER_LOG"
! grep -q 'up -d --no-build' "$TEST_DOCKER_LOG"
! grep -q 'POSTGRES_PASSWORD' "$deploy_root/.env"
python3 -c 'import os,stat,sys; assert stat.S_IMODE(os.stat(sys.argv[1]).st_mode)==0o600' "$deploy_root/secrets/postgres_password"
python3 -c 'import os,stat,sys; assert stat.S_IMODE(os.stat(sys.argv[1]).st_mode)==0o600' "$deploy_root/pg-admin-secrets/postgres_admin_password"
POSTGRES_USER=kpi_admin POSTGRES_DB=kpi KPI_APP_USER=kpi_app \
  KPI_APP_PASSWORD_FILE="$deploy_root/secrets/postgres_password" \
  sh "$repo_root/scripts/provision-postgres-role.sh" > "$fixture_root/role-output.log"
grep -q 'NOSUPERUSER NOCREATEDB NOCREATEROLE' "$TEST_PSQL_SQL"
grep -q 'GRANT CONNECT, CREATE, TEMPORARY ON DATABASE' "$TEST_PSQL_SQL"
grep -q 'GRANT USAGE, CREATE ON SCHEMA public' "$TEST_PSQL_SQL"
app_password="$(cat "$deploy_root/secrets/postgres_password")"
grep -Fq "$app_password" "$TEST_PSQL_SQL"
! grep -Fq "$app_password" "$TEST_PSQL_ARGS"

# Verify the copied runtime files also satisfy the real Compose parser.
"$real_docker" compose --project-directory "$deploy_root" \
  --env-file "$deploy_root/.env" \
  -f "$deploy_root/docker-compose.yml" \
  -f "$deploy_root/docker-compose.postgres.yml" config --quiet
"$real_docker" compose --project-directory "$deploy_root" \
  --env-file "$deploy_root/.env" \
  -f "$deploy_root/docker-compose.yml" \
  -f "$deploy_root/docker-compose.postgres.yml" config --format json |
  python3 -c 'import json,sys; s=json.load(sys.stdin)["services"]; b=s["backend"]; p=s["postgres"]; assert b["environment"]["PGUSER"]=="kpi_app"; assert p["environment"]["POSTGRES_USER"]=="kpi_admin"; assert not p.get("ports"); assert not any("pg-admin-secrets" in str(v) for v in b["volumes"]); assert any("pg-admin-secrets" in str(v) for v in p["volumes"])'

# The normal user path runs the script inside ci-build/source/scripts.
second_root="$fixture_root/from-source"
mkdir -p "$second_root/ci-build"
git clone -q -- "$fixture_repo" "$second_root/ci-build/source"
bash "$second_root/ci-build/source/scripts/deploy-production.sh" \
  --postgres --db-only --skip-pull --domain https://kpi.example.com \
  > "$fixture_root/source-stage-output.log"
test -f "$second_root/.env"
test -f "$second_root/docker-compose.postgres.yml"
test ! -f "$second_root/ci-build/source/.env"

# An existing .env must not point staging at the SQLite project, reuse the
# admin role, seed catalog data, or bootstrap accounts before migration.
cp "$second_root/.env" "$fixture_root/safe-env"
staging_starts_before="$(grep -c 'up -d --wait postgres$' "$TEST_DOCKER_LOG")"
assert_unsafe_env_rejected() {
  local expected="$1"
  if bash "$second_root/ci-build/source/scripts/deploy-production.sh" \
    --postgres --db-only --skip-pull > "$fixture_root/unsafe-output.log" 2>&1; then
    echo 'Unsafe PostgreSQL staging configuration was accepted' >&2
    exit 1
  fi
  grep -q "$expected" "$fixture_root/unsafe-output.log"
  test "$(grep -c 'up -d --wait postgres$' "$TEST_DOCKER_LOG")" = "$staging_starts_before"
}
sed 's/^COMPOSE_PROJECT_NAME=kpi-system-pg$/COMPOSE_PROJECT_NAME=kpi-system/' "$fixture_root/safe-env" > "$second_root/.env"
assert_unsafe_env_rejected 'requires a distinct COMPOSE_PROJECT_NAME'
sed 's/^PG_USER=kpi_app$/PG_USER=kpi_admin/' "$fixture_root/safe-env" > "$second_root/.env"
assert_unsafe_env_rejected 'PG_USER and PG_ADMIN_USER must be distinct'
sed 's/^SEED_HONGXING_CATALOG=false$/SEED_HONGXING_CATALOG=true/' "$fixture_root/safe-env" > "$second_root/.env"
assert_unsafe_env_rejected 'SEED_HONGXING_CATALOG=false'
sed 's/^SEED_HONGXING_CATALOG=false$/SEED_HONGXING_CATALOG=/' "$fixture_root/safe-env" > "$second_root/.env"
assert_unsafe_env_rejected 'SEED_HONGXING_CATALOG=false'
sed 's/^BOOTSTRAP_ADMIN_EMAIL=$/BOOTSTRAP_ADMIN_EMAIL=admin@example.com/' "$fixture_root/safe-env" > "$second_root/.env"
assert_unsafe_env_rejected 'requires BOOTSTRAP_ADMIN_EMAIL to be empty'
cp "$fixture_root/safe-env" "$second_root/.env"

# After migration, the same checkout builds application images and starts the
# whole stack with the PostgreSQL overlay while retaining its secret file.
cp "$second_root/secrets/postgres_password" "$fixture_root/original-pg-password"
if bash "$second_root/ci-build/source/scripts/deploy-production.sh" \
  --postgres --skip-pull > "$fixture_root/premature-release-output.log" 2>&1; then
  echo 'PostgreSQL application started before migration approval' >&2
  exit 1
fi
grep -q 'application startup is blocked' "$fixture_root/premature-release-output.log"
: > "$second_root/secrets/postgres_migration_verified"
bash "$second_root/ci-build/source/scripts/deploy-production.sh" \
  --postgres --skip-pull > "$fixture_root/source-release-output.log"
cmp "$fixture_root/original-pg-password" "$second_root/secrets/postgres_password"
grep -q 'up -d --no-build$' "$TEST_DOCKER_LOG"
grep -q -- '-f .*docker-compose.postgres.yml up -d --no-build$' "$TEST_DOCKER_LOG"
grep -Fxq 'http://127.0.0.1:8088/health' "$TEST_CURL_LOG"

# The health check follows Compose's effective bind address and port. Private
# interfaces must not be probed through loopback; wildcard binds can be.
sed -e 's/^KPI_HTTP_BIND=127\.0\.0\.1$/KPI_HTTP_BIND=172.16.230.14/' \
  -e 's/^KPI_HTTP_PORT=8088$/KPI_HTTP_PORT=18090/' \
  "$fixture_root/safe-env" > "$second_root/.env"
bash "$second_root/ci-build/source/scripts/deploy-production.sh" \
  --postgres --skip-pull > "$fixture_root/private-bind-output.log"
grep -Fxq 'http://172.16.230.14:18090/health' "$TEST_CURL_LOG"
sed 's/^KPI_HTTP_BIND=172\.16\.230\.14$/KPI_HTTP_BIND=0.0.0.0/' \
  "$second_root/.env" > "$fixture_root/wildcard-env"
cp "$fixture_root/wildcard-env" "$second_root/.env"
bash "$second_root/ci-build/source/scripts/deploy-production.sh" \
  --postgres --skip-pull > "$fixture_root/wildcard-bind-output.log"
grep -Fxq 'http://127.0.0.1:18090/health' "$TEST_CURL_LOG"

echo 'PostgreSQL cold-deploy staging test passed'
