REVOKE ALL ON DATABASE auditoria FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE auditoria TO auditoria_app;
GRANT USAGE ON SCHEMA public TO auditoria_app;

-- Campos da seção 3.2 da Etapa 3: autor, alvo, operação, horário UTC, origem e
-- resultado (E7.1).
CREATE TABLE eventos (
	id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	horario   timestamptz NOT NULL,
	evento    text NOT NULL,
	autor     text NOT NULL,
	operacao  text NOT NULL,
	alvo      text NOT NULL,
	resultado text NOT NULL,
	motivo    text NOT NULL,
	origem    text NOT NULL
);
CREATE INDEX eventos_evento_horario ON eventos (evento, horario);
GRANT INSERT ON eventos TO auditoria_app;
