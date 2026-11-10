-- After the purchase: notifications, cancellation, support, address change
-- and retention (E8.4, E8.5, RF10, RF11, C21).

GRANT CONNECT ON DATABASE nugget TO support, panel_support, retention;
GRANT USAGE ON SCHEMA public TO support, panel_support, retention;

-- The registered e-mail, taken from the verified token when the order is
-- quoted: the webhooks and the panel notify the customer without asking the
-- IdP. Retention clears the address once it is no longer needed.
ALTER TABLE orders
	ADD COLUMN contact_email     text NOT NULL DEFAULT '',
	ADD COLUMN address_purged_at timestamptz;

-- Refunds of a cancelled order (RF10).
ALTER TABLE payments DROP CONSTRAINT payments_status_check;
ALTER TABLE payments
	ADD CONSTRAINT payments_status_check CHECK (status IN ('pending', 'succeeded', 'failed', 'refund_pending', 'refunded')),
	ADD COLUMN refund_id text;
GRANT UPDATE (status, refund_id, updated_at) ON payments TO orders;

-- Support tickets (RF11). The messages are plain text, kept as typed.
CREATE TABLE tickets (
	id            uuid PRIMARY KEY DEFAULT uuidv7(),
	owner_id      uuid NOT NULL,
	order_id      uuid REFERENCES orders (id),
	subject       text NOT NULL CHECK (length(subject) BETWEEN 1 AND 200),
	status        text NOT NULL CHECK (status IN ('open', 'answered', 'closed')),
	contact_email text NOT NULL,
	created_at    timestamptz NOT NULL DEFAULT now(),
	updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tickets_owner_id_idx ON tickets (owner_id, created_at);
CREATE INDEX tickets_status_idx ON tickets (status, updated_at);

CREATE TABLE ticket_messages (
	id          uuid PRIMARY KEY DEFAULT uuidv7(),
	ticket_id   uuid NOT NULL REFERENCES tickets (id),
	author_id   uuid NOT NULL,
	author_kind text NOT NULL CHECK (author_kind IN ('customer', 'staff')),
	body        text NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
	created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ticket_messages_ticket_id_idx ON ticket_messages (ticket_id, created_at);

-- The support module checks that a ticket's order belongs to the customer.
GRANT SELECT (id, owner_id) ON orders TO support;
GRANT SELECT, INSERT ON tickets, ticket_messages TO support;
GRANT UPDATE (status, updated_at) ON tickets TO support;
GRANT SELECT ON tickets, ticket_messages TO panel_support;
GRANT INSERT ON ticket_messages TO panel_support;
GRANT UPDATE (status, updated_at) ON tickets TO panel_support;

-- Address change proposed by support and confirmed by the customer with a
-- code sent to the registered e-mail (C21). Only the code's hash is kept.
CREATE TABLE address_changes (
	id               uuid PRIMARY KEY DEFAULT uuidv7(),
	order_id         uuid NOT NULL REFERENCES orders (id),
	requested_by     uuid NOT NULL,
	ticket_id        uuid REFERENCES tickets (id),
	new_recipient    text NOT NULL,
	new_street       text NOT NULL,
	new_number       text NOT NULL,
	new_city         text NOT NULL,
	new_state        text NOT NULL,
	new_postal_code  text NOT NULL,
	code_hash        bytea NOT NULL CHECK (length(code_hash) = 32),
	attempts         integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
	expires_at       timestamptz NOT NULL,
	status           text NOT NULL CHECK (status IN ('pending', 'confirmed', 'expired', 'locked', 'superseded')),
	created_at       timestamptz NOT NULL DEFAULT now(),
	decided_at       timestamptz
);
CREATE UNIQUE INDEX address_changes_one_pending_per_order_idx ON address_changes (order_id) WHERE status = 'pending';

GRANT SELECT, INSERT ON address_changes TO panel_orders;
GRANT UPDATE (status, decided_at) ON address_changes TO panel_orders;
GRANT SELECT ON address_changes TO orders;
GRANT UPDATE (attempts, status, decided_at) ON address_changes TO orders;
GRANT UPDATE (ship_recipient, ship_street, ship_number, ship_city, ship_state, ship_postal_code) ON orders TO orders;

-- Retention (E8.5): the STL 90 days after delivery, or 30 days without a live
-- order, and the address 90 days after delivery or cancellation.
ALTER TABLE files ADD COLUMN purged_at timestamptz;

GRANT SELECT (id, created_at, purged_at) ON files TO retention;
GRANT UPDATE (purged_at) ON files TO retention;
-- The panel tells a purged design apart from a missing one.
GRANT SELECT (id, purged_at) ON files TO panel_orders;
GRANT SELECT (id, file_id, status, address_purged_at, quote_expires_at) ON orders TO retention;
GRANT SELECT ON order_status_history TO retention;
GRANT UPDATE (ship_recipient, ship_street, ship_number, ship_city, ship_state, ship_postal_code, address_purged_at) ON orders TO retention;
