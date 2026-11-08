#!/bin/sh
# One database role per module (E3.1). Passwords come from Docker secrets.
#
# Keycloak gets its own database and owns it. The application roles cannot
# even connect to it.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v files_pw="$(cat /run/secrets/db_files_password)" \
	-v queue_pw="$(cat /run/secrets/db_queue_password)" \
	-v keycloak_pw="$(cat /run/secrets/db_keycloak_password)" <<'SQL'
CREATE ROLE files LOGIN PASSWORD :'files_pw';
CREATE ROLE queue LOGIN PASSWORD :'queue_pw';
CREATE ROLE keycloak LOGIN PASSWORD :'keycloak_pw';
CREATE DATABASE keycloak OWNER keycloak;
REVOKE ALL ON DATABASE keycloak FROM PUBLIC;
SQL
