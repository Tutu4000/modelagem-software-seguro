REVOKE ALL ON DATABASE audit FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE audit TO audit_app;
GRANT USAGE ON SCHEMA public TO audit_app;

-- Fields from section 3.2 of Stage 3: actor, target, action, UTC time, origin
-- and result (E7.1).
CREATE TABLE events (
	id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	occurred_at timestamptz NOT NULL,
	name        text NOT NULL,
	actor       text NOT NULL,
	action      text NOT NULL,
	target      text NOT NULL,
	result      text NOT NULL,
	reason      text NOT NULL,
	origin      text NOT NULL
);
CREATE INDEX events_name_occurred_at_idx ON events (name, occurred_at);
GRANT INSERT ON events TO audit_app;
