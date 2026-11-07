#!/bin/sh
# One database role per module (E3.1). Passwords come from Docker secrets.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v files_pw="$(cat /run/secrets/db_files_password)" \
	-v queue_pw="$(cat /run/secrets/db_queue_password)" <<'SQL'
CREATE ROLE files LOGIN PASSWORD :'files_pw';
CREATE ROLE queue LOGIN PASSWORD :'queue_pw';
SQL
