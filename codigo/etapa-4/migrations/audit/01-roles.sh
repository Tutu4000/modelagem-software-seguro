#!/bin/sh
# The application role can only insert events: it cannot change or delete its
# own trail (E7.2, C18).
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v app_pw="$(cat /run/secrets/db_audit_password)" <<'SQL'
CREATE ROLE audit_app LOGIN PASSWORD :'app_pw';
SQL
