#!/bin/sh
# Um papel por módulo (E3.1). As senhas vêm dos Docker secrets.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v arquivos_pw="$(cat /run/secrets/db_arquivos_password)" \
	-v fila_pw="$(cat /run/secrets/db_fila_password)" <<'SQL'
CREATE ROLE arquivos LOGIN PASSWORD :'arquivos_pw';
CREATE ROLE fila LOGIN PASSWORD :'fila_pw';
SQL
