#!/bin/sh
# The application roles can only insert events: they cannot change or delete
# their own trail (E7.2, C18). The API, the panel and the retention job have
# one role each.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v app_pw="$(cat /run/secrets/db_audit_password)" \
	-v panel_pw="$(cat /run/secrets/db_panel_audit_password)" \
	-v retention_pw="$(cat /run/secrets/db_retention_audit_password)" <<'SQL'
CREATE ROLE audit_app LOGIN PASSWORD :'app_pw';
CREATE ROLE panel_audit LOGIN PASSWORD :'panel_pw';
CREATE ROLE retention_audit LOGIN PASSWORD :'retention_pw';
SQL
