REVOKE ALL ON DATABASE audit FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE audit TO audit_app, panel_audit;
GRANT USAGE ON SCHEMA public TO audit_app, panel_audit;

-- Fields from section 3.2 of Stage 3: actor, target, action, UTC time, origin
-- and result (E7.1). The ID is a UUIDv7: events are internal, and the key
-- follows insertion order.
CREATE TABLE events (
	id          uuid PRIMARY KEY DEFAULT uuidv7(),
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
GRANT INSERT ON events TO audit_app, panel_audit;
