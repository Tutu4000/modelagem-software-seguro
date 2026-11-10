#!/bin/sh
# One database role per module and per binary (E3.1, D02). Passwords come
# from Docker secrets.
#
# Keycloak gets its own database and owns it. The application roles cannot
# even connect to it.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v files_pw="$(cat /run/secrets/db_files_password)" \
	-v queue_pw="$(cat /run/secrets/db_queue_password)" \
	-v orders_pw="$(cat /run/secrets/db_orders_password)" \
	-v panel_orders_pw="$(cat /run/secrets/db_panel_orders_password)" \
	-v support_pw="$(cat /run/secrets/db_support_password)" \
	-v panel_support_pw="$(cat /run/secrets/db_panel_support_password)" \
	-v retention_pw="$(cat /run/secrets/db_retention_password)" \
	-v grafana_pw="$(cat /run/secrets/db_grafana_main_password)" \
	-v exporter_pw="$(cat /run/secrets/db_exporter_password)" \
	-v backup_pw="$(cat /run/secrets/db_backup_password)" \
	-v replicator_pw="$(cat /run/secrets/db_replicator_password)" \
	-v keycloak_pw="$(cat /run/secrets/db_keycloak_password)" <<'SQL'
CREATE ROLE files LOGIN PASSWORD :'files_pw';
CREATE ROLE queue LOGIN PASSWORD :'queue_pw';
CREATE ROLE orders LOGIN PASSWORD :'orders_pw';
CREATE ROLE panel_orders LOGIN PASSWORD :'panel_orders_pw';
CREATE ROLE support LOGIN PASSWORD :'support_pw';
CREATE ROLE panel_support LOGIN PASSWORD :'panel_support_pw';
CREATE ROLE retention LOGIN PASSWORD :'retention_pw';
CREATE ROLE grafana_main LOGIN PASSWORD :'grafana_pw';
CREATE ROLE exporter LOGIN PASSWORD :'exporter_pw';
CREATE ROLE backup LOGIN PASSWORD :'backup_pw';
CREATE ROLE replicator LOGIN REPLICATION PASSWORD :'replicator_pw';
CREATE ROLE keycloak LOGIN PASSWORD :'keycloak_pw';
CREATE DATABASE keycloak OWNER keycloak;
REVOKE ALL ON DATABASE keycloak FROM PUBLIC;
SQL

# The read replica (E3.4) streams from this server with its own role. The
# roles and grants above reach the replica through the replication itself.
echo "host replication replicator all scram-sha-256" >> "$PGDATA/pg_hba.conf"
