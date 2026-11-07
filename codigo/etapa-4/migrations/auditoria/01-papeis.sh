#!/bin/sh
# O papel da aplicação só insere eventos: não altera nem apaga o próprio
# rastro (E7.2, C18).
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v app_pw="$(cat /run/secrets/db_auditoria_password)" <<'SQL'
CREATE ROLE auditoria_app LOGIN PASSWORD :'app_pw';
SQL
