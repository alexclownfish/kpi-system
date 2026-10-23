#!/bin/sh
# Run inside the PostgreSQL container after the cluster becomes healthy.
set -eu

: "${POSTGRES_USER:?missing PostgreSQL administrator role}"
: "${POSTGRES_DB:?missing PostgreSQL database}"
app_user="${KPI_APP_USER:-kpi_app}"
case "$app_user" in
  ''|[0-9]*|*[!A-Za-z0-9_]*)
    echo "Invalid PostgreSQL application role name" >&2
    exit 1
    ;;
esac
case "$POSTGRES_DB" in
  ''|[0-9]*|*[!A-Za-z0-9_]*)
    echo "Invalid PostgreSQL database name" >&2
    exit 1
    ;;
esac
if [ "$app_user" = "$POSTGRES_USER" ]; then
  echo "Application role cannot be the PostgreSQL administrator" >&2
  exit 1
fi

app_password_file="${KPI_APP_PASSWORD_FILE:-/run/kpi-app-secrets/postgres_password}"
app_password="$(cat "$app_password_file")"
# Newly generated passwords are hex. Also accept existing base64 secrets so
# a rerun does not rotate a password created by an earlier deploy script.
case "$app_password" in
  ''|*[!A-Za-z0-9+/=]*)
    echo "Invalid PostgreSQL application password file" >&2
    exit 1
    ;;
esac

# The password is passed on stdin, never as a psql command argument or log line.
psql -X -q -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -v app_user="$app_user" -v db_name="$POSTGRES_DB" >/dev/null <<SQL
BEGIN;
SELECT format('CREATE ROLE %I LOGIN', :'app_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')
\gexec
ALTER ROLE :"app_user" WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '$app_password';
GRANT CONNECT, CREATE, TEMPORARY ON DATABASE :"db_name" TO :"app_user";
GRANT USAGE, CREATE ON SCHEMA public TO :"app_user";
COMMIT;
SQL
unset app_password
echo "Restricted PostgreSQL application role is ready: $app_user"
