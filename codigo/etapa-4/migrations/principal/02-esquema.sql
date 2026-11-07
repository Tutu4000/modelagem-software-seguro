-- Cada módulo só alcança as próprias tabelas (E3.1, C02). O dono das tabelas é
-- o superusuário, que a aplicação não usa.

REVOKE ALL ON DATABASE nugget FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE nugget TO arquivos, fila;
GRANT USAGE ON SCHEMA public TO arquivos, fila;

-- Módulo Arquivos
CREATE TABLE arquivos (
	id        uuid PRIMARY KEY,
	dono      uuid NOT NULL,
	tamanho   bigint NOT NULL CHECK (tamanho > 0),
	sha256    bytea NOT NULL CHECK (length(sha256) = 32),
	criado_em timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX arquivos_dono ON arquivos (dono);
GRANT SELECT, INSERT ON arquivos TO arquivos;

-- Módulo Fila: estado das tarefas e cota por conta (E4.3)
CREATE TABLE tarefas (
	id            uuid PRIMARY KEY,
	arquivo_id    uuid NOT NULL UNIQUE REFERENCES arquivos (id),
	dono          uuid NOT NULL,
	sha256        bytea NOT NULL CHECK (length(sha256) = 32),
	triangulos    bigint NOT NULL CHECK (triangulos >= 0),
	status        text NOT NULL CHECK (status IN ('aguardando', 'processando', 'concluida', 'falhou')),
	volume_mm3    double precision,
	caixa_mm      double precision[],
	criado_em     timestamptz NOT NULL DEFAULT now(),
	atualizado_em timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tarefas_dono_status ON tarefas (dono, status, criado_em);
GRANT SELECT, INSERT, UPDATE ON tarefas TO fila;
