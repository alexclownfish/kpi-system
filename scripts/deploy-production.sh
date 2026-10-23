#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/deploy-production.sh [options]

Pull Git code, build versioned backend/frontend/Nginx images, and redeploy with
Docker Compose. Data volumes are preserved.

Options:
  --tag TAG             Image tag. Default: UTC timestamp + Git short SHA
  --prefix PREFIX       Image name prefix. Default: kpi-system
  --domain DOMAIN        Public HTTPS URL for a first-time .env, e.g. https://kpi.example.com
  --admin-email EMAIL    First bootstrap administrator email for a first-time .env
  --admin-name NAME      First bootstrap administrator name. Default: admin
  --postgres             Use PostgreSQL Compose overlay and a separate data volume
                         App startup requires secrets/postgres_migration_verified
  --db-only              Start only PostgreSQL for a SQLite data migration (requires --postgres)
  --repo-url URL         Git clone URL when ci-build/source is empty
  --push                Push backend/frontend/Nginx images before deployment
  --skip-pull           Skip git pull and deploy the current checkout
  --platform PLATFORM   Optional build platform, e.g. linux/amd64
  --no-cache            Build images without Docker layer cache
  -h, --help            Show this help

Examples:
  ci-build/deploy-production.sh
  scripts/deploy-production.sh
  scripts/deploy-production.sh \
    --domain https://kpi.example.com \
    --admin-email admin@example.com
  scripts/deploy-production.sh --skip-pull
  scripts/deploy-production.sh --postgres --db-only --domain https://kpi.example.com
  scripts/deploy-production.sh \
    --prefix registry.example.com/team/kpi \
    --tag 2026.10.21 \
    --push
EOF
}

image_tag="${IMAGE_TAG:-}"
image_prefix="${IMAGE_PREFIX:-kpi-system}"
public_domain="${DEPLOY_PUBLIC_DOMAIN:-}"
admin_email="${DEPLOY_ADMIN_EMAIL:-}"
admin_name="${DEPLOY_ADMIN_NAME:-admin}"
push_images=false
skip_git_pull=false
build_platform="${BUILD_PLATFORM:-}"
no_cache=false
use_postgres=false
db_only=false
repository_url="${KPI_GIT_REPOSITORY:-git@github.com:alexclownfish/kpi-system.git}"
original_args=("$@")

while (($# > 0)); do
  case "$1" in
    --tag)
      [[ $# -ge 2 ]] || { echo "Missing value for --tag" >&2; exit 2; }
      image_tag="$2"
      shift 2
      ;;
    --prefix)
      [[ $# -ge 2 ]] || { echo "Missing value for --prefix" >&2; exit 2; }
      image_prefix="${2%/}"
      shift 2
      ;;
    --domain)
      [[ $# -ge 2 ]] || { echo "Missing value for --domain" >&2; exit 2; }
      public_domain="$2"
      shift 2
      ;;
    --admin-email)
      [[ $# -ge 2 ]] || { echo "Missing value for --admin-email" >&2; exit 2; }
      admin_email="$2"
      shift 2
      ;;
    --admin-name)
      [[ $# -ge 2 ]] || { echo "Missing value for --admin-name" >&2; exit 2; }
      admin_name="$2"
      shift 2
      ;;
    --postgres)
      use_postgres=true
      shift
      ;;
    --db-only)
      db_only=true
      shift
      ;;
    --repo-url)
      [[ $# -ge 2 ]] || { echo "Missing value for --repo-url" >&2; exit 2; }
      repository_url="$2"
      shift 2
      ;;
    --push)
      push_images=true
      shift
      ;;
    --skip-pull)
      skip_git_pull=true
      shift
      ;;
    --platform)
      [[ $# -ge 2 ]] || { echo "Missing value for --platform" >&2; exit 2; }
      build_platform="$2"
      shift 2
      ;;
    --no-cache)
      no_cache=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done
if [[ "$db_only" == true && "$use_postgres" != true ]]; then
  echo "--db-only requires --postgres." >&2
  exit 2
fi
if [[ "$db_only" == true && -n "$admin_email" ]]; then
  echo "--db-only cannot set a bootstrap administrator before migrating old accounts." >&2
  exit 2
fi

# Re-exec from a temporary copy because `git pull` can update this script while
# Bash is still reading the original file.
if [[ "${KPI_DEPLOY_REEXEC:-0}" != "1" ]]; then
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  script_path="$script_dir/$(basename "${BASH_SOURCE[0]}")"
  if [[ "$(basename "$script_dir")" == "ci-build" ]]; then
    source_dir="$script_dir/source"
    deploy_root="$(cd "$script_dir/.." && pwd)"
    compose_file="$deploy_root/docker-compose.yml"
  else
    source_dir="$(cd "$script_dir/.." && pwd)"
    if [[ "$(basename "$source_dir")" == "source" && "$(basename "$(dirname "$source_dir")")" == "ci-build" ]]; then
      deploy_root="$(cd "$source_dir/../.." && pwd)"
    else
      deploy_root="$source_dir"
    fi
    compose_file="$deploy_root/docker-compose.yml"
  fi
  env_file="$deploy_root/.env"
  temp_script="$(mktemp "${TMPDIR:-/tmp}/kpi-deploy.XXXXXX")"
  cp "$script_path" "$temp_script"
  chmod 700 "$temp_script"
  set +e
  KPI_DEPLOY_REEXEC=1 \
    KPI_DEPLOY_SOURCE_DIR="$source_dir" \
    KPI_DEPLOY_ROOT="$deploy_root" \
    KPI_DEPLOY_COMPOSE_FILE="$compose_file" \
    KPI_DEPLOY_ENV_FILE="$env_file" \
    bash "$temp_script" "${original_args[@]}"
  rc=$?
  set -e
  rm -f "$temp_script"
  exit "$rc"
fi

source_dir="${KPI_DEPLOY_SOURCE_DIR:?missing source directory}"
deploy_root="${KPI_DEPLOY_ROOT:?missing deployment root}"
compose_file="${KPI_DEPLOY_COMPOSE_FILE:?missing compose file}"
env_file="${KPI_DEPLOY_ENV_FILE:?missing env file}"
pg_compose_file="$deploy_root/docker-compose.postgres.yml"

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  echo "Docker with the Compose plugin is required." >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required for first-time secret generation." >&2
  exit 1
fi

cloned=false
if [[ ! -d "$source_dir/.git" ]]; then
  if [[ "$skip_git_pull" == true ]]; then
    echo "Cannot --skip-pull before ci-build/source has been cloned." >&2
    exit 1
  fi
  if [[ -d "$source_dir" ]] && [[ -n "$(ls -A "$source_dir")" ]]; then
    echo "Source path is nonempty and not a Git clone: $source_dir" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$source_dir")"
  echo "==> Cloning source into $source_dir"
  git clone -- "$repository_url" "$source_dir"
  cloned=true
fi
cd "$source_dir"

if [[ "$skip_git_pull" != true && "$cloned" != true ]]; then
  if ! git rev-parse --git-dir >/dev/null 2>&1; then
    echo "Not a Git repository. Use --skip-pull only for a prepared source directory." >&2
    exit 1
  fi
  echo "==> Pulling latest code"
  git status --short
  git pull --ff-only
fi

if [[ ! -f Dockerfile.backend || ! -f Dockerfile.frontend ]]; then
  echo "Dockerfiles not found in $source_dir. Deploy from a complete source clone." >&2
  exit 1
fi
if [[ ! -f "$compose_file" ]]; then
  cp "$source_dir/docker-compose.yml" "$compose_file"
  echo "==> Created runtime Compose file: $compose_file"
fi
if [[ "$use_postgres" == true ]]; then
  if [[ ! -f "$source_dir/docker-compose.postgres.yml" ]]; then
    echo "PostgreSQL Compose overlay is missing from the source checkout." >&2
    exit 1
  fi
  if [[ ! -f "$source_dir/scripts/provision-postgres-role.sh" ]]; then
    echo "PostgreSQL application-role setup script is missing from the source checkout." >&2
    exit 1
  fi
  if [[ ! -f "$pg_compose_file" ]]; then
    cp "$source_dir/docker-compose.postgres.yml" "$pg_compose_file"
    echo "==> Created PostgreSQL Compose overlay: $pg_compose_file"
  fi
  secret_dir="$deploy_root/secrets"
  mkdir -p "$secret_dir"
  chmod 700 "$secret_dir"
  pg_password_file="$secret_dir/postgres_password"
  if [[ ! -f "$pg_password_file" ]]; then
    umask 077
    openssl rand -hex 32 | tr -d '\r\n' > "$pg_password_file"
    echo "==> Created PostgreSQL application password file: $pg_password_file (content not printed)"
  fi
  chmod 600 "$pg_password_file"
  if [[ ! -s "$pg_password_file" ]]; then
    echo "PostgreSQL application password file is empty: $pg_password_file" >&2
    exit 1
  fi
  pg_admin_secret_dir="$deploy_root/pg-admin-secrets"
  mkdir -p "$pg_admin_secret_dir"
  chmod 700 "$pg_admin_secret_dir"
  pg_admin_password_file="$pg_admin_secret_dir/postgres_admin_password"
  if [[ ! -f "$pg_admin_password_file" ]]; then
    umask 077
    openssl rand -hex 32 | tr -d '\r\n' > "$pg_admin_password_file"
    echo "==> Created PostgreSQL administrator password file: $pg_admin_password_file (content not printed)"
  fi
  chmod 600 "$pg_admin_password_file"
  if [[ ! -s "$pg_admin_password_file" ]]; then
    echo "PostgreSQL administrator password file is empty: $pg_admin_password_file" >&2
    exit 1
  fi
  if cmp -s "$pg_password_file" "$pg_admin_password_file"; then
    echo "PostgreSQL application and administrator passwords must differ." >&2
    exit 1
  fi
fi

if [[ ! -f "$env_file" ]]; then
  if [[ -z "$public_domain" || ( "$db_only" != true && -z "$admin_email" ) ]]; then
    echo "First deployment requires --domain; --admin-email is also required unless --db-only is used." >&2
    exit 1
  fi
  if [[ "$public_domain" != http://* && "$public_domain" != https://* ]]; then
    public_domain="https://$public_domain"
  fi
  if [[ -n "$admin_email" ]]; then
    secret_dir="$deploy_root/secrets"
    mkdir -p "$secret_dir"
    chmod 700 "$secret_dir"
    admin_password_file="$secret_dir/bootstrap_admin_password"
    if [[ ! -f "$admin_password_file" ]]; then
      umask 077
      openssl rand -base64 24 | tr -d '\r\n' > "$admin_password_file"
    fi
    bootstrap_password_env="BOOTSTRAP_ADMIN_PASSWORD_FILE=/run/kpi-secrets/bootstrap_admin_password"
  else
    bootstrap_password_env="BOOTSTRAP_ADMIN_PASSWORD_FILE="
  fi
  umask 022
  jwt_secret="$(openssl rand -base64 48 | tr -d '\r\n')"
  project_name=kpi-system
  seed_catalog=true
  workflow_mode=enabled
  configured_admin_name="$admin_name"
  if [[ "$use_postgres" == true ]]; then
    project_name=kpi-system-pg
    if [[ "$db_only" == true ]]; then
      seed_catalog=false
      workflow_mode=audit
      configured_admin_name=""
    fi
  fi
  cat > "$env_file" <<EOF
APP_ENV=production
COMPOSE_PROJECT_NAME=$project_name
SEED_HONGXING_CATALOG=$seed_catalog
EVALUATION_WORKFLOW_MODE=$workflow_mode
JWT_SECRET=$jwt_secret
CORS_ALLOWED_ORIGINS=$public_domain
KPI_HTTP_BIND=127.0.0.1
KPI_HTTP_PORT=8088
PG_DATABASE=kpi
PG_ADMIN_USER=kpi_admin
PG_USER=kpi_app
BOOTSTRAP_ADMIN_EMAIL=$admin_email
BOOTSTRAP_ADMIN_NAME=$configured_admin_name
$bootstrap_password_env
BOOTSTRAP_SUPER_ADMIN_EMAIL=
EOF
  chmod 600 "$env_file"
  echo "==> Created production .env: $env_file"
  if [[ -n "$admin_email" ]]; then
    echo "==> Bootstrap administrator password: $admin_password_file (content not printed)"
  fi
fi

if [[ ! -f "$env_file" ]]; then
  echo "Production .env is missing: $env_file" >&2
  exit 1
fi
if ! grep -q 'KPI_BACKEND_IMAGE' "$compose_file" || ! grep -q 'KPI_SOURCE_DIR' "$compose_file"; then
  echo "Runtime Compose file is outdated. Copy source/docker-compose.yml to $compose_file once before deploying." >&2
  exit 1
fi
if [[ "$use_postgres" == true ]] && ! grep -q 'kpi-postgres-data' "$pg_compose_file"; then
  echo "Runtime PostgreSQL overlay is outdated. Review and copy source/docker-compose.postgres.yml to $pg_compose_file." >&2
  exit 1
fi
if [[ "$use_postgres" == true && "$db_only" != true ]]; then
  migration_marker="$deploy_root/secrets/postgres_migration_verified"
  if [[ ! -f "$migration_marker" || -L "$migration_marker" ]]; then
    echo "PostgreSQL application startup is blocked until migration and attachment checks pass." >&2
    echo "After verification, create the operator approval file: $migration_marker" >&2
    exit 1
  fi
fi
revision="$(git rev-parse --short HEAD 2>/dev/null || printf 'nogit')"
if [[ -z "$image_tag" ]]; then
  image_tag="$(date -u +%Y%m%d-%H%M%S)-${revision}"
fi

backend_image="${image_prefix}-backend:${image_tag}"
frontend_image="${image_prefix}-frontend:${image_tag}"
nginx_image="${image_prefix}-nginx:${image_tag}"

echo "==> Building images"
build_args=(
  --prefix "$image_prefix"
  --tag "$image_tag"
  --with-nginx
  --skip-pull
)
[[ -n "$build_platform" ]] && build_args+=(--platform "$build_platform")
[[ "$no_cache" == true ]] && build_args+=(--no-cache)
[[ "$push_images" == true ]] && build_args+=(--push)
if [[ "$db_only" != true ]]; then
  "$source_dir/scripts/build-docker-images.sh" "${build_args[@]}"
fi

export KPI_BACKEND_IMAGE="$backend_image"
export KPI_FRONTEND_IMAGE="$frontend_image"
export KPI_NGINX_IMAGE="$nginx_image"
export KPI_SOURCE_DIR="$source_dir"
if [[ -z "${COMPOSE_PROJECT_NAME:-}" ]]; then
  env_project_name="$(awk -F= '$1 == "COMPOSE_PROJECT_NAME" { value=$2 } END { print value }' "$env_file" 2>/dev/null || true)"
  env_project_name="${env_project_name%\"}"
  env_project_name="${env_project_name#\"}"
  COMPOSE_PROJECT_NAME="$env_project_name"
fi
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-kpi-system}"
if [[ "$use_postgres" == true && "$COMPOSE_PROJECT_NAME" == "kpi-system" ]]; then
  echo "PostgreSQL deployment requires a distinct COMPOSE_PROJECT_NAME, e.g. kpi-system-pg, to avoid reusing SQLite-era volumes." >&2
  exit 1
fi

compose_files=(-f "$compose_file")
if [[ "$use_postgres" == true ]]; then
  compose_files+=(-f "$pg_compose_file")
fi

compose() {
  docker compose \
    --project-directory "$deploy_root" \
    --env-file "$env_file" \
    "${compose_files[@]}" \
    "$@"
}

echo "==> Validating Compose configuration"
compose config --quiet
# Use the same interpolated values that Compose uses for the published HTTP port.
# Never print this output: it may also contain JWT_SECRET.
effective_env="$(compose config --environment)"
effective_value() {
  local key="$1"
  printf '%s\n' "$effective_env" | awk -v wanted="$key" '
    index($0, wanted "=") == 1 { sub(/^[^=]*=/, ""); print; exit }
  '
}
if [[ "$use_postgres" == true ]]; then
  effective_app_user="$(effective_value PG_USER)"
  effective_admin_user="$(effective_value PG_ADMIN_USER)"
  if [[ -z "$effective_app_user" || -z "$effective_admin_user" || "$effective_app_user" == "$effective_admin_user" ]]; then
    echo "PG_USER and PG_ADMIN_USER must be distinct, nonempty PostgreSQL roles." >&2
    exit 1
  fi
  if [[ "$db_only" == true ]]; then
    seed_mode="$(effective_value SEED_HONGXING_CATALOG)"
    if [[ "$(printf '%s' "$seed_mode" | tr '[:upper:]' '[:lower:]')" != false ]]; then
      echo "--db-only requires SEED_HONGXING_CATALOG=false while old data is pending migration." >&2
      exit 1
    fi
    for bootstrap_key in BOOTSTRAP_ADMIN_EMAIL BOOTSTRAP_ADMIN_NAME BOOTSTRAP_ADMIN_PASSWORD_FILE BOOTSTRAP_SUPER_ADMIN_EMAIL; do
      if [[ -n "$(effective_value "$bootstrap_key")" ]]; then
        echo "--db-only requires $bootstrap_key to be empty while old accounts are pending migration." >&2
        exit 1
      fi
    done
  fi
fi
if [[ "$db_only" == true ]]; then
  echo "==> Starting PostgreSQL only; application remains stopped for SQLite migration"
  compose up -d --wait postgres
  echo "==> Provisioning restricted PostgreSQL application role"
  compose exec -T --user 0 postgres sh -s < "$source_dir/scripts/provision-postgres-role.sh"
  compose ps postgres
  echo "PostgreSQL is ready. Migrate and verify the old SQLite data before starting the application."
  exit 0
fi

echo "==> Deploying backend/frontend/Nginx images"
compose up -d --no-build

http_bind="$(effective_value KPI_HTTP_BIND)"
http_port="$(effective_value KPI_HTTP_PORT)"
http_bind="${http_bind:-0.0.0.0}"
http_port="${http_port:-80}"
case "$http_bind" in
  0.0.0.0) health_host=127.0.0.1 ;;
  ::|"[::]") health_host="[::1]" ;;
  *:*)
    if [[ "$http_bind" == \[*\] ]]; then
      health_host="$http_bind"
    else
      health_host="[$http_bind]"
    fi
    ;;
  *) health_host="$http_bind" ;;
esac
health_url="http://${health_host}:${http_port}/health"

ready=false
for _ in {1..30}; do
  if curl --silent --show-error --fail "$health_url" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 2
done

if [[ "$ready" != true ]]; then
  echo "Deployment health check failed: $health_url" >&2
  compose ps >&2 || true
  compose logs --tail=200 backend >&2 || true
  exit 1
fi

echo
echo "Deployment complete"
echo "Backend image:  $backend_image"
echo "Frontend image: $frontend_image"
echo "Nginx image:    $nginx_image"
echo "Health:         $health_url"
compose ps
